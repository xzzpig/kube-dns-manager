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
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ResourceWatcherSpec defines the desired state of ResourceWatcher
type ResourceWatcherSpec struct {
	Resource NamespacedName `json:"resource"`
}

// ResourceWatcherStatus defines the observed state of ResourceWatcher
type ResourceWatcherStatus struct {
	Ready     bool            `json:"ready"`
	Reason    string          `json:"reason,omitempty"`
	Checked   bool            `json:"-"`
	Resources []WatchResource `json:"resources"`
}

type WatchResource struct {
	NamespacedName `json:",inline"`
	Kind           WatchResourceKind `json:"kind"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Resource",type=string,JSONPath=".spec.resource.name"
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=".status.reason"
// ResourceWatcher is the Schema for the resourcewatchers API
type ResourceWatcher struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceWatcherSpec   `json:"spec,omitempty"`
	Status ResourceWatcherStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceWatcherList contains a list of ResourceWatcher
type ResourceWatcherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceWatcher `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceWatcher{}, &ResourceWatcherList{})
}
