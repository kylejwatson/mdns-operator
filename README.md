# mDNS Operator (Go)

This operator makes it easier to set up local domain names for services on a private network using mDNS, so Kubernetes-managed workloads can advertise simple hostnames without manual DNS configuration.

## What it does

- Watches all `Gateway` resources in `gateway.networking.k8s.io/v1`.
- Reads `mdns.alpha.kubernetes.io/hostname` annotation.
- Reads `status.addresses` for the first `IPAddress` value.
- Publishes or withdraws `.local` hostname records for the Gateway IP.
- Answers hostname `A` and `AAAA` queries directly over mDNS multicast.

If the annotation is removed, the Gateway is deleted, or there is no `IPAddress` status address, the mDNS record is unpublished.

## Project layout

- `cmd/mdns-operator/main.go`: manager startup, health probes, leader election.
- `internal/controller/gateway_watcher.go`: Gateway watch loop and object parsing.
- `internal/mdns/publisher.go`: mDNS publish/unpublish lifecycle with retries.
- `config/default`: top-level kustomize entrypoint for installation.
- `config/rbac`: ServiceAccount, ClusterRole, ClusterRoleBinding.
- `config/manager`: controller Deployment.

## Local build

```bash
go mod tidy
go build ./...
```

## Build and push controller image

Set your own registry path before deploying.

```bash
export IMG=ghcr.io/<your-org>/mdns-operator:latest
docker build -t "$IMG" -f Dockerfile .
docker push "$IMG"
```

If you are using another build system (for example `ko` or `buildx`), publish the final image tag you want and set it in `config/manager/kustomization.yaml`.

## Deploy to Kubernetes

1. Update the image override in `config/manager/kustomization.yaml` to your published image name and tag.
2. Install manifests:

```bash
kubectl apply -k config/default
```

3. Confirm rollout:

```bash
kubectl -n mdns-operator-system get deploy,pods
kubectl -n mdns-operator-system logs deploy/mdns-operator-controller-manager -f
```

## Example Gateway with mDNS annotation

This annotation is what enables publication.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
	name: demo-gateway
	namespace: default
	annotations:
		mdns.alpha.kubernetes.io/hostname: demo-home.local
spec:
	gatewayClassName: your-gateway-class
	listeners:
		- name: http
			protocol: HTTP
			port: 80
			allowedRoutes:
				namespaces:
					from: All
```

Notes:

- The operator only publishes once `status.addresses` contains an entry with `type: IPAddress`.
- The hostname is normalized to end in `.local` if it does not already.

## Verify published records

After the Gateway controller assigns an IP to `status.addresses`, confirm the node answers direct hostname lookups on your LAN:

```bash
dns-sd -Q demo-home.local A
dns-sd -Q demo-home.local AAAA
```

## Troubleshooting mDNS visibility

- mDNS is link-local multicast (UDP 5353 to `224.0.0.251` / `ff02::fb`) and is usually not routed across Kubernetes overlay networks.
- The provided manager manifest enables `hostNetwork: true` so announcements are sent from the node network namespace.
- If service browsing still shows no entries, verify your LAN/firewall allows multicast and that client and node are on the same L2 broadcast domain/VLAN.
- The operator listens for mDNS traffic in the node network namespace and answers queries from the local multicast domain.

## Uninstall

```bash
kubectl delete -k config/default
```
