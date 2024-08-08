package dns

import (
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	resourceKindField    = ".spec.resourceKind"
	ownerReferencesField = ".metadata.ownerReferences"
	resourcesField       = ".status.resources"
	providersField       = ".status.providers"
	finalizerLabel       = "dns.xzzpig.com/finalizer"
)

var (
	ErrorWaitRecords = errors.New("waiting for Records")
	ErrorUnknownKind = errors.New("unknown kind")
	ErrorNoTemplate  = errors.New("no template specified")
)

func addFinalizer[T client.Object](object T) (changed bool) {
	for _, finalizer := range object.GetFinalizers() {
		if finalizer == finalizerLabel {
			return false
		}
	}
	object.SetFinalizers(append(object.GetFinalizers(), finalizerLabel))
	return true
}

func removeFinalizer[T client.Object](object T) {
	for i, finalizer := range object.GetFinalizers() {
		if finalizer == finalizerLabel {
			object.SetFinalizers(append(object.GetFinalizers()[:i], object.GetFinalizers()[i+1:]...))
			break
		}
	}
}
