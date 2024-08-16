package job

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"

	dnsv1 "github.com/xzzpig/kube-dns-manager/api/dns/v1"
	"github.com/xzzpig/kube-dns-manager/internal/controller/dns"
	"github.com/xzzpig/kube-dns-manager/internal/controller/dns/provider"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	ErrClientNotFound  = errors.New("client not found in context")
	ErrClientNotClient = errors.New("client is not a client.Client")
	ErrJobRunning      = errors.New("job is running")
)

type JobProvider struct {
	namespace          string
	createTemplate     *template.Template
	updateTemplate     *template.Template
	deleteTemplate     *template.Template
	dataTemplate       *template.Template
	dataUpdateStrategy dnsv1.DataUpdateStrategy
}

type JobExecutePayload struct {
	*provider.DnsProviderPayload
	Action string
}

func getClient(ctx context.Context) (client.Client, error) {
	c := ctx.Value(provider.CtxKeyClient)
	if c == nil {
		return nil, ErrClientNotFound
	}
	if client, ok := c.(client.Client); ok {
		return client, nil
	} else {
		return nil, ErrClientNotClient
	}
}

func (p *JobProvider) executeJob(ctx context.Context, tpl *template.Template, payload *JobExecutePayload) (err error) {
	cli, err := getClient(ctx)
	if err != nil {
		return err
	}

	if payload.Id != "" {
		namespacedName := strings.Split(payload.Id, string(types.Separator))
		job := batchv1.Job{}
		err := cli.Get(ctx, types.NamespacedName{Namespace: namespacedName[0], Name: namespacedName[1]}, &job)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				return err
			}
		} else {
			// check job status
			if len(job.Status.Conditions) == 0 {
				return ErrJobRunning
			}
			if job.Status.Conditions[0].Type == "Complete" && job.Status.Conditions[0].Status == "True" {
				backgroundDeletion := metav1.DeletePropagationBackground
				if p.dataUpdateStrategy == dnsv1.DataUpdateStratagyOnComplete || p.dataUpdateStrategy == dnsv1.DataUpdateStratagyOnCompleteOrFailed {
					buffer := new(bytes.Buffer)
					if err := p.dataTemplate.Execute(buffer, payload); err != nil {
						return err
					}
					defer func() {
						if err == nil {
							payload.Data = buffer.String()
						}
					}()
				}
				if err := cli.Delete(ctx, &job, &client.DeleteOptions{PropagationPolicy: &backgroundDeletion}); err != nil {
					return err
				}
				return nil
			} else if job.Status.Conditions[0].Type == "Failed" && job.Status.Conditions[0].Status == "True" {
				if p.dataUpdateStrategy == dnsv1.DataUpdateStratagyOnCompleteOrFailed {
					buffer := new(bytes.Buffer)
					if err := p.dataTemplate.Execute(buffer, payload); err != nil {
						return err
					}
					payload.Data = buffer.String()
				}
				return fmt.Errorf("job failed, %s: %s", job.Status.Conditions[0].Reason, job.Status.Conditions[0].Message)
			} else {
				return ErrJobRunning
			}
		}
	}

	buffer := new(bytes.Buffer)
	if err := tpl.Execute(buffer, payload); err != nil {
		return err
	}

	job := batchv1.Job{}
	if err := yaml.Unmarshal(buffer.Bytes(), &job); err != nil {
		return err
	}

	if p.namespace != "" {
		job.Namespace = p.namespace
	} else if job.Namespace == "" {
		job.Namespace = os.Getenv("POD_NAMESPACE")
	}

	if p.dataUpdateStrategy == dnsv1.DataUpdateStrategyOnCreate {
		buffer = new(bytes.Buffer)
		if err := p.dataTemplate.Execute(buffer, payload); err != nil {
			return err
		}
	}

	if err := cli.Create(ctx, &job); err != nil {
		return err
	}

	if p.dataUpdateStrategy == dnsv1.DataUpdateStrategyOnCreate {
		payload.Data = buffer.String()
	}

	payload.Id = (&dnsv1.NamespacedName{Namespace: job.Namespace, Name: job.Name}).String()
	return ErrJobRunning
}

func (p *JobProvider) Create(ctx context.Context, payload *provider.DnsProviderPayload) (err error) {
	return p.executeJob(ctx, p.createTemplate, &JobExecutePayload{payload, "create"})
}

func (p *JobProvider) Update(ctx context.Context, payload *provider.DnsProviderPayload) (err error) {
	return p.executeJob(ctx, p.updateTemplate, &JobExecutePayload{payload, "update"})
}

func (p *JobProvider) Delete(ctx context.Context, payload *provider.DnsProviderPayload) (err error) {
	if err := p.executeJob(ctx, p.deleteTemplate, &JobExecutePayload{payload, "delete"}); err != nil {
		return err
	} else {
		payload.Id = ""
		payload.Data = ""
		return nil
	}
}

func init() {
	provider.Register(dnsv1.ProviderTypeJob, func(ctx context.Context, provider dnsv1.ProviderObject) (provider.DNSProvider, error) {
		spec := provider.GetSpec()
		p := new(JobProvider)
		p.namespace = provider.GetNamespace()
		p.dataUpdateStrategy = spec.Job.DataUpdateStrategy

		tpl := dns.NewTemplate(provider.GetName())
		if createTpl, err := tpl.New("create").Parse(string(spec.Job.CreateJobTemplate)); err != nil {
			return nil, err
		} else {
			p.createTemplate = createTpl
		}
		if spec.Job.UpdateJobTemplate == "" {
			p.updateTemplate = p.createTemplate
		} else if updateTpl, err := tpl.New("update").Parse(string(spec.Job.UpdateJobTemplate)); err != nil {
			return nil, err
		} else {
			p.updateTemplate = updateTpl
		}
		if spec.Job.DeleteJobTemplate == "" {
			p.deleteTemplate = p.createTemplate
		} else if deleteTpl, err := tpl.New("delete").Parse(string(spec.Job.DeleteJobTemplate)); err != nil {
			return nil, err
		} else {
			p.deleteTemplate = deleteTpl
		}
		if spec.Job.DataTemplate != "" {
			if dataTpl, err := tpl.New("data").Parse(string(spec.Job.DataTemplate)); err != nil {
				return nil, err
			} else {
				p.dataTemplate = dataTpl
			}
		}
		return p, nil
	})
}
