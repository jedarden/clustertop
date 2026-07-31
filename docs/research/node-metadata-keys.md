# What exact label/taint keys does the POOL/TYPE column and the autoscaler-warning flag read?

**Type:** Specification

## What I found

Read directly from `declarative-config/k8s/CLAUDE.md`, the "Rackspace Spot
Node Pools" section — its own worked `kubectl get nodes -o json` snippet:

```python
ntype = n['metadata']['labels'].get('servers.ngpc.rxt.io/type', '?')
pool = n['metadata']['labels'].get('nodepool.ngpc.rxt.io/name', '?')
```

That section also states: "Any node showing `type=ondemand` must be
investigated — its node pool should be deleted from the Spot UI." This is a
standing, explicitly-documented org concern, not something I'm inferring.

Separately, `declarative-config/k8s/iad-ci/CLAUDE.md` (read directly, "Cluster
shape" section): "Cluster-autoscaler is active — nodes carry
`DeletionCandidateOfClusterAutoscaler:PreferNoSchedule` as they are scaled
down."

Both labels are Rackspace-Spot-specific (`servers.ngpc.rxt.io/*`) — they will
be absent on `ardenone-cluster` (k3s on Hetzner) and `ardenone-manager` (k3s,
single node), which are not Rackspace Spot.

## What this means for design

- POOL/TYPE column reads `node.metadata.labels["servers.ngpc.rxt.io/type"]`;
  render `<none>` if absent (not an error — expected on non-Spot clusters).
- The ⚠ warning annotation on a node row fires when
  `node.metadata.labels["servers.ngpc.rxt.io/type"] == "ondemand"` (matches
  the standing org concern directly) **or** when any entry in
  `node.spec.taints[].key` contains the substring
  `DeletionCandidateOfClusterAutoscaler` (autoscaler mid-scale-down) —
  these are two different conditions with two different meanings, both
  worth a visible flag, and should be distinguishable in the row (different
  warning text), not collapsed into one generic "⚠".

## What remains unknown

Whether `nodepool.ngpc.rxt.io/name` (the pool *name*, not the pool *shape*)
is worth a separate column or is redundant with POOL/TYPE for this tool's
purposes — deferred; `servers.ngpc.rxt.io/type` alone (compute1-4, compute1-8,
memory1-30, ondemand) is the more decision-relevant of the two and is what
ships in v1.
