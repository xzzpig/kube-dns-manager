package alidns

import (
	"context"
	"fmt"
	"strings"

	alidns "github.com/alibabacloud-go/alidns-20150109/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	"github.com/alibabacloud-go/tea/tea"
	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
	"github.com/xzzpig/kube-dns-manager/internal/controller/dns/provider"
)

const (
	ExtraKeyLine = "dns.xzzpig.com/alidns/line"
)

type AliyunDNSProvider struct {
	domainName string
	client     *alidns.Client
}

func (p *AliyunDNSProvider) getRR(domain string) *string {
	if domain == p.domainName {
		rr := ""
		return &rr
	}
	rr := strings.TrimSuffix(domain, "."+p.domainName)
	return &rr
}

func (p *AliyunDNSProvider) getType(recordType dnsv1.RecordType) *string {
	t := string(recordType)
	return &t
}

func (p *AliyunDNSProvider) getTTL(ttl int) *int64 {
	if ttl == 0 {
		return nil
	}
	t := int64(ttl)
	return &t
}

func (p *AliyunDNSProvider) find(record *dnsv1.RecordSpec) (string, error) {
	result, err := p.client.DescribeDomainRecords(&alidns.DescribeDomainRecordsRequest{
		DomainName:   &p.domainName,
		RRKeyWord:    p.getRR(record.Name),
		TypeKeyWord:  p.getType(record.Type),
		ValueKeyWord: &record.Value,
	})
	if err != nil {
		return "", err
	}
	if *result.Body.TotalCount == 0 {
		return "", nil
	}
	return *result.Body.DomainRecords.Record[0].RecordId, nil
}

func (p *AliyunDNSProvider) Create(ctx context.Context, record *dnsv1.RecordSpec) (string, error) {
	result, err := p.client.AddDomainRecord(&alidns.AddDomainRecordRequest{
		DomainName: &p.domainName,
		RR:         p.getRR(record.Name),
		Type:       p.getType(record.Type),
		Value:      &record.Value,
		TTL:        p.getTTL(record.TTL),
		Line:       record.ExtraString(ExtraKeyLine),
	})
	if IsRecordDuplicateError(err) {
		if id, err := p.find(record); err != nil {
			return "", err
		} else if id != "" {
			return id, nil
		}
	}
	if err != nil {
		return "", err
	}
	return *result.Body.RecordId, nil
}

func (p *AliyunDNSProvider) Update(ctx context.Context, id string, record *dnsv1.RecordSpec) (newId string, err error) {
	result, err := p.client.UpdateDomainRecord(&alidns.UpdateDomainRecordRequest{
		RecordId: &id,
		RR:       p.getRR(record.Name),
		Type:     p.getType(record.Type),
		Value:    &record.Value,
		TTL:      p.getTTL(record.TTL),
		Line:     record.ExtraString(ExtraKeyLine),
	})
	if IsReccordNotFoundError(err) {
		return p.Create(ctx, record)
	}
	if IsRecordDuplicateError(err) {
		return id, nil
	}
	if err != nil {
		return "", err
	}
	return *result.Body.RecordId, nil
}

func (p *AliyunDNSProvider) Delete(ctx context.Context, id string) (err error) {
	_, err = p.client.DeleteDomainRecord(&alidns.DeleteDomainRecordRequest{
		RecordId: &id,
	})
	return err
}

func IsReccordNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if sdkError, ok := err.(*tea.SDKError); ok {
		return *sdkError.Code == "DomainRecordNotBelongToUser"
	}
	return false
}

func IsRecordDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	if sdkError, ok := err.(*tea.SDKError); ok {
		return *sdkError.Code == "DomainRecordDuplicate"
	}
	return false
}

func init() {
	provider.Register(dnsv1.ProviderTypeAliyun, func(ctx context.Context, spec *dnsv1.ProviderSpec) (provider.DNSProvider, error) {
		p := new(AliyunDNSProvider)

		if spec.Aliyun.DomainName != "" {
			p.domainName = spec.Aliyun.DomainName
		} else if spec.Selector.Domain != "" {
			p.domainName = spec.Selector.Domain
		} else {
			return nil, fmt.Errorf("aliyun DNS provider requires domain name")
		}

		if spec.Aliyun.Endpoint == "" {
			spec.Aliyun.Endpoint = "dns.aliyuncs.com"
		}

		if client, err := alidns.NewClient(&openapi.Config{
			AccessKeyId:     &spec.Aliyun.AccessKeyID,
			AccessKeySecret: &spec.Aliyun.AccessKeySecret,
			Endpoint:        &spec.Aliyun.Endpoint,
		}); err != nil {
			return nil, err
		} else {
			p.client = client
		}

		// Check if domain exists
		if _, err := p.client.DescribeDomainInfo(&alidns.DescribeDomainInfoRequest{DomainName: &p.domainName}); err != nil {
			return nil, err
		}

		return p, nil
	})
}
