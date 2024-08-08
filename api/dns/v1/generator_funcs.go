package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (g *GeneratorSpec) Matches(obj client.Object) (bool, error) {
	selector, err := metav1.LabelSelectorAsSelector(&g.Selector)
	if err != nil {
		return false, err
	}
	return selector.Matches(labels.Set(obj.GetLabels())), nil
}

func (s *GeneratorStatus) AddResource(res NamespacedName) (changed bool) {
	for _, r := range s.Resources {
		if res.Equal(&r) {
			return false
		}
	}
	s.Resources = append(s.Resources, res)
	return true
}

func (s *GeneratorStatus) RemoveResource(res NamespacedName) (changed bool) {
	for i, r := range s.Resources {
		if res.Equal(&r) {
			s.Resources = append(s.Resources[:i], s.Resources[i+1:]...)
			changed = true
		}
	}
	return changed
}

func (g *Generator) GetSpec() *GeneratorSpec     { return &g.Spec }
func (g *Generator) GetStatus() *GeneratorStatus { return &g.Status }
func (g *Generator) New() GeneratorObject        { return &Generator{} }

func (g *ClusterGenerator) GetSpec() *GeneratorSpec     { return &g.Spec }
func (g *ClusterGenerator) GetStatus() *GeneratorStatus { return &g.Status }
func (g *ClusterGenerator) New() GeneratorObject        { return &ClusterGenerator{} }

var (
	_ = (GeneratorObject)(&Generator{})
	_ = (GeneratorObject)(&ClusterGenerator{})
)
