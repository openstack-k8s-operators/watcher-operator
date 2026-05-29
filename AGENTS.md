# AGENTS.md - watcher-operator

## Project overview

watcher-operator is a Kubernetes operator that manages
[OpenStack Watcher](https://docs.openstack.org/watcher/latest/)
on OpenShift/Kubernetes. It is part of the
[openstack-k8s-operators](https://github.com/openstack-k8s-operators) project.

## Tech stack

| Layer | Technology |
|-------|------------|
| Language | Go (modules, multi-module workspace via `go.work`) |
| Scaffolding | [Kubebuilder v4](https://book.kubebuilder.io/) + [Operator SDK](https://sdk.operatorframework.io/) |
| CRD generation | controller-gen (DeepCopy, CRDs, RBAC, webhooks) |
| Config management | Kustomize |
| Packaging | OLM bundle |
| Testing | Ginkgo/Gomega + envtest (functional), KUTTL (integration) |
| Linting | golangci-lint (`.golangci.yaml`) |
| CI | Zuul (`zuul.d/`), Prow (`.ci-operator.yaml`), GitHub Actions |

## Directory structure

**Maintenance rule:** when directories are added, removed, or renamed, or when their purpose changes, update this table to match.

| Directory | Contents |
|-----------|----------|
| `api/v1beta1/` | CRD types (`watcher.openstack.org/v1beta1`), conditions, webhook markers |
| `internal/controller/` | Reconcilers for Watcher, WatcherAPI, WatcherApplier, WatcherDecisionEngine |
| `internal/watcher/` | Watcher resource-building helpers |
| `internal/watcherapi/` | WatcherAPI resource-building helpers |
| `internal/watcherapplier/` | WatcherApplier resource-building helpers |
| `internal/watcherdecisionengine/` | WatcherDecisionEngine resource-building helpers |
| `internal/webhook/` | Webhook implementations |
| `templates/` | Config templates per service, mounted via `OPERATOR_TEMPLATES` |
| `test/functional/` | envtest-based Ginkgo/Gomega tests |
| `test/kuttl/` | KUTTL integration tests |

## Build commands

After modifying Go code, always run: `make generate manifests fmt vet`.

## Code style guidelines

- Follow standard openstack-k8s-operators conventions and lib-common patterns.
- Use `lib-common` modules for conditions, endpoints, TLS, storage, and other
  cross-cutting concerns rather than re-implementing them.
- CRD types live in `api/v1beta1/`.
- Controller logic goes in `internal/controller/`.
  Resource-building helpers go in `internal/<service>/` packages.
- Config templates are split per service under `templates/` and mounted at
  runtime via the `OPERATOR_TEMPLATES` environment variable.
- Webhook logic is split between the kubebuilder markers in `api/` and
  the implementation in `internal/webhook/`.

## Testing

- Functional tests use the envtest framework with Ginkgo/Gomega and live in
  `test/functional/`.
- KUTTL integration tests live in `test/kuttl/`.
- Run all functional tests: `make test`.
- When adding a new field or feature, add corresponding test cases in
  `test/functional/` and update fixture data accordingly.

## Key dependencies

- [lib-common](https://github.com/openstack-k8s-operators/lib-common): shared modules for conditions, endpoints, database, TLS, secrets, etc.
- [infra-operator](https://github.com/openstack-k8s-operators/infra-operator): RabbitMQ and topology APIs.
- [mariadb-operator](https://github.com/openstack-k8s-operators/mariadb-operator): database provisioning.
- [keystone-operator](https://github.com/openstack-k8s-operators/keystone-operator): identity service registration.
- [dev-docs](https://github.com/openstack-k8s-operators/dev-docs): shared development guide covering operator conventions, debugging, testing, and best practices.
- [Documentation](docs/main.adoc): install guide and user workflows.
