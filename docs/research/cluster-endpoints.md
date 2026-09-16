# Which clusters exist and how does clustertop reach each one's nodes?

**Type:** Behavioural + Measurement

## What I found

`ls k8s/` on `declarative-config`'s `main` branch (run directly, 2026-07-31):

```
$ ls k8s/
apexalgo-iad  ardenone-cluster  ardenone-manager  argocd  capacity-check.sh
CLAUDE.md  iad-ci  iad-kalshi  iad-options  node-labeling.md  ord-devimprint
openbao-dr-runbook.md  rs-manager  volsync-restore-runbook.md
```

8 cluster directories (`argocd/` is a loose Application file, not a cluster).
`k8s/CLAUDE.md`'s own cluster table lists a 9th, `ardenone-hub` — confirmed
stale, see `ardenone-hub-decommission.md`.

A dedicated research agent (background task, this session) cross-checked every
one of the 8 directories for a `devpod-observer/kubectl-proxy.yml` and the
Service/IngressRouteTCP that exposes it. Result, verbatim from that pass:

| Cluster | Proxy manifest | Reachable at | Route |
|---|---|---|---|
| apexalgo-iad | `k8s/apexalgo-iad/devpod-observer/kubectl-proxy.yml` | `traefik-apexalgo-iad.tail1b1987.ts.net:8001` | Traefik `kubectl-tcp` |
| ardenone-cluster | `k8s/ardenone-cluster/devpod-observer/kubectl-proxy.yml` | `traefik-ardenone-cluster.tail1b1987.ts.net:8001` | Traefik `kubectl-tcp` |
| ardenone-manager | `k8s/ardenone-manager/devpod-observer/kubectl-proxy.yml` | `traefik-ardenone-manager.tail1b1987.ts.net:8001` | Traefik `kubectl-tcp` |
| iad-ci | `k8s/iad-ci/devpod-observer/kubectl-proxy.yml` | `traefik-iad-ci.tail1b1987.ts.net:8001` | Traefik `kubectl-tcp` |
| iad-kalshi | `k8s/iad-kalshi/devpod-observer/kubectl-proxy.yml` | `kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001` | Direct Tailscale operator (no Traefik on this cluster) |
| iad-options | `k8s/iad-options/devpod-observer/kubectl-proxy.yml` | `traefik-iad-options.tail1b1987.ts.net:8001` | Traefik `kubectl-tcp` |
| ord-devimprint | `k8s/ord-devimprint/devpod-observer/kubectl-proxy.yml` | `kubectl-proxy-ord-devimprint.tail1b1987.ts.net:8001` | Direct Tailscale operator |
| rs-manager | `k8s/rs-manager/devpod-observer/kubectl-proxy.yml` | `traefik-rs-manager.tail1b1987.ts.net:8001` | Traefik `kubectl-tcp` |

I independently verified the manifest mechanism itself by reading
`k8s/iad-options/devpod-observer/kubectl-proxy.yml` directly:

```yaml
containers:
  - name: kubectl-proxy
    image: alpine/k8s:1.31.3
    command: [kubectl, proxy, --port=8001, --address=0.0.0.0,
              --accept-hosts=.*, --accept-paths=.*]
```

This is a bare `kubectl proxy` process using the `devpod-observer`
ServiceAccount's mounted credentials — it does not require a bearer token or
TLS from the client. A plain `GET http://<host>:8001/api/v1/nodes` is
sufficient. `CLAUDE.md` at the repo root independently documents the same
pattern for read access on every cluster (e.g. `kubectl
--server=http://traefik-apexalgo-iad:8001 get pods -n <namespace>`, no
`--token` flag).

RBAC: `iad-ci`, `iad-kalshi`, and `iad-options` (all rs-manager-managed)
omit `secrets` from their ClusterRole entirely. The rest grant list-only on
secrets. `ord-devimprint` additionally has a separate, more-privileged
`secret-reader` SA scoped to its own namespace — irrelevant here, since this
tool never touches secrets.

## What this means for design

- `clusters.yaml` can hardcode all 8 endpoints as bare `http://` URLs — no
  token/cert fields belong in the config schema at all, and adding them later
  would signal a design that doesn't match how these endpoints actually work.
- `iad-kalshi` and `ord-devimprint` use a different hostname pattern
  (`kubectl-proxy-<cluster>` vs `traefik-<cluster>`) — this is exactly why the
  config needs an explicit endpoint per cluster rather than a
  `templated-from-name` convention; the two routing styles aren't
  interchangeable.

