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
