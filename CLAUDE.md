# clustertop — Agent Instructions

## Build / test (copy-pasteable)

```bash
go build ./...
go vet ./...
go test ./...
```

CI runs the same two gate commands (`go vet ./...`, `go test -race ./...`) in
`iad-ci` on every push to `main`. Run them locally before committing — a
failing gate on `main` is a broken build for anyone who pulls.

## Commit conventions

Conventional-commit style subject (`feat(ui): ...`, `fix(fetch): ...`,
`chore(ci): ...`). Every commit that closes a tracked bead MUST carry a
trailer:

```
feat(ui): render per-cluster node table

Task: bf-xxxx
```

Git identity for all commits in this repo:
```
user.email = github@jedarden.com
user.name  = jedarden
```

## Never-rules

- **Never add authentication, TLS, or credentials to the cluster HTTP client.**
  The read-only `kubectl-proxy` endpoints this tool polls are deliberately
  unauthenticated over Tailscale — adding auth fields to `k8sclient` implies a
  different, higher-privilege access path exists. It doesn't. If a cluster
  needs write access or secret data, that's a different tool, not this one.
- **Never import `k8s.io/client-go`.** This tool does exactly one thing —
  `GET /api/v1/nodes` — and client-go's discovery/RESTMapper/credential-plugin
  machinery has no use here. See `docs/research/` for the reasoning. Adding it
  back is a regression, not an improvement.
- **Never let one cluster's fetch block or crash the rest.** Every cluster
  fetch is wrapped in its own `context.WithTimeout`; a hung or unreachable
  cluster must degrade to a visible `UNREACHABLE` state for that cluster only.
  This is the whole point of the fault-isolation design — violating it in a
  "quick fix" reintroduces the exact failure mode the tool exists to avoid.
- **Never use `kind: Job` or `CronJob`** if this tool ever grows a
  cluster-deployed component (it currently has none — it's a CLI binary run
  directly). Standing org-wide rule; see `declarative-config/k8s/CLAUDE.md`.

## When blocked

Stop. Do not guess past a genuine blocker (missing credential, ambiguous
requirement, a cluster endpoint that doesn't match what `clusters.yaml`
claims). Leave a comment on the relevant bead describing exactly what's
missing, release the bead, and let a human or a later task resolve it. Do not
close a bead that doesn't meet its acceptance criteria.

## Tracker

Beads (`bf` CLI) live in this repo. Every task bead's acceptance criteria are
commands (`go build`, `go test ./path/...`, etc.) — a task isn't done until
the command it names actually passes.
