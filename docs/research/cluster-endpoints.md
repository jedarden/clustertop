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

## What remains unknown

- I have not personally curled all 8 endpoints from this host — the table
  above is one agent's manifest reading, not a live connectivity test. The
  verification playbook in `plan.md` covers this: running the actual binary
  against `clusters.yaml` is the real test, and a wrong or dead endpoint fails
  loudly (`UNREACHABLE` in the TUI) rather than silently, so this is an
  acceptable risk to carry into implementation rather than block on.
