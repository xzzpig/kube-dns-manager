package v1

func (w *ResourceWatcherList) Get(ref NamespacedName) *ResourceWatcher {
	for i := range w.Items {
		watcher := &w.Items[i]
		if watcher.Spec.Resource.Name == ref.Name && watcher.Spec.Resource.Namespace == ref.Namespace {
			return watcher
		}
	}
	return nil
}

func (s *ResourceWatcherStatus) AddResource(kind WatchResourceKind, namespace, name string) {
	for _, r := range s.Resources {
		if r.Kind == kind && r.Name == name && r.Namespace == namespace {
			return
		}
	}
	s.Resources = append(s.Resources, WatchResource{
		NamespacedName: NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
		Kind: kind,
	})
}

func (r *WatchResource) String() string {
	return string(r.Kind) + "/" + r.Namespace + "/" + r.Name
}
