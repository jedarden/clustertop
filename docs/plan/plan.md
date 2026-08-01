# clustertop — Plan

## 1. Overview

`clustertop` is a Go + Bubble Tea terminal dashboard that polls every cluster
in the fleet's existing read-only `kubectl-proxy` endpoint for node status and
renders a live, auto-refreshing, terminal-width-responsive view — one bordered
section per cluster, one bordered box per node, wrapped in a grid inside that
cluster's border. It replaces "SSH in, pick a cluster, run `kubectl get
nodes`, repeat 8 times" with one screen, and the border is what communicates
"these nodes are one cluster" — not just a text header.

It ships as a single static binary via a new Argo CI pipeline
(quality-gate → GoReleaser → GitHub Release), hosted on Forgejo with a GitHub
mirror per the org's standard flow. It is a client tool only — it never writes
to any cluster, and it is not itself deployed to any cluster.

Node-box grid, real output at 100 columns (`docs/notes/node-box-grid-mockup.md`
has the full design history, including the two layouts this superseded):

```
┌─ apexalgo-iad ── 3/3 Ready ──────────────────────────────────────────────────────────────────────┐
│ ╭─────────────────────╮ ╭─────────────────────╮ ╭─────────────────────╮                          │
│ │ memory1-30-a        │ │ memory1-30-b        │ │ memory1-30-c        │                          │
│ │ ● Ready             │ │ ● Ready             │ │ ● Ready             │                          │
│ │ roles: <none>       │ │ roles: <none>       │ │ roles: <none>       │                          │
│ │ pool: memory1-30    │ │ pool: memory1-30    │ │ pool: memory1-30    │                          │
│ │ v1.33.0             │ │ v1.33.0             │ │ v1.33.0             │                          │
│ │ age: 45d            │ │ age: 45d            │ │ age: 45d            │                          │
│ ╰─────────────────────╯ ╰─────────────────────╯ ╰─────────────────────╯                          │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘

┌─ iad-ci ── 5/6 Ready ⚠ ──────────────────────────────────────────────────────────────────────────┐
│ ╭─────────────────────╮ ╭─────────────────────╮ ╭─────────────────────╮ ╭─────────────────────╮  │
│ │ prod-instance-1781… │ │ prod-instance-1781… │ │ prod-instance-1781… │ │ prod-instance-1782… │  │
│ │ ● Ready             │ │ ● Ready             │ │ ● Ready             │ │ ⬤ NotReady ⚠ scale… │  │
│ │ roles: <none>       │ │ roles: <none>       │ │ roles: <none>       │ │ roles: <none>       │  │
│ │ pool: compute1-8    │ │ pool: compute1-8    │ │ pool: compute1-8    │ │ pool: compute1-4    │  │
│ │ v1.33.0             │ │ v1.33.0             │ │ v1.33.0             │ │ v1.33.0             │  │
│ │ age: 32h            │ │ age: 32h            │ │ age: 32h            │ │ age: 24h            │  │
│ ╰─────────────────────╯ ╰─────────────────────╯ ╰─────────────────────╯ ╰─────────────────────╯  │
│ ╭─────────────────────╮ ╭─────────────────────╮                                                  │
│ │ prod-instance-1782… │ │ prod-instance-1782… │                                                  │
│ │ ● Ready             │ │ ● Ready             │                                                  │
│ │ roles: <none>       │ │ roles: <none>       │                                                  │
│ │ pool: compute1-4    │ │ pool: compute1-4    │                                                  │
│ │ v1.33.0             │ │ v1.33.0             │                                                  │
│ │ age: 24h            │ │ age: 23h            │                                                  │
│ ╰─────────────────────╯ ╰─────────────────────╯                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘

┌─ iad-kalshi — UNREACHABLE ───────────────────────────────────────────────────────────────────────┐
│ ⚠ dial tcp: context deadline exceeded (last seen 4m ago)                                          │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

Design decisions (see `docs/notes/node-box-grid-mockup.md` for the options
that were rejected):

1. **Unreachable cluster** collapses to a single error line inside its border
   (not a grid of dimmed/stale boxes) — simplest, and matches the
   fault-isolation rule that a dead cluster must never block or corrupt the
   rest of the render.
