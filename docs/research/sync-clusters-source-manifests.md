# Which exact manifest does sync-clusters need to read to get each cluster's hostname?

**Type:** Behavioural

## What I found

Surfaced as a defect during the plan review pass, then verified directly by
reading the real files in `~/declarative-config` (2026-07-31):

```
$ grep -A15 "^kind: Service" k8s/apexalgo-iad/devpod-observer/kubectl-proxy.yml
kind: Service
metadata:
  name: kubectl-proxy
  namespace: devpod-observer
  ...
spec:
  type: ClusterIP
  ...
```

No `tailscale.com/*` annotation anywhere on the `kubectl-proxy` Service for a
Traefik-routed cluster. The actual hostname lives in a sibling file:

```
$ cat k8s/apexalgo-iad/traefik/tailscale-service.yml
apiVersion: v1
kind: Service
metadata:
  name: traefik-tailscale
  namespace: traefik
  annotations:
    tailscale.com/expose: "true"
    tailscale.com/hostname: "traefik-apexalgo-iad"
spec:
  ports:
    - name: kubectl-tcp
      port: 8001
      ...
```

For a direct-Tailscale-operator cluster (no Traefik), the annotation is on the
`kubectl-proxy` Service itself:

```
$ grep -B5 -A20 "^kind: Service" k8s/iad-kalshi/devpod-observer/kubectl-proxy.yml
  annotations:
    tailscale.com/expose: "true"
    tailscale.com/hostname: kubectl-proxy-iad-kalshi
```

## What this means for design

`sync-clusters` cannot always read the same file per cluster. The rule:

1. Check `<cluster>/devpod-observer/kubectl-proxy.yml`'s Service for a
   `tailscale.com/hostname` annotation. If present → direct-Tailscale-operator
   cluster, use it as-is.
2. If absent → Traefik-routed cluster. Read
   `<cluster>/traefik/tailscale-service.yml`'s `traefik-tailscale` Service
   `tailscale.com/hostname` annotation instead, and confirm it exposes a
   `kubectl-tcp` port (8001) among its `spec.ports`.

This is a two-file-path decision tree, not a single fixed path — the original
plan task description ("scan `kubectl-proxy.yml` + its paired
Service/IngressRouteTCP") was wrong: the IngressRouteTCP only carries
`HostSNI(\`*\`)` and an entrypoint reference, no hostname at all.

## What remains unknown

Whether every Traefik-routed cluster's `traefik/tailscale-service.yml` is
named identically (`traefik-tailscale` object name, same file path). Verified
for `apexalgo-iad` only; `sync-clusters`' tests should use a fixture directory
covering both shapes (direct-operator and Traefik-routed) rather than assuming
the second shape generalizes untested — if a cluster deviates, the regenerator
should skip it with a warning, not silently emit a wrong hostname.
