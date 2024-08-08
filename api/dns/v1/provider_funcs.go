package v1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (s *ProviderSelector) Matches(record *Record) (bool, error) {
	if s.Domain != "" {
		if record.Spec.Name != s.Domain && !strings.HasSuffix(record.Spec.Name, "."+s.Domain) {
			return false, nil
		}
	}
	if s.LabelSelector.Size() != 0 {
		selector, err := metav1.LabelSelectorAsSelector(&s.LabelSelector)
		if err != nil {
			return false, err
		}
		if !selector.Matches(labels.Set(record.Labels)) {
			return false, nil
		}
	}
	return true, nil
}

func (p *Provider) GetSpec() *ProviderSpec     { return &p.Spec }
func (p *Provider) GetStatus() *ProviderStatus { return &p.Status }
func (p *Provider) New() ProviderObject        { return &Provider{} }

func (p *ClusterProvider) GetSpec() *ProviderSpec     { return &p.Spec }
func (p *ClusterProvider) GetStatus() *ProviderStatus { return &p.Status }
func (p *ClusterProvider) New() ProviderObject        { return &ClusterProvider{} }

var (
	_ = (ProviderObject)(&Provider{})
	_ = (ProviderObject)(&ClusterProvider{})
)
