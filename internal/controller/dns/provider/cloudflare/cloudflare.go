package cloudflare

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
	"github.com/xzzpig/kube-dns-manager/internal/controller/dns/provider"
)

const (
	ExtraKeyProxied = "dns.xzzpig.com/cloudflare/proxied"
	ExtraKeyComment = "dns.xzzpig.com/cloudflare/comment"
	ExtraKeyTags    = "dns.xzzpig.com/cloudflare/tags"
)

type CloudflareProvider struct {
	api               *cloudflare.API
	zoneID            string
	matchExistsRecord bool
}

func (p *CloudflareProvider) find(ctx context.Context, record *dnsv1.RecordSpec) (id string, err error) {
	records, r, err := p.api.ListDNSRecords(ctx, cloudflare.ZoneIdentifier(p.zoneID), cloudflare.ListDNSRecordsParams{
		Name:    record.Name,
		Type:    string(record.Type),
		Content: record.Value,
	})
	if err != nil {
		return "", err
	}
	if r.Total == 0 {
		return "", nil
	}
	return records[0].ID, nil
}

func (p *CloudflareProvider) Create(ctx context.Context, record *dnsv1.RecordSpec) (id string, err error) {
	r, err := p.api.CreateDNSRecord(ctx, cloudflare.ZoneIdentifier(p.zoneID), cloudflare.CreateDNSRecordParams{
		Name:    record.Name,
		Type:    string(record.Type),
		Content: record.Value,
		TTL:     record.TTL,
		Proxied: record.ExtraBool(ExtraKeyProxied),
		Comment: record.Extra[ExtraKeyComment],
		Tags:    record.ExtraStrings(ExtraKeyTags),
	})
	if p.matchExistsRecord && IsRecordDuplicateError(err) {
		return p.find(ctx, record)
	}
	if err != nil {
		return "", err
	}
	return r.ID, nil
}

func (p *CloudflareProvider) Update(ctx context.Context, id string, record *dnsv1.RecordSpec) (newId string, err error) {
	r, err := p.api.UpdateDNSRecord(ctx, cloudflare.ZoneIdentifier(p.zoneID), cloudflare.UpdateDNSRecordParams{
		ID:      id,
		Name:    record.Name,
		Type:    string(record.Type),
		Content: record.Value,
		TTL:     record.TTL,
		Proxied: record.ExtraBool(ExtraKeyProxied),
		Comment: record.ExtraString(ExtraKeyComment),
		Tags:    record.ExtraStrings(ExtraKeyTags),
	})
	if _, ok := err.(*cloudflare.NotFoundError); ok {
		return p.Create(ctx, record)
	}
	if err != nil {
		return "", err
	}
	return r.ID, nil
}

func (p *CloudflareProvider) Delete(ctx context.Context, id string) (err error) {
	return p.api.DeleteDNSRecord(ctx, cloudflare.ZoneIdentifier(p.zoneID), id)
}

func IsRecordDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	if err, ok := err.(*cloudflare.RequestError); ok {
		if len(err.ErrorCodes()) == 0 {
			return false
		}
		for _, code := range err.ErrorCodes() {
			if code == 81058 {
				return true
			}
		}
	}
	return false
}

func init() {
	provider.Register(dnsv1.ProviderTypeCloudflare, func(ctx context.Context, spec *dnsv1.ProviderSpec) (provider.DNSProvider, error) {
		p := new(CloudflareProvider)
		if spec.Cloudflare.APIToken != "" {
			if api, err := cloudflare.NewWithAPIToken(spec.Cloudflare.APIToken); err != nil {
				return nil, err
			} else {
				p.api = api
			}
		} else if spec.Cloudflare.Key != "" && spec.Cloudflare.Email != "" {
			if api, err := cloudflare.New(spec.Cloudflare.Key, spec.Cloudflare.Email); err != nil {
				return nil, err
			} else {
				p.api = api
			}
		} else {
			return nil, fmt.Errorf("cloudflare provider requires either apiToken or key/email to be set")
		}

		zoneName := spec.Cloudflare.ZoneName
		if zoneName == "" {
			zoneName = spec.Selector.Domain
		}
		zoneID, err := p.api.ZoneIDByName(zoneName)
		if err != nil {
			return nil, err
		}
		p.zoneID = zoneID

		p.matchExistsRecord = spec.Cloudflare.MatchExistsRecord

		return p, nil
	})
}
