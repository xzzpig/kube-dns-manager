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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
	"github.com/xzzpig/kube-dns-manager/internal/controller/dns/provider"

	_ "github.com/xzzpig/kube-dns-manager/internal/controller/dns/provider/alidns"
	_ "github.com/xzzpig/kube-dns-manager/internal/controller/dns/provider/cloudflare"
)

var providerCache = make(map[types.UID]*provider.CachedDnsProvider)

// ProviderReconciler reconciles a Provider object
type ProviderReconciler[T dnsv1.ProviderObject] struct {
	client.Client
	Scheme *runtime.Scheme
	newer  T
}

// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=providers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=providers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=providers/finalizers,verbs=update
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=clusterproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=clusterproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.xzzpig.com,resources=clusterproviders/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Provider object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *ProviderReconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	p := r.newer.New()
	if err := r.Get(ctx, req.NamespacedName, p); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if addFinalizer(p) {
		if err := r.Update(ctx, p); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	defer func() {
		if err != nil {
			p.GetStatus().Ready = false
			p.GetStatus().Reason = err.Error()
			err = nil
		} else {
			p.GetStatus().Ready = true
			p.GetStatus().Reason = ""
		}
		if updateErr := r.Status().Update(ctx, p); updateErr != nil {
			logger.Error(updateErr, "Failed to update provider status")
		}
	}()

	newDnsProvider := func() error {
		dnsProvider, err := provider.New(ctx, p.GetSpec())
		if err != nil {
			return err
		}
		providerCache[p.GetUID()] = &provider.CachedDnsProvider{DNSProvider: dnsProvider, Generation: p.GetGeneration()}
		return nil
	}

	if !p.GetDeletionTimestamp().IsZero() {
		recordList := &dnsv1.RecordList{}
		if err := r.List(ctx, recordList, client.InNamespace(p.GetNamespace()), client.MatchingFields{providersField: req.String()}); err != nil {
			return ctrl.Result{}, err
		}
		canDelete := true
		for _, record := range recordList.Items {
			for _, provider := range record.Status.Providers {
				if provider.Namespace != p.GetNamespace() || provider.Name != p.GetName() {
					continue
				}
				if provider.RecordID != "" {
					canDelete = false
					break
				}
			}
		}
		if canDelete {
			removeFinalizer(p)
			if err := r.Update(ctx, p); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if dnsProvider := providerCache[p.GetUID()]; dnsProvider == nil || dnsProvider.Generation != p.GetGeneration() {
			err = newDnsProvider()
			if err != nil {
				return ctrl.Result{RequeueAfter: time.Second}, err
			}
		}
		return ctrl.Result{RequeueAfter: time.Second}, ErrorWaitRecords
	}

	err = newDnsProvider()
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProviderReconciler[T]) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.newer.New()).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
