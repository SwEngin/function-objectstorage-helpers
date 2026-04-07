# function-objectstorage-helpers

A [Crossplane Composition Function](https://docs.crossplane.io/v2.1/guides/write-a-composition-function-in-go/) that computes shared derived values from the observed composite resource (XR) and writes them to the pipeline context. Downstream template steps consume these values instead of recomputing them inline.

## Overview

This function runs as an **early pipeline step** that enriches the pipeline context before any template steps execute:

- **This step** — derives values once from the observed XR and composed resources, writes them to the `swengin.io/helpers` context key
- **Downstream template steps** — read values via `{{ $h := index .context "swengin.io/helpers" }}` instead of recomputing them inline

### Values written to context

| Key | Type | Description |
|-----|------|-------------|
| `saExtName` | `string` | Azure-safe storage account name suffix derived from XR name + UID (max 24 chars, lowercase alphanumeric) |
| `isSAReady` | `bool` | `true` when the `storage-account` composed resource has `Ready=True` |
| `primaryBlobEndpoint` | `string` | Azure Storage Account primary blob endpoint from `status.atProvider` |
| `rookReady` | `bool` | `true` when the CephCluster observer reports `HEALTH_OK` or `HEALTH_WARN` |
| `obcReady` | `bool` | `true` when the Ceph `ObjectBucketClaim` has reached phase `Bound` |

## Usage in a composition pipeline

```yaml
pipeline:
  - step: helpers
    functionRef:
      name: function-objectstorage-helpers

  - step: render-storage-account
    functionRef:
      name: function-go-templating
    input:
      inline:
        template: |
          {{- $h := index .context "swengin.io/helpers" }}
          apiVersion: storage.azure.upbound.io/v1beta2
          kind: Account
          metadata:
            name: {{ $h.saExtName }}
          ...
          {{- if $h.isSAReady }}
          # gate-dependent resources
          {{- end }}
```

## Required RBAC

This function queries the Kubernetes API server at runtime — not through a provider — so the function's ServiceAccount needs explicit permissions. Crossplane creates the ServiceAccount in `crossplane-system` for each `FunctionRevision`.

The binding might use `system:serviceaccounts:crossplane-system` as the subject so it covers all present and future FunctionRevision SAs automatically.

### Permissions needed

| API Group | Resource | Verb | Purpose |
|-----------|----------|------|---------|
| `azure.m.upbound.io` | `clusterproviderconfigs` | `get` | Check Azure provider is configured |
| `ceph.rook.io` | `cephclusters` | `list` | Read CephCluster health status |

> **Note:** Without this RBAC, `rookReady`, `azureProviderExists`, and `kubernetesProviderExists` will silently return `false`, causing compositions to emit no resources.

## Install

Apply the `Function` manifest pointing to the versioned image:

```yaml
apiVersion: pkg.crossplane.io/v1beta1
kind: Function
metadata:
  name: function-objectstorage-helpers
spec:
  package: ghcr.io/swengin/function-objectstorage-helpers:v0.1.0
  # if the repo is private:
  # packagePullSecrets:
  #   - name: ghcr-pull-secret
```

Create a pull secret if the package repository is private:

```bash
kubectl create secret docker-registry ghcr-pull-secret \
  -n crossplane-system \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<PAT-with-read:packages-scope>
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
