# How should CI workflow steps be sized for iad-ci?

**Type:** Specification + Measurement

## What I found

`declarative-config/k8s/iad-ci/CLAUDE.md`, read directly:

```
Nodes: 6, mixed: 3 x compute1-8 (3.50 CPU / 6.16 GiB allocatable each)
       + 3 x compute1-4 (1.50 CPU / 2.54 GiB allocatable each)
Total allocatable: 15.00 CPU / 26.08 GiB

Sizing rules:
  Default request for a workflow step: 250m CPU / 512Mi memory
  Fits on any node: <= 1.5 CPU / 2.5 GiB
  Absolute ceiling (biggest node allocatable): 3.50 CPU / 6.16 GiB
  Memory limit: <= 2x request; never above 6 GiB
```

Cross-checked against `domain-check-workflowtemplate.yml`'s actual declared
resources (also read directly): its `goreleaser-release` step sets
`requests: {cpu: 1000m, memory: 2Gi}`, `limits: {cpu: 4000m, memory: 8Gi}`.
That limit is **2x the stated absolute CPU ceiling** (4000m vs. the
3500m biggest-node-allocatable) and **exceeds the stated 6 GiB memory-limit
ceiling** (8Gi). The forge-ci/needle-ci/agentscribe-ci Rust templates (a
background research agent read all four) declare the same `4000m/8Gi` limits.

## What this means for design

Every existing Go/Rust release template in this repo already violates its own
cluster's documented sizing rules. Copying `domain-check`'s numbers verbatim
would reproduce a known-bad pattern rather than follow the actual written
policy. clustertop's WorkflowTemplate sizes against the CLAUDE.md rules
directly instead:

| Step | Request | Limit | Check |
|---|---|---|---|
| `quality-gate` | 500m / 1Gi | 1000m / 2Gi | limit = 2x request; well under 6Gi; fits any node |
| `goreleaser-release` | 1000m / 2Gi | 2000m / 4Gi | limit = 2x request; under 6Gi; under 3.50c/6.16Gi absolute ceiling |

## What remains unknown

Whether 2Gi is actually enough headroom for `goreleaser release --clean`
cross-compiling linux/darwin x amd64/arm64 for a Bubble Tea binary (Go
toolchain + module cache + 4 build targets in memory). This is a real risk —
Go builds can spike memory during compilation, and 2Gi is deliberately half of
what `domain-check` uses. Not resolved by research; resolved by running the
actual release workflow once implemented and watching for OOMKill
(`kubectl describe pod` reason `OOMKilled`) — if it happens, bump toward
domain-check's request (1000m/2Gi → keep) but keep the limit within the
documented 6Gi ceiling (i.e. raise to `2000m/4Gi` → `4000m` request /
`6000m`... no, memory not cpu — raise memory limit toward 6Gi if genuinely
needed, not silently back to 8Gi).

## Resolution (2026-09-16): 2Gi request / 4Gi limit is sufficient — no bump needed

Verified against a real end-to-end tag release, workflow
`clustertop-release-8x4z5` in iad-ci (tag `v0.3.2`, committed at
`e7f2b55`, submitted by `clustertop-sensor` from the GitHub webhook with
`tag=refs/tags/v0.3.2`):

| Step | Wall time | Outcome |
|---|---|---|
| `quality-gate` (500m/1Gi req, 1000m/2Gi limit) | 17:41:22Z → 17:47:55Z (~6.5 min) | Succeeded |
| `goreleaser-release` (1000m/2Gi req, 2000m/4Gi limit) | 17:48:05Z → 17:53:27Z (~5.4 min) | Succeeded |

- The goreleaser pod's container status was sampled live every 15s for the
  pod's entire lifetime (pods are GC'd on completion, so OOM evidence has to
  be caught in-flight): `terminated reason=Completed exit=0`,
  `restartCount=0`, `lastState` empty at every sample — no OOMKilled, and no
  `reason=OOMKilling` events in the namespace.
- Well under the 1800s step deadline too (~5.4 min for the full 4-target
  cross-compile on the 2000m limit) — clustertop is a small module and Go
  cross-compilation of it does not approach the budget headroom that the
  half-of-domain-check sizing was worried about.
- GitHub Release `v0.3.2` published non-draft with all four platform
  tarballs (3.8–4.3 MB each, `state=uploaded`) plus `checksums.txt`.

The deliberately-halved sizing stands. Any future template that copies
domain-check's `4000m/8Gi` is still over-budget per iad-ci/CLAUDE.md; this
workflow demonstrates the documented ceiling (`limit <= 2x request, <= 6Gi,
<= 3.50c/6.16Gi allocatable`) is workable in practice, not just on paper.
