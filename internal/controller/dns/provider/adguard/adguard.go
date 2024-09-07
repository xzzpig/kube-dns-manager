package adguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
	"github.com/xzzpig/kube-dns-manager/internal/controller/dns/provider"
)

var (
	ErrRequestFailed = errors.New("request failed")
)

type AdguardProvider struct {
	url url.URL
}

type AdguardRecord struct {
	Domain string `json:"domain"`
	Answer string `json:"answer"`
}

func (p *AdguardProvider) List(ctx context.Context) (records []AdguardRecord, err error) {
	url, err := p.url.Parse("control/rewrite/list")
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Join(ErrRequestFailed, errors.New(string(body)))
	}
	err = json.Unmarshal(body, &records)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (p *AdguardProvider) create(record *AdguardRecord) (err error) {
	url, err := p.url.Parse("control/rewrite/add")
	if err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	resp, err := http.Post(url.String(), "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Join(ErrRequestFailed, errors.New(string(body)))
	}
	return nil
}

func (p *AdguardProvider) Create(ctx context.Context, payload *provider.DnsProviderPayload) (err error) {
	record := AdguardRecord{Domain: payload.Record.Name, Answer: payload.Record.Value}
	id, err := json.Marshal(record)
	if err != nil {
		return err
	}
	err = p.create(&record)
	if err != nil {
		return err
	}
	payload.Id = string(id)
	return nil
}

func (p *AdguardProvider) Update(ctx context.Context, payload *provider.DnsProviderPayload) (err error) {
	if payload.Id != "" {
		oldRecord := AdguardRecord{}
		err = json.Unmarshal([]byte(payload.Id), &oldRecord)
		if err != nil {
			return err
		}
		if oldRecord.Domain == payload.Record.Name && oldRecord.Answer == payload.Record.Value {
			return nil
		}
		if err = p.delete(&oldRecord); err != nil {
			return err
		}
	}
	err = p.Create(ctx, payload)
	return err
}

func (p *AdguardProvider) delete(record *AdguardRecord) (err error) {
	url, err := p.url.Parse("control/rewrite/delete")
	if err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	resp, err := http.Post(url.String(), "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Join(ErrRequestFailed, errors.New(string(body)))
	}
	return nil
}

func (p *AdguardProvider) Delete(ctx context.Context, payload *provider.DnsProviderPayload) (err error) {
	record := AdguardRecord{Domain: payload.Record.Name, Answer: payload.Record.Value}
	err = p.delete(&record)
	if err != nil {
		return err
	}
	payload.Id = ""
	payload.Data = ""
	return nil
}

func init() {
	provider.Register(dnsv1.ProviderTypeAdguard, func(ctx context.Context, provider dnsv1.ProviderObject) (provider.DNSProvider, error) {
		spec := provider.GetSpec()
		p := new(AdguardProvider)
		u, err := url.Parse(spec.Adguard.URL)
		if err != nil {
			return nil, err
		}
		if provider.GetSpec().Adguard.Username != "" {
			u.User = url.UserPassword(provider.GetSpec().Adguard.Username, provider.GetSpec().Adguard.Password)
		}
		p.url = *u
		return p, nil
	})
}
