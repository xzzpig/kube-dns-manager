# kube-dns-manager
Manage DNS Record in kubernetes.

## Description
Features:
- Generate DNS Record by kubernetes resources, eg. Ingress, Service, Node
- Sync DNS Record to DNS Providers, eg. alidns, cloudflare

## Getting Started
### Installation
#### Helm
1. Run the following command to add the chart repository first:
```sh
helm repo add kube-dns-manager https://xzzpig.github.io/kube-dns-manager/
helm repo update
```
2. Install the chart:
```sh
helm install kube-dns-manager kube-dns-manager/kube-dns-manager --namespace kube-dns-manager --create-namespace
```
#### Bundles
```sh
kubectl apply -f https://raw.githubusercontent.com/xzzpig/kube-dns-manager/main/dist/install.yaml
```

### Uninstall
#### Helm
```sh
helm uninstall kube-dns-manager --namespace kube-dns-manager
```
#### Bundles
```sh
kubectl delete -f https://raw.githubusercontent.com/xzzpig/kube-dns-manager/main/dist/install.yaml --wait
```

### Model
![Model](model.drawio.svg)

### Configure
1. Create a 
[Template](config/samples/dns_v1_clustertemplate.yaml)/[ClusterTemplate](config/samples/dns_v1_template.yaml). This sample template is used for Ingress, and is a `Record` template with 
- label:`dns.xzzpig.com/scope: public`
- domain: hosts in the Ingress
- type: CNAME
- value: sample.sample.com
- extra: a comment if `Provider` is cloudflare
2. Create a [Generator](config/samples/dns_v1_generator.yaml)/[ClusterGenerator](config/samples/dns_v1_clustergenerator.yaml) to generate DNS Record by kubernetes resources. This samele generator will match `public` Ingress and create a `ResourceWatcher` to watch the changes of the Ingress which is used in the `Template`(If other resources are used in the `Template`, they will also be watched by the `ResourceWatcher`). Then the `ResourceWatcher` will generate DNS `Record` via the `Template`.
3. Create a [Provider](config/samples/dns_v1_provider.yaml)/[ClusterProvider](config/samples/dns_v1_clusterprovider.yaml). This samele provider will match any `Record` with label `dns.xzzpig.com/scope: public` and domain is `sample.com` and then sync to DNS Providers.

## License

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

