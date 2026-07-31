# What exact label/taint keys does the POOL/TYPE column and the autoscaler-warning flag read?

**Type:** Specification, corrected against live data during Phase 2 smoke testing

## What I found (original pass)

Read directly from `declarative-config/k8s/CLAUDE.md`'s "Rackspace Spot Node
Pools" section:

```python
ntype = n['metadata']['labels'].get('servers.ngpc.rxt.io/type', '?')
pool = n['metadata']['labels'].get('nodepool.ngpc.rxt.io/name', '?')
```

That section states: "Any node showing `type=ondemand` must be investigated."
`k8s/iad-ci/CLAUDE.md`: "nodes carry `DeletionCandidateOfClusterAutoscaler:PreferNoSchedule`."

I initially assumed `servers.ngpc.rxt.io/type` held values like `compute1-4`/
`compute1-8`/`memory1-30` (the node *shape*, matching the capacity tables in
`k8s/CLAUDE.md`), since that's the only label CLAUDE.md's snippet named.

## Correction — verified against a live node during Phase 2 smoke testing

Once the TUI could actually run, I curled a real cluster's node list directly
(`ord-devimprint`, 2026-07-31) rather than trust the assumption further:

```
$ curl -s http://kubectl-proxy-ord-devimprint.tail1b1987.ts.net:8001/api/v1/nodes \
    | python3 -c "... print label keys/values ..."
prod-instance-17854394915530225 -> node.kubernetes.io/instance-type = compute1-4
prod-instance-17854394915530225 -> servers.ngpc.rxt.io/class = ch.vs1.medium-ord
prod-instance-17854394915530225 -> servers.ngpc.rxt.io/type = spot
```

`servers.ngpc.rxt.io/type` actually holds `spot` / `ondemand` — the pricing
model, exactly what `k8s/CLAUDE.md`'s warning is about, but **not** the node
shape. The shape (`compute1-4`, `compute1-8`, `memory1-30`, etc. — what the
mockups in `plan.md` actually show in the POOL/TYPE column) lives in the
standard Kubernetes label `node.kubernetes.io/instance-type`.

## What this means for design

Two different labels, two different jobs — conflating them would have shipped
a POOL/TYPE column showing "spot" on every single node (since that's the
overwhelmingly common value) instead of the actually-useful compute1-4 vs
compute1-8 vs memory1-30 distinction:

- **POOL/TYPE column** reads `node.kubernetes.io/instance-type`. This is what
  `internal/fetch.FromNode` now does (corrected from the original
  `servers.ngpc.rxt.io/type` read).
- **On-demand warning** still correctly reads
  `servers.ngpc.rxt.io/type == "ondemand"` — that part of the original
  research was right, just displayed under the wrong column originally (it
  isn't displayed at all, it only drives the Warning flag, so no display bug
  resulted from this — only the POOL/TYPE column itself was wrong).

## What remains unknown

Whether `node.kubernetes.io/instance-type` is populated consistently on the
non-Rackspace clusters (`ardenone-cluster`, `ardenone-manager`, both k3s on
Hetzner) — if absent there, POOL/TYPE renders `<none>` for those nodes, which
is the correct fallback behavior already implemented, not a bug to fix.
