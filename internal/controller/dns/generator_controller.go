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

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// GeneratorReconciler reconciles a Generator object
type GeneratorReconciler[T dnsv1.GeneratorObject] struct {
	client.Client
	Scheme *runtime.Scheme
	newer  T
}

// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=generators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=generators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=generators/finalizers,verbs=update
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=clustergenerators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=clustergenerators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=clustergenerators/finalizers,verbs=update
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=clustertemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=templates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Generator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *GeneratorReconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// logger := log.FromContext(ctx)

	generator := r.newer.New()
	if err := r.Get(ctx, req.NamespacedName, generator); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// spec changed, re-match resources
	if generator.GetGeneration() != generator.GetStatus().AppliedGeneration {
		resources, err := r.listResources(ctx, generator)
		if err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		generator.GetStatus().Resources = make([]dnsv1.NamespacedName, 0)
		for _, resource := range resources {
			if ok, _ := generator.GetSpec().Matches(resource); ok {
				generator.GetStatus().AddResource(dnsv1.NamespacedName{
					Name:      resource.GetName(),
					Namespace: resource.GetNamespace(),
				})
			}
		}
		return r.updateAppliedGeneration(ctx, generator, generator.GetGeneration())
	}

	// list owned watchers
	watcherList := &dnsv1.ResourceWatcherList{}
	if err := r.List(ctx, watcherList, &client.MatchingFields{ownerReferencesField: string(generator.GetUID())}); err != nil {
		return ctrl.Result{}, err
	}

	// create watchers for matched resources
	for _, resource := range generator.GetStatus().Resources {
		resourceObj, err := r.getResource(ctx, generator.GetSpec().ResourceKind, resource)
		if apierrors.IsNotFound(err) {
			generator.GetStatus().RemoveResource(resource)
			return ctrl.Result{}, r.Status().Update(ctx, generator)
		} else if err != nil {
			return ctrl.Result{}, err
		}
		if ok, _ := generator.GetSpec().Matches(resourceObj); !ok { // resource no longer matches, do re-match
			return r.updateAppliedGeneration(ctx, generator, 0)
		}

		watcher := watcherList.Get(resource)
		if watcher == nil {
			watcher = new(dnsv1.ResourceWatcher)
			if generator.GetSpec().WatcherGenerateName == "" {
				watcher.GenerateName = "watcher-"
			} else {
				watcher.GenerateName = generator.GetSpec().WatcherGenerateName
			}
			watcher.Namespace = resource.Namespace
			if err := ctrl.SetControllerReference(generator, watcher, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
		}

		watcher.Status.Checked = true

		if !watcher.Spec.Resource.Equal(&resource) {
			watcher.Spec.Resource = *resource.DeepCopy()
			watcher.Status.Ready = false
			if watcher.Generation == 0 {
				if err := r.Create(ctx, watcher); err != nil {
					return ctrl.Result{}, err
				}
			} else {
				if err := r.Update(ctx, watcher); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	for _, watcher := range watcherList.Items {
		if !watcher.Status.Checked {
			if err := r.Delete(ctx, &watcher); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *GeneratorReconciler[T]) updateAppliedGeneration(ctx context.Context, generator dnsv1.GeneratorObject, generation int64) (ctrl.Result, error) {
	generator.GetStatus().AppliedGeneration = generation
	if err := r.Status().Update(ctx, generator); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *GeneratorReconciler[T]) getResource(ctx context.Context, kind dnsv1.GeneratorResourceKind, resource dnsv1.NamespacedName) (client.Object, error) {
	switch kind {
	case dnsv1.GeneratorResourceKindIngress:
		ingress := &netv1.Ingress{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: resource.Namespace, Name: resource.Name}, ingress); err != nil {
			return nil, err
		}
		return ingress, nil
	case dnsv1.GeneratorResourceKindRecord:
		record := &dnsv1.Record{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: resource.Namespace, Name: resource.Name}, record); err != nil {
			return nil, err
		}
		return record, nil
	default:
		return nil, ErrorUnknownKind
	}
}

func (r *GeneratorReconciler[T]) listResources(ctx context.Context, generator dnsv1.GeneratorObject) ([]client.Object, error) {
	spec := generator.GetSpec()
	selector, err := metav1.LabelSelectorAsSelector(&spec.Selector)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	switch spec.ResourceKind {
	case dnsv1.GeneratorResourceKindIngress:
		list := &netv1.IngressList{}
		if err = r.List(ctx, list, &client.ListOptions{
			Namespace:     generator.GetNamespace(),
			LabelSelector: selector,
		}); err != nil {
			return nil, err
		}
		objs = make([]client.Object, len(list.Items))
		for i := range list.Items {
			objs[i] = &list.Items[i]
		}
	case dnsv1.GeneratorResourceKindRecord:
		list := &dnsv1.RecordList{}
		if err = r.List(ctx, list, &client.ListOptions{
			Namespace:     generator.GetNamespace(),
			LabelSelector: selector,
		}); err != nil {
			return nil, err
		}
		objs = make([]client.Object, len(list.Items))
		for i := range list.Items {
			objs[i] = &list.Items[i]
		}
	default:
		return nil, ErrorUnknownKind
	}
	return objs, nil
}

// watch resource's changes and update to generator's status
func (r *GeneratorReconciler[T]) watchResources(resourceType dnsv1.GeneratorResourceKind) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []ctrl.Request {
		logger := log.FromContext(ctx).WithName("Watcher").WithValues("resource", obj.GetName()).WithValues("kind", resourceType)

		checkGenerator := func(generator dnsv1.GeneratorObject) {
			changed := false
			if ok, err := generator.GetSpec().Matches(obj); err != nil {
				logger.Error(err, "Failed to match generator", "generator", generator.GetName())
			} else if ok && obj.GetDeletionTimestamp() == nil {
				changed = generator.GetStatus().AddResource(dnsv1.NamespacedName{
					Name:      obj.GetName(),
					Namespace: obj.GetNamespace(),
				})
			} else {
				changed = generator.GetStatus().RemoveResource(dnsv1.NamespacedName{
					Name:      obj.GetName(),
					Namespace: obj.GetNamespace(),
				})
			}
			if changed {
				if err := r.Status().Update(ctx, generator); err != nil {
					logger.Error(err, "Failed to update generator status", "generator", generator.GetName())
				}
			}
		}

		switch r.newer.New().(type) {
		case *dnsv1.Generator:
			generators := &dnsv1.GeneratorList{}
			if err := r.List(ctx, generators, &client.ListOptions{
				Namespace:     obj.GetNamespace(),
				FieldSelector: fields.OneTermEqualSelector(resourceKindField, string(resourceType)),
			}); err != nil {
				return []ctrl.Request{}
			}
			for _, generator := range generators.Items {
				checkGenerator(&generator)
			}
		case *dnsv1.ClusterGenerator:
			generators := &dnsv1.ClusterGeneratorList{}
			if err := r.List(ctx, generators, &client.ListOptions{
				FieldSelector: fields.OneTermEqualSelector(resourceKindField, string(resourceType)),
			}); err != nil {
				return []ctrl.Request{}
			}
			for _, generator := range generators.Items {
				checkGenerator(&generator)
			}
		default:
			logger.Error(ErrorUnknownKind, "failed to watch resources, unknown generator type")
		}

		return []ctrl.Request{}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *GeneratorReconciler[T]) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), r.newer.New(), resourceKindField, func(rawObj client.Object) []string {
		dnsGenerator := rawObj.(dnsv1.GeneratorObject)
		return []string{string(dnsGenerator.GetSpec().ResourceKind)}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(r.newer.New()).
		Watches(&netv1.Ingress{},
			handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.GeneratorResourceKindIngress)),
			builder.WithPredicates(predicate.Or(
				predicate.LabelChangedPredicate{},
				predicate.Funcs{DeleteFunc: func(e event.DeleteEvent) bool { return true }},
			))).
		Watches(&dnsv1.Record{},
			handler.EnqueueRequestsFromMapFunc(r.watchResources(dnsv1.GeneratorResourceKindRecord)),
			builder.WithPredicates(predicate.Or(
				predicate.LabelChangedPredicate{},
				predicate.Funcs{DeleteFunc: func(e event.DeleteEvent) bool { return true }},
			))).
		Complete(r)
}
