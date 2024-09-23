package v1

import "sigs.k8s.io/controller-runtime/pkg/client"

// GoTemplateString is a string that represents a Go template
type GoTemplateString string

// +kubebuilder:validation:Enum=Ingress;Record;Node
type GeneratorResourceKind string

const (
	GeneratorResourceKindIngress GeneratorResourceKind = "Ingress"
	GeneratorResourceKindRecord  GeneratorResourceKind = "Record"
	GeneratorResourceKindNode    GeneratorResourceKind = "Node"
)

type WatchResourceKind string

const (
	WatchResourceKindTemplate        WatchResourceKind = "Template"
	WatchResourceKindClusterTemplate WatchResourceKind = "ClusterTemplate"
	WatchResourceKindNamespace       WatchResourceKind = "Namespace"
	WatchResourceKindIngress         WatchResourceKind = "Ingress"
	WatchResourceKindService         WatchResourceKind = "Service"
	WatchResourceKindEndpoints       WatchResourceKind = "Endpoints"
	WatchResourceKindNode            WatchResourceKind = "Node"
	WatchResourceKindPod             WatchResourceKind = "Pod"
	WatchResourceKindRecord          WatchResourceKind = "Record"
)

// +kubebuilder:validation:Enum=A;CNAME;TXT;MX;SRV;AAAA;NS;CAA
type RecordType string

const (
	RecordTypeA     RecordType = "A"
	RecordTypeCNAME RecordType = "CNAME"
	RecordTypeTXT   RecordType = "TXT"
	RecordTypeMX    RecordType = "MX"
	RecordTypeSRV   RecordType = "SRV"
	RecordTypeAAAA  RecordType = "AAAA"
	RecordTypeNS    RecordType = "NS"
	RecordTypeCAA   RecordType = "CAA"
)

type NamespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// +kubebuilder:object:generate=false
type ObjectSpec[SPEC any] interface {
	GetSpec() *SPEC
}

// +kubebuilder:object:generate=false
type ObjectStatus[STATUS any] interface {
	GetStatus() *STATUS
}

// +kubebuilder:object:generate=false
type ObjectNewer[T client.Object] interface {
	New() T
}
