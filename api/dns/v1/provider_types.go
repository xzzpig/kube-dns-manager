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

// +kubebuilder:validation:Enum=ALIYUN;CLOUDFLARE
type ProviderType string

const (
	ProviderTypeAliyun     ProviderType = "ALIYUN"
	ProviderTypeCloudflare ProviderType = "CLOUDFLARE"
)

type AliyunProviderConfig struct {
	// If empty, spec.selector.domain will be used as domain name
	DomainName      string `json:"domainName,omitempty"`
	AccessKeyID     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	// +kubebuilder:default: dns.aliyuncs.com
	Endpoint string `json:"endpoint,omitempty"`
}

type CloudflareProviderConfig struct {
	// If empty, spec.selector.domain will be used as zone name
	ZoneName string `json:"zoneName,omitempty"`
	APIToken string `json:"apiToken,omitempty"`
	Key      string `json:"key,omitempty"`
	Email    string `json:"email,omitempty"`
	// When creating a Record, if the record already exists in CloudFlare, should it be associated with the existing record? Otherwise, an error will be reported
	MatchExistsRecord bool `json:"matchExistsRecord,omitempty"`
}

// ProviderSpec defines the desired state of Provider
type ProviderSpec struct {
	Type       ProviderType              `json:"type"`
	Selector   ProviderSelector          `json:"selector,omitempty"`
	Aliyun     *AliyunProviderConfig     `json:"aliyun,omitempty"`
	Cloudflare *CloudflareProviderConfig `json:"cloudflare,omitempty"`
}

type ProviderSelector struct {
	// Records which has the same domain (suffix) will be managed by this provider, should not start with a dot (.)
	Domain               string `json:"domain,omitempty"`
	metav1.LabelSelector `json:",inline"`
}

// ProviderStatus defines the observed state of Provider
type ProviderStatus struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

// +kubebuilder:object:generate=false
type ProviderObject interface {
	client.Object
	ObjectSpec[ProviderSpec]
	ObjectStatus[ProviderStatus]
	ObjectNewer[ProviderObject]
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.selector.domain`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.reason`,priority=1
// Provider is the Schema for the providers API
type Provider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderSpec   `json:"spec,omitempty"`
	Status ProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderList contains a list of Provider
type ProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Provider `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.selector.domain`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.reason`,priority=1
// ClusterProvider is the Schema for the clusterproviders API
type ClusterProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderSpec   `json:"spec,omitempty"`
	Status ProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterProviderList contains a list of ClusterProvider
type ClusterProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Provider{}, &ProviderList{})
	SchemeBuilder.Register(&ClusterProvider{}, &ClusterProviderList{})
}
