# Scope K8s Proxy

The authentication proxy that [Scope](https://github.com/Altair-Hyun/scope) starts for each cluster.
It reads a kubeconfig context, signs requests with its credentials (tokens, certificates and exec
plugins such as `aws eks get-token`), and serves the Kubernetes API on a local port. It is a more
secure alternative to `kubectl proxy`, built on the kubectl proxy package.

Forked from [Open-Lens/lens-k8s-proxy](https://github.com/Open-Lens/lens-k8s-proxy).

## How Scope runs it

Configuration comes from environment variables:

| Variable | Meaning |
| --- | --- |
| `KUBECONFIG` | Path to the kubeconfig file |
| `KUBECONFIG_CONTEXT` | Context to use (the current context when empty) |
| `API_PREFIX` | Path prefix the API is served under |
| `PROXY_CERT`, `PROXY_KEY` | PEM certificate and key; serves TLS when both are set |

It listens on a free port on `127.0.0.1` and prints `starting to serve on 127.0.0.1:<port>`, which
Scope reads to find the port. `scope-k8s-proxy version` prints the build version as JSON.

## Build

```sh
VERSION=v1.0.0 make build
go test ./...
```

Pushing a `v*` tag builds macOS, Linux and Windows binaries (amd64 and arm64) with SHA-256 files and
publishes them as a GitHub release.

## License

Copyright (c) 2022 Mirantis, Inc.

Licensed under the [MIT license](./LICENSE).