## Live verification (2026-09-16, codinghome)

Ran the full playbook (`plan.md` §7) against the real fleet: `curl` + `jq` per
endpoint, then the built binary in tmux (220×120, three 15s refresh cycles),
plus the negative fault-isolation test and the narrow-layout check. This
closes the "manifest reading, not a live connectivity test" caveat above —
and it caught real drift, so the caveat was justified.

### Reachability + decode results

All 8 endpoints answered `HTTP 200` with `kind: NodeList` / `apiVersion: v1`,
and every cluster rendered a full Ready node grid through the binary's own
`k8sclient.FetchNodes` decode path (a decode failure would have collapsed
that cluster's section to an error line):

| Cluster | Nodes | Solo fetch total | Notes |
|---|---|---|---|
| apexalgo-iad | 3 | 0.80s | |
| ardenone-cluster | 7 | 0.71s | largest body, 258KB |
| ardenone-manager | 1 | 0.34s | |
| iad-ci | 6 | **6.5–9.7s** | see failure #2 |
| iad-kalshi | 2 | 0.46s | |
| iad-options | 3 | 0.58s | |
| ord-devimprint | 4 | 0.67s | healthy only after fix #1 |
| rs-manager | 3 | 0.47s | |

### Failure #1 — `clusters.yaml` had a dead endpoint for ord-devimprint (fixed)

`http://kubectl-proxy-ord-devimprint.tail1b1987.ts.net:8001` stopped
resolving (NXDOMAIN). Root cause: `declarative-config` commit `ab028563`
(2026-08-22, "feat(ord-devimprint): add Traefik Tailscale service exposure")
moved the cluster from direct-Tailscale-operator exposure to Traefik-routed
(`traefik-ord-devimprint`, port `kubectl-tcp` 8001); `clusters.yaml` predated
that change and was never regenerated. `go run . sync-clusters` against a
current `declarative-config` checkout produced exactly the one-entry diff
(endpoint + route), which is what's checked in now. The old direct-exposure
shape was verified live *only* through its failure: the pre-fix hostname
NXDOMAIN'd, everything else about the cluster was healthy.

Playbook implication: `sync-clusters` diffing (`plan.md` §7 Phase 5) is not
just a scanner-bug check — it is the drift detector for this file, and it
should be run before trusting a stale checkout's `clusters.yaml`.

### Failure #2 — iad-ci is consistently slower than the 5s fetch timeout (recorded, not changed)

iad-ci's `/api/v1/nodes` took **6.5s / 7.9s / 9.0s / 9.7s** across four
sequential solo samples (153KB, chunked; TTFB ~0.25s — the body trickles).
`defaultFetchTimeout` is 5s, so iad-ci flapped `UNREACHABLE — decode nodelist:
context deadline exceeded` on roughly half of the observed refresh cycles,
recovering on the next. Every other cluster completes in under a second solo;
under the 8-way concurrent cold start, one or two clusters (apexalgo-iad,
ord-devimprint seen) could also briefly exceed 5s, but always recovered by the
next cycle. Fault isolation behaved exactly as designed throughout: only the
timed-out cluster's section degraded; the other seven stayed fresh.

This is the data half of the `plan.md` §6 open question ("per-cluster fetch
timeout tuning — adjust after running against the real fleet"). The remedy is
deliberately *not* applied here — that's a design decision (raise
`defaultFetchTimeout` vs. a per-cluster `timeout:` field in `clusters.yaml`,
since 7/8 clusters need <1s and only iad-ci needs more). The slowdown's cause
(proxy sidecar, API server, or network path) was not diagnosed from here.

### Negative tests (both passed)

- **Fault isolation:** with `iad-kalshi` pointed at `dead-host.invalid:8001`
  (config copy in a scratch dir, tracked file untouched), that section alone
  collapsed to `UNREACHABLE` with a clear `dial tcp: lookup … no such host`
  error while the other 7 kept fresh Ready counts.
- **Responsive layout:** at 60 columns the VERSION line is dropped and node
  rows still render.

## What remains unknown

- Why iad-ci's node list is slow (see failure #2) — the numbers above are
  recorded from the client side only.
- The node counts in the table are a point-in-time snapshot (2026-09-16);
  fleet membership changes constantly and is not itself a correctness claim.
