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
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
	"github.com/xzzpig/kube-dns-manager/internal/controller/dns/provider"
	corev1 "k8s.io/api/core/v1"
)

const (
	TimeWaitProvider = time.Minute
)

var NewPayload = provider.NewPayload

// RecordReconciler reconciles a Record object
type RecordReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=records,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=records/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=records/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Record object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *RecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ctx = context.WithValue(ctx, provider.CtxKeyClient, r.Client)

	record := &dnsv1.Record{}
	if err := r.Get(ctx, req.NamespacedName, record); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if addFinalizer(record) {
		if err := r.Update(ctx, record); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	providerList := &dnsv1.ProviderList{}
	if err := r.List(ctx, providerList, client.InNamespace(record.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	clusterProviderList := &dnsv1.ClusterProviderList{}
	if err := r.List(ctx, clusterProviderList); err != nil {
		return ctrl.Result{}, err
	}

	providers := make([]dnsv1.ProviderObject, 0, len(providerList.Items)+len(clusterProviderList.Items))
	for _, provider := range providerList.Items {
		providers = append(providers, &provider)
	}
	for _, provider := range clusterProviderList.Items {
		providers = append(providers, &provider)
	}

	//handle matched providers
	for _, provider := range providers {
		if ok, err := provider.GetSpec().Selector.Matches(record); err != nil {
			return ctrl.Result{}, err
		} else if !ok {
			continue
		}

		providerStatus := record.Status.FindProviderStatus(dnsv1.NamespacedName{Namespace: provider.GetNamespace(), Name: provider.GetName()})
		if providerStatus == nil {
			providerStatus = &dnsv1.RecordProviderStatus{NamespacedName: dnsv1.NamespacedName{Namespace: provider.GetNamespace(), Name: provider.GetName()}}
			record.Status.Providers = append(record.Status.Providers, providerStatus)
		}
		providerStatus.Checked = true

		dnsProvider := providerCache[provider.GetUID()]
		if dnsProvider == nil || dnsProvider.Generation != provider.GetGeneration() {
			providerStatus.Message = "provider not ready"
			continue
		}

		payload := NewPayload(providerStatus, &record.Spec)
		if !record.DeletionTimestamp.IsZero() || !provider.GetDeletionTimestamp().IsZero() { // delete
			if providerStatus.RecordID == "" { // already deleted or not yet created
				providerStatus.Success(providerStatus.RecordID, providerStatus.Data)
				continue
			}
			if err := dnsProvider.Delete(ctx, payload); err != nil {
				providerStatus.Error(payload.Id, payload.Data, err)
				r.Recorder.Eventf(record, corev1.EventTypeWarning, "Failed", "Failed to delete record by provider %s", providerStatus.NamespacedName.String())
			} else {
				providerStatus.Success(payload.Id, payload.Data)
				r.Recorder.Eventf(record, corev1.EventTypeNormal, "Deleted", "Record is deleted by provider %s", providerStatus.NamespacedName.String())
			}
		} else if providerStatus.RecordID == "" { // create
			if err := dnsProvider.Create(ctx, payload); err != nil {
				providerStatus.Error(payload.Id, payload.Data, err)
				r.Recorder.Eventf(record, corev1.EventTypeWarning, "Failed", "Failed to create record by provider %s", providerStatus.NamespacedName.String())
			} else {
				providerStatus.Success(payload.Id, payload.Data)
				r.Recorder.Eventf(record, corev1.EventTypeNormal, "Created", "Record is created by provider %s", providerStatus.NamespacedName.String())
			}
		} else { //update
			if err := dnsProvider.Update(ctx, payload); err != nil {
				providerStatus.Error(payload.Id, payload.Data, err)
				r.Recorder.Eventf(record, corev1.EventTypeWarning, "Failed", "Failed to update record by provider %s", providerStatus.NamespacedName.String())
			} else {
				providerStatus.Success(payload.Id, payload.Data)
				r.Recorder.Eventf(record, corev1.EventTypeNormal, "Updated", "Record is updated by provider %s", providerStatus.NamespacedName.String())
			}
		}
	}

	// handle not matched providers
	providerStatusCount := len(record.Status.Providers)
	for i, providerStatus := range record.Status.Providers {
		if providerStatus.Checked {
			continue
		}
		var provider dnsv1.ProviderObject
		if err := r.getProvider(ctx, types.NamespacedName{Namespace: providerStatus.Namespace, Name: providerStatus.Name}, &provider); client.IgnoreNotFound(err) != nil {
			providerStatus.Error(providerStatus.RecordID, providerStatus.Data, err)
			continue
		}
		if providerStatus.RecordID != "" && provider != nil {
			dnsProvider := providerCache[provider.GetUID()]
			if dnsProvider == nil || dnsProvider.Generation != provider.GetGeneration() {
				providerStatus.Message = "provider not ready"
				continue
			}
			payload := NewPayload(providerStatus, &record.Spec)
			if err := dnsProvider.Delete(ctx, payload); err != nil {
				providerStatus.Error(payload.Id, payload.Data, err)
				r.Recorder.Eventf(record, corev1.EventTypeWarning, "Failed", "Failed to delete record by provider %s", providerStatus.NamespacedName.String())
				continue
			}
			r.Recorder.Eventf(record, corev1.EventTypeNormal, "Deleted", "Record is deleted by provider %s", providerStatus.NamespacedName.String())
		}
		record.Status.Providers[i] = nil
		providerStatusCount--
	}

	if providerStatusCount != len(record.Status.Providers) {
		newStatus := make([]*dnsv1.RecordProviderStatus, 0, providerStatusCount)
		for _, providerStatus := range record.Status.Providers {
			if providerStatus != nil {
				newStatus = append(newStatus, providerStatus)
			}
		}
		record.Status.Providers = newStatus
	}

	record.Status.AllReady = true
	record.Status.Message = ""
	for _, providerStatus := range record.Status.Providers {
		if providerStatus.Message != "" {
			record.Status.AllReady = false
			record.Status.Message += providerStatus.Message + "\n"
		}
	}
	record.Status.Message = strings.TrimSuffix(record.Status.Message, "\n")
	if err := r.Status().Update(ctx, record); err != nil {
		logger.Error(err, "failed to update record status")
		r.Recorder.Event(record, corev1.EventTypeWarning, "Failed", "Failed to update record status")
	}
	if !record.Status.AllReady {
		return ctrl.Result{RequeueAfter: TimeWaitProvider}, nil
	}

	if !record.DeletionTimestamp.IsZero() {
		removeFinalizer(record)
		if err := r.Update(ctx, record); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *RecordReconciler) getProvider(ctx context.Context, key types.NamespacedName, provider *dnsv1.ProviderObject) error {
	var obj dnsv1.ProviderObject
	if key.Namespace == "" {
		obj = &dnsv1.ClusterProvider{}

	} else {
		obj = &dnsv1.Provider{}
	}
	if err := r.Get(ctx, key, obj); err != nil {
		return err
	}
	*provider = obj
	return nil
}

func (r *RecordReconciler) watchForProviders(ctx context.Context, o client.Object) []reconcile.Request {
	provider := o.(dnsv1.ProviderObject)
	selector, err := metav1.LabelSelectorAsSelector(&provider.GetSpec().Selector.LabelSelector)
	if err != nil {
		return []reconcile.Request{}
	}

	recordList := &dnsv1.RecordList{}
	if err := r.List(ctx, recordList, &client.ListOptions{
		Namespace:     provider.GetNamespace(),
		LabelSelector: selector,
	}); err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, record := range recordList.Items {
		if ok, err := provider.GetSpec().Selector.Matches(&record); err != nil {
			continue
		} else if ok {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: record.Namespace, Name: record.Name}})
		}
	}

	return requests
}

func (r *RecordReconciler) getProviderWatchPredicates() builder.Predicates {
	return builder.WithPredicates(
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectNew == nil || e.ObjectOld == nil {
					return false
				}
				return !e.ObjectNew.GetDeletionTimestamp().IsZero() || e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
		})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &dnsv1.Record{}, providersField, func(o client.Object) []string {
		record := o.(*dnsv1.Record)
		providers := make([]string, 0, len(record.Status.Providers))
		for _, provider := range record.Status.Providers {
			if provider == nil {
				continue
			}
			providers = append(providers, provider.NamespacedName.String())
		}
		return providers
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1.Record{}).
		Watches(&dnsv1.Provider{}, handler.EnqueueRequestsFromMapFunc(r.watchForProviders), r.getProviderWatchPredicates()).
		Watches(&dnsv1.ClusterProvider{}, handler.EnqueueRequestsFromMapFunc(r.watchForProviders), r.getProviderWatchPredicates()).
		WithEventFilter(predicate.Or(
			predicate.GenerationChangedPredicate{},
			predicate.LabelChangedPredicate{},
		)).
		Complete(r)
}
