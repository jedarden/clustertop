# clustertop

A terminal dashboard that polls every cluster in the fleet's read-only
`kubectl-proxy` endpoint and shows live node status — Ready/NotReady, roles,
node pool/type, autoscaler taints, Kubernetes version, and age — refreshing
automatically and re-flowing to fit whatever terminal it's running in.

This repo contains the Go source for the `clustertop` binary, the static
`clusters.yaml` endpoint config, the GoReleaser config used by its CI pipeline,
and the project's documentation. See `docs/plan/plan.md` for the full plan,
`docs/research/` for the research notes backing its decisions, and
`docs/notes/` for feature-specific detail as it accumulates.

## Obtaining the binary

Prebuilt archives (`linux`/`darwin` × `amd64`/`arm64`, plus `checksums.txt`)
are attached to each tag's release at
[jedarden/clustertop/releases](https://github.com/jedarden/clustertop/releases).
CI cuts a release automatically when a `v*` tag is pushed — see
[CI](#ci) below.

With a Go 1.26+ toolchain you can install instead:

```bash
go install github.com/jedarden/clustertop@latest   # or pin: @v0.3.1
```

Or build from source:

```bash
git clone https://git.ardenone.com/jedarden/clustertop.git
cd clustertop
go build -o clustertop .
```

## Running

```bash
clustertop                                   # the TUI dashboard
clustertop sync-clusters \
  --declarative-config-path ~/declarative-config   # regenerate clusters.yaml
```

`clustertop` reads `clusters.yaml` from the **current working directory** —
there is no flag or environment variable that changes the path. To run against
a different endpoint list, `cd` to (or symlink `clusters.yaml` into) a
directory containing the file you want and launch from there. Run
`clustertop` with no `clusters.yaml` present and it exits immediately with the
load error rather than starting an empty dashboard.

Each cluster renders as a bordered section: a node grid with Ready/NotReady
state, roles, instance type (pool), kubelet version, and age, plus a warning
marker for on-demand pools and scale-down-tainted nodes. The footer shows the
current keybindings.

## Configuration — clusters.yaml

```yaml
clusters:
  - name: iad-ci
    endpoint: http://traefik-iad-ci.tail1b1987.ts.net:8001
    route: traefik-kubectl-tcp
    notes: optional free text; carried but never rendered
```

| Field | Meaning |
|---|---|
| `name` | Section heading in the UI, and the key fetch results are routed back by — keep values unique |
| `endpoint` | Base URL of the cluster's read-only kubectl-proxy Service |
| `route` | How the endpoint is exposed: `traefik-kubectl-tcp` or `direct-tailscale-operator` |
| `notes` | Optional; parsed but unused by the binary |

Unknown keys are ignored (lenient decode), so a stale field never breaks an
already-released binary. An empty `clusters:` list is the only fatal config
error — there would be nothing to display.

Don't maintain this list by hand: regenerate it from the declarative-config
checkout with `sync-clusters` so it can't silently drift.

## sync-clusters

`sync-clusters` rebuilds `clusters.yaml` by scanning a
[`declarative-config`](https://git.ardenone.com/jedarden/declarative-config)
checkout — the same manifests ArgoCD applies — instead of trusting a
hand-edited list (see `docs/research/sync-clusters-source-manifests.md` for
why).

```bash
clustertop sync-clusters \
  --declarative-config-path ~/declarative-config \
  --out clusters.yaml        # this is the default
```

`--declarative-config-path` is required. For each directory under
`<path>/k8s/` it recognizes two manifest shapes:

1. `devpod-observer/kubectl-proxy.yml` — first Service in the file carries a
   `tailscale.com/hostname` annotation → emits
   `http://<host>.tail1b1987.ts.net:8001` with route
   `direct-tailscale-operator`.
2. otherwise `traefik/tailscale-service.yml` — Service carries that annotation
   **and** a port named `kubectl-tcp` → same URL form, route
   `traefik-kubectl-tcp`.

Behavior at the edges:

- Directories with a kubectl-proxy manifest but no recognized hostname
  annotation are skipped with a logged warning.
- Directories with no kubectl-proxy manifest at all are skipped silently —
  not every `k8s/` subdirectory is a cluster (e.g. `argocd/` is a loose
  Application file).
- Zero clusters resolved is a hard error; the existing `clusters.yaml` is
  left untouched.

Commit the regenerated file — it's the endpoint list the next release ships
with.

## Keybindings

| Key | Action |
|---|---|
| `r` | Refresh all clusters now |
| `q` / `ctrl+c` | Quit |
| `↑` `↓` `pgup` `pgdown` | Scroll when the grid is taller than the terminal |

Any other key is forwarded to the viewport, which also honors its own default
scroll keys (`j`/`k`, `g`/`G`, half/full-page jumps). The footer always shows
the short help.

## Refresh behavior

- All clusters are fetched concurrently at startup and again every **15s**.
- Each cluster's fetch is bounded by its own **5s** timeout and runs on its
  own goroutine, so a hung endpoint only delays its own section.
- A failed cluster flips to `UNREACHABLE` in its section while the rest keep
  updating — fault isolation is per cluster, by design.
- The last successful node grid stays visible through a failure, titled
  `stale <n>m ago`, rather than blanking. It's replaced at the next success,
  so a flaky cluster degrades to stale data instead of disappearing.
- Before the first fetch completes a cluster shows `— connecting…`, keeping a
  cold start visibly distinct from an incident.
- The 15s refresh interval and 5s fetch timeout are compile-time constants
  (`internal/ui/run.go`), not runtime options.
- Resizing the terminal re-flows the grid immediately.

## Read-only endpoint requirements

Every `endpoint` must serve `GET /api/v1/nodes` unauthenticated over plain
HTTP. That is exactly what the fleet's `kubectl-proxy` Deployments (namespace
`devpod-observer`) provide: a ServiceAccount whose RBAC is read-only by
construction, exposed on the tailnet and protected by the tailnet ACL rather
than by a credential.

- clustertop has no auth, TLS, or kubeconfig support — adding any would imply
  a higher-privilege access path exists. It doesn't. A cluster that requires
  a token to read nodes is out of scope, not a missing feature.
- Endpoints are `tag:k8s` tailnet hosts. Reaching them requires tailnet
  membership and an ACL rule permitting your device. Every cluster showing
  `UNREACHABLE` at once usually means you're off the tailnet, not that the
  fleet is down.
- Any response other than an HTTP 200 NodeList JSON body renders as an error
  for that cluster.

## CI

CI runs on **Argo Workflows in `iad-ci`** (GitHub Actions are disabled
org-wide; the pipeline is modeled on domain-check's — see
`docs/research/ci-pipeline-pattern.md`):

- **Push to `main`** → `quality-gate`: `go vet ./...` and `go test -race ./...`
- **Push a `v*` tag** → quality gate, then GoReleaser publishes the release
  artifacts to GitHub Releases

Run the gate locally before pushing — a failing gate on `main` is a broken
build for anyone who pulls.
