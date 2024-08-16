package provider

import (
	"context"
	"errors"

	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
)

type ContextKey string

const (
	CtxKeyClient ContextKey = "CLIENT"
)

var ErrProviderNotFound = errors.New("provider not found")

var providers = make(map[dnsv1.ProviderType]DNSProviderFactory)

type DnsProviderPayload struct {
	Id     string            //in,out
	Data   string            //in,out
	Record *dnsv1.RecordSpec //in
}

type DNSProvider interface {
	Create(ctx context.Context, data *DnsProviderPayload) error
	Update(ctx context.Context, data *DnsProviderPayload) error
	Delete(ctx context.Context, data *DnsProviderPayload) error
}

type CachedDnsProvider struct {
	DNSProvider
	Generation int64
}

type DNSProviderFactory = func(ctx context.Context, provider dnsv1.ProviderObject) (DNSProvider, error)

func Register(providerType dnsv1.ProviderType, factory DNSProviderFactory) {
	providers[providerType] = factory
}

func New(ctx context.Context, provider dnsv1.ProviderObject) (DNSProvider, error) {
	factory, ok := providers[provider.GetSpec().Type]
	if !ok {
		return nil, ErrProviderNotFound
	}
	return factory(ctx, provider)
}

func NewPayload(status *dnsv1.RecordProviderStatus, record *dnsv1.RecordSpec) *DnsProviderPayload {
	return &DnsProviderPayload{
		Id:     status.RecordID,
		Data:   status.Data,
		Record: record,
	}
}
