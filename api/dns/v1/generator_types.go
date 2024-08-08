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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GeneratorSpec defines the desired state of Generator
type GeneratorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Selector     metav1.LabelSelector  `json:"selector,omitempty"`
	ResourceKind GeneratorResourceKind `json:"resourceKind"`
	TemplateRef  string                `json:"templateRef,omitempty"`
	Template     GoTemplateString      `json:"template,omitempty"`
	// +kubebuilder:default:watcher-
	WatcherGenerateName string `json:"watcherGenerateName,omitempty"`
}

// GeneratorStatus defines the observed state of Generator
type GeneratorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Resources watched by the generator
	Resources         []NamespacedName `json:"resources,omitempty"`
	AppliedGeneration int64            `json:"appliedGeneration,omitempty"`
}

// +kubebuilder:object:generate=false
type GeneratorObject interface {
	client.Object
	ObjectSpec[GeneratorSpec]
	ObjectStatus[GeneratorStatus]
	ObjectNewer[GeneratorObject]
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=boolean,JSONPath=`.spec.resourceKind`
// Generator is the Schema for the generators API
type Generator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GeneratorSpec   `json:"spec,omitempty"`
	Status GeneratorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GeneratorList contains a list of Generator
type GeneratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Generator `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Kind",type=boolean,JSONPath=`.spec.resourceKind`
// ClusterGenerator is the Schema for the clustergenerators API
type ClusterGenerator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GeneratorSpec   `json:"spec,omitempty"`
	Status GeneratorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterGeneratorList contains a list of ClusterGenerator
type ClusterGeneratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterGenerator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Generator{}, &GeneratorList{})
	SchemeBuilder.Register(&ClusterGenerator{}, &ClusterGeneratorList{})
}
