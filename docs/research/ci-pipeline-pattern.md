# What does clustertop's Argo CI pipeline need to look like?

**Type:** Prior art + Specification

## What I found

`declarative-config` already has a Go-binary → GitHub-Release pipeline for
`domain-check`, via GoReleaser. I read
`k8s/iad-ci/argo-workflows/domain-check-workflowtemplate.yml` directly.
Relevant excerpt (the `release` path — triggered by a tag push, the part
clustertop's pipeline actually needs):

```yaml
- name: release
  steps:
    - - name: quality-gate
        template: quality-gate
    - - name: goreleaser-release
        template: goreleaser-release

- name: quality-gate
  container:
    image: golang:1.26
    command: [sh, -c]
    args:
      - |
        git clone --branch "$TAG" "https://github.com/{{workflow.parameters.git-repo}}.git" /workspace
        cd /workspace
        go vet ./...
        go test -race ./...

- name: goreleaser-release
  container:
    image: golang:1.26-alpine
    command: [sh, -c]
    args:
      - |
        GORELEASER_VERSION="2.5.0"
        wget -qO- ".../goreleaser_Linux_x86_64.tar.gz" | tar -xz -C /usr/local/bin goreleaser
        git clone "https://github.com/{{workflow.parameters.git-repo}}.git" /workspace
        cd /workspace && git checkout "$TAG"
        git describe --tags --exact-match
        goreleaser release --clean
```

Both steps auth via `GH_TOKEN`/`GITHUB_TOKEN` from the existing
`github-webhook-secret` ExternalSecret — no new secret needed.

The trigger side (`k8s/iad-ci/argo-events/domain-check-sensor.yml`, also read
directly) has two triggers: push-to-`main` → `build` entrypoint, tag push
`v*` → `release` entrypoint, each gated by a `Sensor` `filters.data` match on
`body.ref`. The `build` path also auto-bumps a `VERSION` file and pushes,
which is why that sensor has a `ci-author-filter` — it excludes commits
authored by `"Argo Workflows CI"` to avoid the bot re-triggering itself.

`k8s/iad-ci/argo-events/github-eventsource.yml` registers one entry per repo
under a shared `EventSource`; I read the full file and confirmed the pattern
— `domain-check` already has an entry under a `# Go release repos` comment
block, endpoint `/domain-check`, port 12000, pointing at the same shared
`github-webhook-secret`.

A background research agent additionally confirmed (searching the whole
`k8s/iad-ci/` tree): there is **no shared/callable Workflow template** for
checkout or release steps anywhere in this repo — `grep -r "templateRef:"` and
`ClusterWorkflowTemplate` both returned zero hits across every CI template.
Every WorkflowTemplate, Rust or Go, inlines its own script from scratch. This
is the established convention, not an oversight.

## What this means for design

- clustertop's WorkflowTemplate should be a **new, fully self-contained file**
  modeled on `domain-check`'s `quality-gate` + `goreleaser-release` steps —
  not a shared/refactored template, since nothing else in this directory does
  that.
- Only two entrypoints are needed: `build` (push to `main`, quality-gate only)
  and `release` (tag push, quality-gate + goreleaser-release). clustertop
  ships a CLI binary, not a container image, so `domain-check`'s
  `docker-build`/`resolve-version`/`VERSION`-file-bump machinery doesn't
  apply — GoReleaser reads the git tag directly on a tag-triggered run, so
  there's nothing to version-bump.
- No `ci-author-filter` needed on the sensor — clustertop's `build` path never
  commits back to the repo, so there's no self-trigger loop to guard against
  (unlike `domain-check`, which needs the filter because of its VERSION bump).
- Auth: reuse `github-webhook-secret`. No new ExternalSecret.

## What remains unknown

Whether GoReleaser 2.5.0 (pinned in `domain-check`) is still the latest stable
— worth checking at implementation time rather than research time, since
GoReleaser's own release cadence is outside this repo's control and pinning to
whatever's current when the WorkflowTemplate is actually written is more
correct than freezing a version now.
