package v1

import (
	"k8s.io/apimachinery/pkg/types"
)

func (n *NamespacedName) Equal(other *NamespacedName) bool {
	return n.Name == other.Name && n.Namespace == other.Namespace
}

func (n *NamespacedName) String() string {
	return n.Namespace + string(types.Separator) + n.Name
}