2. **Box width is consistent across the whole render, scaled to available
   terminal width** — `gridLayout` (in `internal/ui/box.go`) picks the most
   columns that fit at a minimum readable width (16 chars), then stretches
   every box in that row evenly into whatever space is left over (capped at
   30 chars, so a very wide terminal doesn't produce mostly-empty boxes).
   Column count and width are recomputed on every `tea.WindowSizeMsg` — there
   is no fixed breakpoint table like the old table-based layout had.
3. **Node names truncate to fit whatever width `gridLayout` computed** for
   that render, not a fixed character count — `…` marks the cut.
4. **Every cluster gets a full bordered section, including single-node
   clusters** (`ardenone-manager`) — no special-casing by node count.
5. **Color carries status**: node box borders and status lines are green
   (Ready) or red (NotReady); a warning suffix renders in yellow. The
   cluster's outer border also reflects aggregate health — green if every
   node is Ready, yellow if all Ready but at least one carries a warning, red
   if any node is NotReady or the cluster is unreachable.

One real bug worth remembering if this box-rendering code is ever touched
again: lipgloss's `Style.Width(n)` sizes the *padded* content area, not the
text area inside the padding. With `Padding(0, 1)`, rendering
already-truncated text at `Width(contentWidth)` still wrapped it onto an
extra line, because the padding ate 2 of those columns. The fix is
`Width(contentWidth + 2)`. Caught by an actual rendered-output smoke check,
not by `go vet`/`go test` — the tests that would have caught it
(`TestRenderClusterSection_AllLinesSameWidth`) were written *after* finding
this by eye, precisely so it can't silently regress.

## 2. Hard constraints and invariants

- **Never add auth/TLS/credentials to the cluster HTTP client.** The
  `kubectl-proxy` endpoints are deliberately unauthenticated over Tailscale.
  See `docs/research/cluster-endpoints.md`.
- **Never import `k8s.io/client-go`.** One resource type, one verb, no auth —
  `net/http` + `k8s.io/api/core/v1` types only. See
  `docs/research/k8s-client-choice.md`.
- **A single unreachable cluster must never block or blank the rest of the
  UI.** Every fetch has its own `context.WithTimeout`; `ClusterState.Nodes` is
  only overwritten on success, never cleared on error. See
  `docs/research/bubbletea-fault-isolation.md`.
- **Never use `kind: Job`/`CronJob`** if a cluster-side component is ever
  added (none exist today). Standing `declarative-config` rule.
- **CI resource requests/limits stay within `iad-ci`'s documented sizing
  rules** — limit never above 2× request, never above 6 GiB, never above one
  node's 3.50c/6.16Gi allocatable. Not copied from `domain-check`, which
  violates its own cluster's rules. See `docs/research/iad-ci-resource-budget.md`.
- **`clusters.yaml` loading is lenient**: unknown keys ignored, empty list is
  the only hard load error. A stale field must never break an
  already-released binary.
- **`internal/fetch` owns `NodeRow` and the `corev1.Node → NodeRow` mapping —
  `internal/ui` never does this mapping itself.** `internal/ui` depends on
  `internal/fetch`, never the reverse. (Fixed after plan review: the original
  draft had `internal/fetch` referencing a `ui.NodeRow` type, which is a
  backwards/circular package dependency across the stated phase order.)

## 3. Current state (verified 2026-07-31)

- Nothing has been built yet. This repo (`clustertop`) exists locally as a
  scaffold only — no code yet, no git remote, not yet pushed to
  Forgejo/GitHub.
- 8 live clusters, each with a `devpod-observer/kubectl-proxy.yml` manifest on
  `declarative-config`'s `main` branch — full inventory in
  `docs/research/cluster-endpoints.md`. Caveat carried forward honestly: this
  is a manifest reading, not a live connectivity test from this host — Phase 1's
  verification step (§7) is where that actually gets confirmed by running the
  binary.
- `ardenone-hub` is fully decommissioned on `declarative-config`'s `main`
  (removed 2026-06-09) — confirmed via `git log`, not a candidate for this
  tool. See `docs/research/ardenone-hub-decommission.md`.
- `declarative-config`'s `iad-ci` CI already has a directly-reusable pattern
  (`domain-check`, Go + GoReleaser → GitHub Release) to model the new pipeline
  on. See `docs/research/ci-pipeline-pattern.md`.

## 4. Architecture decisions

| Decision | Rationale |
|---|---|
| Go + Bubble Tea (not Rust+ratatui, not Python+textual) | User decision. Go's ecosystem fits a K8s-polling tool well, and Bubble Tea's Elm architecture + `bubbles/table` gives concurrent-fetch and table rendering largely for free. |
| `net/http` + `k8s.io/api/core/v1`, not `client-go` | No auth, one endpoint, one verb — client-go's discovery/RESTMapper/credential-plugin machinery is unused weight. See research note. |
| **DECIDED:** Go module path is `github.com/jedarden/clustertop` | Matches the existing CI convention — every Go/Rust CI template in `iad-ci` clones from `https://github.com/{{git-repo}}.git`, and GitHub is the actually-resolvable target for `go get`/module proxy even though Forgejo is the git origin of record. |
| **DECIDED:** `clusters.yaml` endpoint values are full tailnet FQDNs (`traefik-apexalgo-iad.tail1b1987.ts.net:8001`), not short hostnames | Short hostnames (as used in root `CLAUDE.md`'s worked kubectl examples) depend on the resolving host's search-domain config being set up for Tailscale MagicDNS. The FQDN works regardless of that, and is what `docs/research/cluster-endpoints.md`'s inventory actually verified. |
| `NodeRow` type + `corev1.Node → NodeRow` mapping lives in `internal/fetch`, not `internal/ui` | Fixes a phase-order violation found in plan review — `internal/fetch` (Phase 1) can't import a type from `internal/ui` (Phase 2). `internal/ui`'s job is `NodeRow → table.Row` (a *different*, later transformation), not `corev1.Node → NodeRow`. |
| POOL/TYPE column reads `servers.ngpc.rxt.io/type`; autoscaler flag matches taint key substring `DeletionCandidateOfClusterAutoscaler` or label value `ondemand` | Sourced directly from `declarative-config/k8s/CLAUDE.md` and `k8s/iad-ci/CLAUDE.md` — see `docs/research/node-metadata-keys.md`. Both absent → render `<none>`, not an error (non-Spot clusters like `ardenone-cluster`/`ardenone-manager` won't have these labels). |
| Static `clusters.yaml`, checked into repo, plus a `sync-clusters` regenerator subcommand | Avoids the exact drift failure mode just found live in this org (`ardenone-hub` stale in docs for weeks post-decommission). |
| `sync-clusters` reads two different manifest shapes depending on routing style | Traefik-routed clusters' hostname lives in `<cluster>/traefik/tailscale-service.yml`, not on the `kubectl-proxy.yml` Service itself (verified by direct read, see `docs/research/sync-clusters-source-manifests.md`); direct-Tailscale-operator clusters carry the annotation directly on the `kubectl-proxy` Service. |
| One `bubbles/table.Model` per cluster section, never `.Focus()`ed | Keybindings are intentionally minimal (`q`, `r`) — no per-row selection needed. |
| `ClusterStatus` tri-state (Pending/OK/Error), not bool | Collapsing "never fetched" and "confirmed unreachable" would make cold start indistinguishable from a real incident. |
| New, fully self-contained Argo WorkflowTemplate (not shared/refactored) | Matches this repo's actual convention — zero shared Workflow templates exist anywhere in `k8s/iad-ci/`, confirmed by grep. |
| CI resource sizing computed from `iad-ci/CLAUDE.md` rules, not copied from `domain-check` | `domain-check`'s own limits already violate its cluster's documented budget. |
| Forgejo primary + GitHub push-mirror, origin = Forgejo only | Standing org convention (`/home/coding/CLAUDE.md`). Must happen before Phase 4's CI trigger path (GitHub webhook) is reachable at all — see Phase 0. |

## 5. Phases (ordered by dependency)

### Phase 0 — Repo hosting bootstrap
No dependencies. Blocks Phase 4 (the GitHub-webhook-triggered CI path is
unreachable without a GitHub remote existing). Does not block Phases 1–3,
which only need local `go build`/`go test`.
- **Create + wire remotes** — owns no source files; this is operational
  (Forgejo repo → GitHub repo → push mirror → local `origin` = Forgejo),
  following `/home/coding/CLAUDE.md`'s "Git Hosting" section exactly. Push the
  initial repo scaffold (README.md, CLAUDE.md, docs/) as the first commit on
  `main`.

### Phase 1 — Core fetch, no UI
Depends on Phase 0 only for having somewhere to push to; the code itself has
no dependency on Phase 0 completing first. Tasks:

- **go.mod + main.go skeleton** — owns `go.mod`, `go.sum`, `main.go`. Module
  path `github.com/jedarden/clustertop` (decided, §4). Dispatches to
  `ui.Run()` by default or `syncclusters.Run(os.Args[2:])` for the
  `sync-clusters` subcommand; both are stubbed (`return errors.New("not
  implemented")`) so `main.go` compiles immediately and is never touched
  again after this task — Phase 2 and Phase 5 implement the real `Run()`
  bodies in their own files, not in `main.go`.
- **Cluster config loader** — owns `internal/config/config.go`,
  `internal/config/config_test.go`. Exact schema:
  ```go
  package config

  type Config struct {
      Clusters []Cluster `yaml:"clusters"`
  }
  type Cluster struct {
      Name     string `yaml:"name"`
      Endpoint string `yaml:"endpoint"`           // e.g. http://traefik-apexalgo-iad.tail1b1987.ts.net:8001
      Route    string `yaml:"route"`               // display-only: "traefik-kubectl-tcp" | "direct-tailscale-operator"
      Notes    string `yaml:"notes,omitempty"`
  }
  func LoadClusters(path string) (Config, error)   // gopkg.in/yaml.v3, default (non-strict) decode; error only if Clusters is empty
  ```
- **clusters.yaml** — owns `clusters.yaml`. Exact content (FQDN endpoints per
  the DECIDED row in §4, transcribed from `docs/research/cluster-endpoints.md`):
  ```yaml
  clusters:
    - name: apexalgo-iad
      endpoint: http://traefik-apexalgo-iad.tail1b1987.ts.net:8001
      route: traefik-kubectl-tcp
    - name: ardenone-cluster
      endpoint: http://traefik-ardenone-cluster.tail1b1987.ts.net:8001
      route: traefik-kubectl-tcp
    - name: ardenone-manager
      endpoint: http://traefik-ardenone-manager.tail1b1987.ts.net:8001
      route: traefik-kubectl-tcp
    - name: iad-ci
      endpoint: http://traefik-iad-ci.tail1b1987.ts.net:8001
      route: traefik-kubectl-tcp
    - name: iad-kalshi
      endpoint: http://kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001
      route: direct-tailscale-operator
    - name: iad-options
      endpoint: http://traefik-iad-options.tail1b1987.ts.net:8001
      route: traefik-kubectl-tcp
    - name: ord-devimprint
      endpoint: http://kubectl-proxy-ord-devimprint.tail1b1987.ts.net:8001
      route: direct-tailscale-operator
    - name: rs-manager
      endpoint: http://traefik-rs-manager.tail1b1987.ts.net:8001
      route: traefik-kubectl-tcp
  ```
- **K8s HTTP client** — owns `internal/k8sclient/client.go`,
  `internal/k8sclient/client_test.go`. `FetchNodes(ctx, endpoint)
  (*corev1.NodeList, error)`, tested against `httptest.Server` fixtures
  (200-valid, malformed-JSON, 500, connection-refused, slow-handler-past-timeout).
- **Per-cluster fetch/timeout wrapper + NodeRow** — owns
  `internal/fetch/fetch.go`, `internal/fetch/fetch_test.go`. Defines:
  ```go
  package fetch

  type NodeRow struct {
      Name, Roles, PoolType, Version, Age string
      Ready   bool
      Warning string // "" | "on-demand pool" | "scale-down tainted" — see docs/research/node-metadata-keys.md
  }
  func FromNode(n corev1.Node) NodeRow          // the corev1.Node -> NodeRow mapping lives HERE, not in internal/ui
  func FetchClusterNodes(ctx context.Context, c config.Cluster, timeout time.Duration) ([]NodeRow, error)
  ```
  Wraps `k8sclient.FetchNodes` in `context.WithTimeout`, always returns
  `(rows, err)`, never panics.

### Phase 2 — Bubble Tea static render (depends on Phase 1)
- **Model/Msg/Cmd skeleton** — owns `internal/ui/model.go`,
  `internal/ui/msg.go`, `internal/ui/cmd.go`. `ClusterState{Cluster
  config.Cluster; Status ClusterStatus; Nodes []fetch.NodeRow; Err error;
  LastFetch time.Time; Fetching bool; Table table.Model}`, `Model{Clusters
  []ClusterState; idx map[string]int; Width, Height int; RefreshEvery,
  FetchTimeout time.Duration}`, `tickMsg`, `fetchResultMsg{ClusterName string;
  Nodes []fetch.NodeRow; Err error}`, `fetchClusterCmd`, `fetchAllCmd`,
  `tickCmd`.
- **Table rendering** — owns `internal/ui/table.go`,
  `internal/ui/table_test.go`. `toTableRows(nodes []fetch.NodeRow)
  []table.Row` (business rows → widget rows — NOT the `corev1.Node →
  NodeRow` mapping, which Phase 1 already owns), one `bubbles/table.Model`
  per cluster.
- **View composition + styles** — owns `internal/ui/view.go`,
  `internal/ui/style.go`. Section chrome (header, `UNREACHABLE` banner,
  "updated Ns ago"), `lipgloss` styling for Ready/NotReady/Unreachable/⚠.
- **Keybindings** — owns `internal/ui/keys.go`. `q`/`ctrl+c` quit, `r`
  manual refresh.
- **`ui.Run()` real implementation** — owns `internal/ui/run.go`. Replaces
  Phase 1's stub; constructs and runs the `tea.Program`.

### Phase 3 — Concurrency, refresh, responsive layout (depends on Phase 2)
- **Tick/refresh + `WindowSizeMsg` breakpoint** — edits
  `internal/ui/model.go`, `internal/ui/view.go`. 15s auto-refresh, wide/narrow
  column switch at ~100 cols, `bubbles/viewport` wrapper for scrolling.
- **Model transition tests** — owns `internal/ui/model_test.go`. Pure
  `(Model, Msg) -> (Model, Cmd)` tests, including the stale-on-error case.
- **Fault-isolation integration test** — owns
  `internal/ui/integration_test.go`. Multiple `httptest.Server`s (one
  sleeping past timeout) wired through the real `fetchAllCmd`.

### Phase 4 — CI pipeline (depends on Phase 0 for the GitHub remote to exist, and on Phase 1–3 for there to be real code to gate; independent of Phase 5)
Spans two repos with two separate git-discipline flows: `clustertop` itself
(owns `.goreleaser.yml`, tagged for release) and `declarative-config` (owns
the three CI manifests below — follow *that* repo's own `CLAUDE.md`: `git pull
--rebase origin main` before editing, push immediately after committing, never
batch).
- **WorkflowTemplate** — owns
  `declarative-config/k8s/iad-ci/argo-workflows/clustertop-workflowtemplate.yml`.
  `build` (push to `main` → quality-gate) + `release` (tag `v*` →
  quality-gate + goreleaser-release), sized per
  `docs/research/iad-ci-resource-budget.md` (quality-gate: 500m/1Gi request,
  1000m/2Gi limit; goreleaser-release: 1000m/2Gi request, 2000m/4Gi limit).
- **Sensor** — owns
  `declarative-config/k8s/iad-ci/argo-events/clustertop-sensor.yml`. Two
  triggers, no `ci-author-filter` (no self-commit path to guard against,
  unlike `domain-check`).
- **EventSource entry** — edits
  `declarative-config/k8s/iad-ci/argo-events/github-eventsource.yml`, adding
  a `clustertop:` block (`endpoint: /clustertop`, `port: "12000"` — same
  pattern every existing entry uses) next to the existing `domain-check:`
  entry.
- **`.goreleaser.yml`** — owns `.goreleaser.yml` (in this repo).
  `CGO_ENABLED=0`, linux/darwin × amd64/arm64, tar.gz archives, checksums,
  GitHub release.

### Phase 5 — `sync-clusters` subcommand (depends on Phase 1's config package only; independent of Phases 2–4; no file overlap with either)
- **Manifest scanner + regenerator** — owns
  `internal/syncclusters/sync.go`, `internal/syncclusters/sync_test.go`. For
  each `<cluster>` directory in a given `declarative-config` checkout path:
  1. If `<cluster>/devpod-observer/kubectl-proxy.yml`'s Service carries a
     `tailscale.com/hostname` annotation directly → direct-Tailscale-operator
     cluster, use it, `route: direct-tailscale-operator`.
  2. Else, read `<cluster>/traefik/tailscale-service.yml`'s
     `tailscale.com/hostname` annotation instead, confirm it exposes a
     `kubectl-tcp` port → `route: traefik-kubectl-tcp`.
  3. Cluster directories with neither shape are skipped with a logged
     warning, not a fatal error.
  See `docs/research/sync-clusters-source-manifests.md`. Fixture-directory
  tests (`t.TempDir()`) covering both shapes, not a real checkout.

## 6. Open questions

Nothing below blocks any phase above — each is independently resolvable
without changing what's already specified.

- **Is a 4Gi memory limit (2Gi request) enough for the `goreleaser-release`
  CI step?** Deliberately half of `domain-check`'s own (over-budget) 8Gi
  limit. Resolve by running the real release workflow once (Phase 4) and
  checking for `OOMKilled`; if it happens, raise toward the 6 GiB ceiling
  stated in `iad-ci/CLAUDE.md`, not back to `domain-check`'s 8Gi.
- **Exact GoReleaser version to pin** in the WorkflowTemplate — check what's
  current at Phase 4 implementation time (`domain-check` pins 2.5.0 as of
  this writing) rather than freezing a version now.
- **Per-cluster fetch timeout tuning** (currently 5s default) — adjust after
  running against the real fleet if any cluster's proxy is consistently
  slower under normal conditions.
- **Pod-level rollup / live CPU-mem commitment / metrics-server integration**
  — explicitly out of scope for v1 (would require listing pods on every
  cluster, not just nodes). Noted as a fast-follow, not a v1 blocker.
- **Whether every Traefik-routed cluster's hostname manifest is named/shaped
  identically to `apexalgo-iad`'s** (`traefik/tailscale-service.yml`, object
  name `traefik-tailscale`) — verified for one cluster only; Phase 5's
  scanner should warn-and-skip on a shape it doesn't recognize rather than
  assume every cluster matches.

## 7. Verification playbook

```bash
# Phase 1–3: build, vet, test
cd ~/clustertop
go build ./...
go vet ./...
go test ./...
# Expected: all pass, zero output beyond ok lines

# Phase 1–3: live run against the real fleet, captured non-interactively via tmux
tmux new-session -d -s clustertop-check -x 220 -y 50 'go run .'
sleep 6
tmux capture-pane -t clustertop-check -p > /tmp/clustertop-wide.txt
grep -c "Ready" /tmp/clustertop-wide.txt        # expected: >0, one per healthy node/cluster summary
grep -q "UNREACHABLE" /tmp/clustertop-wide.txt && echo "unexpected UNREACHABLE at full width" || echo "ok: no unreachable clusters"
tmux resize-window -t clustertop-check -x 60 -y 50
sleep 1
tmux capture-pane -t clustertop-check -p > /tmp/clustertop-narrow.txt
grep -q "VERSION" /tmp/clustertop-narrow.txt && echo "FAIL: VERSION column should be dropped under 100 cols" || echo "ok: narrow layout dropped VERSION"
tmux send-keys -t clustertop-check q
tmux kill-session -t clustertop-check
# Separately: edit one clusters.yaml entry to a bad host:port, repeat the capture,
# and confirm that cluster's section shows UNREACHABLE while the other 7 still show fresh Ready counts.

# Phase 0 + 4: CI build path (requires Phase 0 done first — origin must exist)
git push origin main
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig \
  get workflows -n argo-workflows --sort-by=.metadata.creationTimestamp | tail -5
# Expected: a clustertop-build-* workflow appears and reaches Succeeded

# Phase 4: CI release path
git tag v0.1.0 && git push origin v0.1.0
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig \
  get workflows -n argo-workflows --sort-by=.metadata.creationTimestamp | tail -5
gh release view v0.1.0 --repo jedarden/clustertop
# Expected: a clustertop-release-* workflow Succeeded; `gh release view` shows
# binaries attached for linux/darwin x amd64/arm64

# Phase 5: sync-clusters — must reproduce the hand-written clusters.yaml exactly,
# since both are reading the same manifests at the same commit. A non-empty
# diff means the scanner has a bug (wrong file, wrong annotation key) — not
# an acceptable drift to wave through.
go run . sync-clusters --declarative-config-path ~/declarative-config
git diff --exit-code clusters.yaml
# Expected: exit code 0, empty diff
```
