/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
)

// ResourceWatcherReconciler reconciles a ResourceWatcher object
type ResourceWatcherReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Template *template.Template
	Recorder record.EventRecorder

	cacheGenMap map[string]int64
	lock        sync.Mutex
}

// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=resourcewatchers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=resourcewatchers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=resourcewatchers/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces;services;endpoints;nodes;pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ResourceWatcher object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *ResourceWatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, _err error) {
	logger := log.FromContext(ctx)

	watcher := &dnsv1.ResourceWatcher{}
	if err := r.Get(ctx, req.NamespacedName, watcher); err != nil {
		return ctrl.Result{}, err
	}

	var generator dnsv1.GeneratorObject
	if err := r.getOwner(ctx, watcher, &generator); err != nil {
		return ctrl.Result{}, err
	}

	recordList := &dnsv1.RecordList{}
	if err := r.List(ctx, recordList, client.InNamespace(watcher.Namespace), client.MatchingFields{ownerReferencesField: string(watcher.UID)}); err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		if _err != nil {
			watcher.Status.Ready = false
			watcher.Status.Reason = _err.Error()
			logger.Error(_err, "failed to reconcile")
			_err = nil
		} else {
			watcher.Status.Ready = true
			watcher.Status.Reason = ""
			r.Recorder.Event(watcher, corev1.EventTypeNormal, "Parsed", "Record parsed successfully")
		}
		if err := r.Status().Update(ctx, watcher); err != nil {
			logger.Error(err, "failed to update status")
		}
	}()
	watcher.Status.Resources = make([]dnsv1.WatchResource, 0)

	templateData, err := r.getTemplateData(ctx, watcher, generator)
	if err != nil {
		r.Recorder.Event(watcher, corev1.EventTypeWarning, "Failed", "Failed to get template data")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	var cachedTpl *template.Template
	if generator.GetSpec().Template != "" {
		cachedTpl, err = r.getTemplate(fmt.Sprintf("Generator/%s/%s", generator.GetNamespace(), generator.GetName()), generator.GetGeneration(), generator.GetSpec().Template) //r.NewTemplate(generator.GetSpec().Template, nil)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if generator.GetSpec().TemplateRef != "" {
		switch generator.(type) {
		case *dnsv1.Generator:
			template := &dnsv1.Template{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: watcher.Namespace, Name: generator.GetSpec().TemplateRef}, template); err != nil {
				return ctrl.Result{}, err
			}
			cachedTpl, err = r.getTemplate(fmt.Sprintf("Template/%s/%s", template.Namespace, template.Name), template.Generation, template.Spec.Template) //r.NewTemplate(template.Spec.Template, template)
			if err != nil {
				return ctrl.Result{}, err
			}
			watcher.Status.AddResource(dnsv1.WatchResourceKindTemplate, watcher.Namespace, generator.GetSpec().TemplateRef)
		case *dnsv1.ClusterGenerator:
			template := &dnsv1.ClusterTemplate{}
			if err := r.Get(ctx, client.ObjectKey{Name: generator.GetSpec().TemplateRef}, template); err != nil {
				return ctrl.Result{}, err
			}
			cachedTpl, err = r.getTemplate(fmt.Sprintf("ClusterTemplate/%s", template.Name), template.Generation, template.Spec.Template) //r.NewTemplate(template.Spec.Template, template)
			if err != nil {
				return ctrl.Result{}, err
			}
			watcher.Status.AddResource(dnsv1.WatchResourceKindClusterTemplate, "", generator.GetSpec().TemplateRef)
		default:
			r.Recorder.Event(watcher, corev1.EventTypeWarning, "Failed", "No template specified")
			return ctrl.Result{}, ErrorUnknownKind
		}
	} else {
		r.Recorder.Event(watcher, corev1.EventTypeWarning, "Failed", "No template specified")
		return ctrl.Result{}, ErrorNoTemplate
	}
	parsedRecords, err := r.parse(ctx, cachedTpl, templateData)
	if err != nil {
		r.Recorder.Event(watcher, corev1.EventTypeWarning, "Failed", "Failed to parse template")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	for _, record := range parsedRecords {
		record.Namespace = watcher.Namespace
		ctrl.SetControllerReference(watcher, &record, r.Scheme)

		oldRecord := recordList.Get(record.Namespace, record.Name)
		oldVersion := ""
		if oldRecord != nil {
			oldVersion = oldRecord.ResourceVersion
			oldRecord.Labels = record.Labels
			oldRecord.Annotations = record.Annotations
			oldRecord.OwnerReferences = record.OwnerReferences
			oldRecord.Spec = record.Spec
			if err := r.Update(ctx, oldRecord); err != nil {
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
			oldRecord.Status.Checked = true
		} else {
			if err := r.Create(ctx, &record); err != nil {
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
			oldRecord = &record
		}
		if oldVersion != oldRecord.ResourceVersion {
			r.Recorder.Eventf(oldRecord, corev1.EventTypeNormal, "Modify", "Record modified by ResourceWatcher %s", watcher.Name)
		}
	}
	for _, record := range recordList.Items {
		if !record.Status.Checked {
			if err := r.Delete(ctx, &record); err != nil {
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *ResourceWatcherReconciler) parse(ctx context.Context, tpl *template.Template, data any) (records []dnsv1.Record, err error) {
	buffer := new(strings.Builder)
	err = tpl.Execute(buffer, data)
	if err != nil {
		return nil, err
	}
	str := buffer.String()
	cleanStr := strings.TrimPrefix(strings.TrimPrefix(str, "\n"), " ")
	if cleanStr == "" {
		return []dnsv1.Record{}, nil
	}
	defer func() {
		if err != nil {
			logger := log.FromContext(ctx)
			logger.Error(err, "failed to parse template", "template", str)
		}
	}()
	if strings.HasPrefix(cleanStr, "{") { //json object
		record := dnsv1.Record{}
		if err := json.Unmarshal([]byte(str), &record); err != nil {
			return nil, err
		}
		return []dnsv1.Record{record}, nil
	} else if strings.HasPrefix(cleanStr, "[") { //json array
		records := []dnsv1.Record{}
		if err := json.Unmarshal([]byte(str), &records); err != nil {
			return nil, err
		}
		return records, nil
	} else if strings.HasPrefix(cleanStr, "-") { //yaml list
		records := []dnsv1.Record{}
		if err := yaml.Unmarshal([]byte(str), &records); err != nil {
			return nil, err
		}
		return records, nil
	} else { //yaml object
		record := dnsv1.Record{}
		if err := yaml.Unmarshal([]byte(str), &record); err != nil {
			return nil, err
		}
		return []dnsv1.Record{record}, nil
	}
}

func (r *ResourceWatcherReconciler) getTemplate(key string, generation int64, tplString dnsv1.GoTemplateString) (tpl *template.Template, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	tpl = r.Template.Lookup(key)
	if tpl == nil {
		tpl = r.Template.New(key)
	}
	if r.cacheGenMap[key] != generation {
		if _, err := tpl.Parse(string(tplString)); err != nil {
			return nil, err
		}
		r.cacheGenMap[key] = generation
	}
	return tpl, nil
}

func (r *ResourceWatcherReconciler) getTemplateData(ctx context.Context, watcher *dnsv1.ResourceWatcher, generator dnsv1.GeneratorObject) (any, error) {
	switch generator.GetSpec().ResourceKind {
	case dnsv1.GeneratorResourceKindIngress:
		ingress := &netv1.Ingress{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: watcher.Spec.Resource.Namespace, Name: watcher.Spec.Resource.Name}, ingress); err != nil {
			return nil, err
		}
		return NewIngressTemplateData(NewTemplateData(ctx, watcher, r.Client), ingress), nil
	case dnsv1.GeneratorResourceKindRecord:
		record := &dnsv1.Record{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: watcher.Spec.Resource.Namespace, Name: watcher.Spec.Resource.Name}, record); err != nil {
			return nil, err
		}
		return NewRecordTemplateData(NewTemplateData(ctx, watcher, r.Client), record), nil
	case dnsv1.GeneratorResourceKindNode:
		node := &corev1.Node{}
		if err := r.Get(ctx, client.ObjectKey{Name: watcher.Spec.Resource.Name}, node); err != nil {
			return nil, err
		}
		return NewNodeTemplateData(NewTemplateData(ctx, watcher, r.Client), node), nil
	case dnsv1.GeneratorResourceKindService:
		service := &corev1.Service{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: watcher.Spec.Resource.Namespace, Name: watcher.Spec.Resource.Name}, service); err != nil {
			return nil, err
		}
		return NewServiceTemplateData(NewTemplateData(ctx, watcher, r.Client), service), nil
	default:
		return nil, ErrorUnknownKind
	}
}

// func (r *ResourceWatcherReconciler) NewTemplate(tplString dnsv1.GoTemplateString, tpl client.Object) (result *CachedTemplate) {
// 	result = new(CachedTemplate)
// 	result.ParentTpl = r.Template
// 	result.TemplateString = tplString
// 	result.cacheTplMap = r.cacheTplMap
// 	result.cacheGenMap = r.cacheGenMap
// 	if tpl != nil {
// 		result.Generation = tpl.GetGeneration()
// 		result.TemplateRef = &dnsv1.NamespacedName{Namespace: tpl.GetNamespace(), Name: tpl.GetName()}
// 	}
// 	return
// }

func (r *ResourceWatcherReconciler) getOwner(ctx context.Context, watcher *dnsv1.ResourceWatcher, generator *dnsv1.GeneratorObject) error {
	var obj dnsv1.GeneratorObject
	for _, ownerRef := range watcher.GetOwnerReferences() {
		if ownerRef.Kind == "Generator" {
			obj = &dnsv1.Generator{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: watcher.Namespace, Name: ownerRef.Name}, obj); err != nil {
				return err
			}
			*generator = obj
			break
		} else if ownerRef.Kind == "ClusterGenerator" {
			obj = &dnsv1.ClusterGenerator{}
			if err := r.Get(ctx, client.ObjectKey{Name: ownerRef.Name}, obj); err != nil {
				return err
			}
			*generator = obj
		} else {
			return ErrorUnknownKind
		}
	}
	return nil
}

func (r *ResourceWatcherReconciler) watchResources(kind dnsv1.WatchResourceKind) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		watchResource := &dnsv1.WatchResource{
			NamespacedName: dnsv1.NamespacedName{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
			},
			Kind: kind,
		}

		logger := log.FromContext(ctx).WithName("ResourceWatcher").WithValues("resource", watchResource)

		watcherList := &dnsv1.ResourceWatcherList{}
		if err := r.List(ctx, watcherList, client.MatchingFields{resourcesField: watchResource.String()}); err != nil {
			logger.Error(err, "failed to list resource watchers")
			return []ctrl.Request{}
		}

		requests := make([]reconcile.Request, len(watcherList.Items))
		for i, watcher := range watcherList.Items {
			requests[i] = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: watcher.Namespace, Name: watcher.Name}}
			r.Recorder.Eventf(&watcher, corev1.EventTypeNormal, "Trigger", "Record re-parsing, trigged by %s", watchResource.String())
		}

		return requests
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceWatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &dnsv1.ResourceWatcher{}, ownerReferencesField, func(rawObj client.Object) []string {
		ownerRefs := rawObj.GetOwnerReferences()
		owners := make([]string, len(ownerRefs))
		for i, refs := range ownerRefs {
			owners[i] = string(refs.UID)
		}
		return owners
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &dnsv1.Record{}, ownerReferencesField, func(rawObj client.Object) []string {
		ownerRefs := rawObj.GetOwnerReferences()
		owners := make([]string, len(ownerRefs))
		for i, refs := range ownerRefs {
			owners[i] = string(refs.UID)
		}
		return owners
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &dnsv1.ResourceWatcher{}, resourcesField, func(rawObj client.Object) []string {
		watcher := rawObj.(*dnsv1.ResourceWatcher)
		resources := make([]string, len(watcher.Status.Resources))
		for i, resource := range watcher.Status.Resources {
			resources[i] = resource.String()
		}
		return resources
	}); err != nil {
		return err
	}

	r.cacheGenMap = make(map[string]int64)

	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1.ResourceWatcher{}).
		Watches(&dnsv1.Template{}, handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.WatchResourceKindTemplate))).
		Watches(&dnsv1.ClusterTemplate{}, handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.WatchResourceKindClusterTemplate))).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.WatchResourceKindNamespace))).
		Watches(&netv1.Ingress{}, handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.WatchResourceKindIngress))).
		Watches(&corev1.Service{}, handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.WatchResourceKindService))).
		Watches(&corev1.Endpoints{}, handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.WatchResourceKindEndpoints))).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.WatchResourceKindNode))).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.WatchResourceKindPod))).
		Watches(&dnsv1.Record{}, handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.WatchResourceKindRecord))).
		Complete(r)
}
