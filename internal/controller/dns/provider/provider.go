package provider

import (
	"context"
	"errors"

	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
)

var ErrProviderNotFound = errors.New("provider not found")

var providers = make(map[dnsv1.ProviderType]DNSProviderFactory)

type DNSProvider interface {
	Create(ctx context.Context, record *dnsv1.RecordSpec) (id string, err error)
	Update(ctx context.Context, id string, record *dnsv1.RecordSpec) (newId string, err error)
	Delete(ctx context.Context, id string) (err error)
}

type CachedDnsProvider struct {
	DNSProvider
	Generation int64
}

type DNSProviderFactory = func(ctx context.Context, spec *dnsv1.ProviderSpec) (DNSProvider, error)

func Register(providerType dnsv1.ProviderType, factory DNSProviderFactory) {
	providers[providerType] = factory
}

func New(ctx context.Context, spec *dnsv1.ProviderSpec) (DNSProvider, error) {
	factory, ok := providers[spec.Type]
	if !ok {
		return nil, ErrProviderNotFound
	}
	return factory(ctx, spec)
}
