package dns

import (
	"context"
	"reflect"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type TemplateData struct {
	ctx     context.Context
	watcher *dnsv1.ResourceWatcher
	client  client.Client
}

func (d *TemplateData) GetNamespace() (*corev1.Namespace, error) {
	ns := &corev1.Namespace{}
	if err := d.client.Get(d.ctx, client.ObjectKey{Name: d.watcher.Namespace}, ns); err != nil {
		return nil, err
	}
	d.watcher.Status.AddResource(dnsv1.WatchResourceKindNamespace, ns.Namespace, ns.Name)
	return ns, nil
}

type IngressTemplateData struct {
	TemplateData `json:",inline"`
	ingress      *netv1.Ingress
}

func (d *IngressTemplateData) Ingress() *IngressData {
	d.watcher.Status.AddResource(dnsv1.WatchResourceKindIngress, d.ingress.Namespace, d.ingress.Name)
	return &IngressData{d.TemplateData, d.ingress}
}

type IngressData struct {
	TemplateData
	*netv1.Ingress
}

func (i *IngressData) Service(ruleIndex, pathIndex int) (*ServiceData, error) {
	serviceName := i.Spec.Rules[ruleIndex].IngressRuleValue.HTTP.Paths[pathIndex].Backend.Service.Name
	service := &corev1.Service{}
	if err := i.client.Get(i.ctx, client.ObjectKey{Namespace: i.Namespace, Name: serviceName}, service); err != nil {
		return nil, err
	}
	i.watcher.Status.AddResource(dnsv1.WatchResourceKindService, i.Namespace, serviceName)
	return &ServiceData{i.TemplateData, service}, nil
}

type ServiceData struct {
	TemplateData
	*corev1.Service
}

func (s *ServiceData) Endpoints() (*EndpointsData, error) {
	endpoints := &corev1.Endpoints{}
	if err := s.client.Get(s.ctx, client.ObjectKey{Namespace: s.Namespace, Name: s.Name}, endpoints); err != nil {
		return nil, err
	}
	s.watcher.Status.AddResource(dnsv1.WatchResourceKindEndpoints, endpoints.Namespace, endpoints.Name)
	return &EndpointsData{s.TemplateData, endpoints}, nil
}

type EndpointsData struct {
	TemplateData
	*corev1.Endpoints
}

func (e *EndpointsData) Nodes() ([]NodeData, error) {
	nodeMap := make(map[string]*corev1.Node)
	for _, subset := range e.Subsets {
		for _, address := range subset.Addresses {
			if address.NodeName == nil {
				continue
			}
			if _, ok := nodeMap[*address.NodeName]; !ok {
				node := &corev1.Node{}
				if err := e.client.Get(e.ctx, client.ObjectKey{Name: *address.NodeName}, node); err != nil {
					return nil, err
				}
				nodeMap[*address.NodeName] = node
			}
		}
	}
	nodes := make([]NodeData, len(nodeMap))
	i := 0
	for _, node := range nodeMap {
		nodes[i] = NodeData{e.TemplateData, node}
		e.watcher.Status.AddResource(dnsv1.WatchResourceKindNode, node.Namespace, node.Name)
		i++
	}
	return nodes, nil
}

func (e *EndpointsData) Pods() ([]PodData, error) {
	podMap := make(map[types.UID]*corev1.Pod)
	for _, subset := range e.Subsets {
		for _, address := range subset.Addresses {
			if address.TargetRef == nil {
				continue
			}
			if address.TargetRef.Kind != "Pod" {
				continue
			}
			if _, ok := podMap[address.TargetRef.UID]; !ok {
				pod := &corev1.Pod{}
				if err := e.client.Get(e.ctx, client.ObjectKey{Namespace: e.Namespace, Name: address.TargetRef.Name}, pod); err != nil {
					return nil, err
				}
				podMap[address.TargetRef.UID] = pod
			}
		}
	}
	pods := make([]PodData, len(podMap))
	i := 0
	for _, pod := range podMap {
		pods[i] = PodData{e.TemplateData, pod}
		e.watcher.Status.AddResource(dnsv1.WatchResourceKindPod, pod.Namespace, pod.Name)
		i++
	}
	return pods, nil
}

type PodData struct {
	TemplateData
	*corev1.Pod
}

func (p *PodData) Node() (*NodeData, error) {
	if p.Spec.NodeName == "" {
		return nil, nil
	}
	node := &corev1.Node{}
	if err := p.client.Get(p.ctx, client.ObjectKey{Name: p.Spec.NodeName}, node); err != nil {
		return nil, err
	}
	p.watcher.Status.AddResource(dnsv1.WatchResourceKindNode, node.Namespace, node.Name)
	return &NodeData{p.TemplateData, node}, nil
}

type NodeData struct {
	TemplateData
	*corev1.Node
}

func (n *NodeData) Ready() bool {
	for _, condition := range n.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func NewTemplateData(ctx context.Context, watcher *dnsv1.ResourceWatcher, client client.Client) TemplateData {
	return TemplateData{
		ctx:     ctx,
		watcher: watcher,
		client:  client,
	}
}

type RecordTemplateData struct {
	TemplateData `json:",inline"`
	record       *dnsv1.Record
}

func (d *RecordTemplateData) Record() *dnsv1.Record {
	d.watcher.Status.AddResource(dnsv1.WatchResourceKindRecord, d.record.Namespace, d.record.Name)
	return d.record
}

func NewIngressTemplateData(data TemplateData, ingress *netv1.Ingress) *IngressTemplateData {
	return &IngressTemplateData{
		TemplateData: data,
		ingress:      ingress,
	}
}

func NewRecordTemplateData(data TemplateData, record *dnsv1.Record) *RecordTemplateData {
	return &RecordTemplateData{
		TemplateData: data,
		record:       record,
	}
}

func NewNodeTemplateData(data TemplateData, node *corev1.Node) *NodeData {
	return &NodeData{
		TemplateData: data,
		Node:         node,
	}
}

func NewTemplate(name string) *template.Template {
	return template.New(name).
		Funcs(sprig.FuncMap()).
		Funcs(template.FuncMap{
			"toYaml": func(v any) (string, error) {
				if v == nil || reflect.ValueOf(v).IsNil() {
					return "", nil
				}
				bs, err := yaml.Marshal(v)
				if err != nil {
					return "", err
				}
				return string(bs), nil
			},
			"unPtrStr": func(v *string) string {
				if v == nil {
					return ""
				}
				return *v
			},
		})
}
