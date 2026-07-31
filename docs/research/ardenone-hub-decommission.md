# Is ardenone-hub still a live cluster clustertop needs to poll?

**Type:** Behavioural

## What I found

`k8s/CLAUDE.md`'s cluster table still lists `ardenone-hub` as a live cluster.
I checked directly against git history (commands run and output pasted
verbatim, 2026-07-31):

```
$ ls k8s/ardenone-hub
ls: cannot access 'k8s/ardenone-hub': No such file or directory

$ git log --oneline --diff-filter=D -- 'k8s/ardenone-hub/*' | head -5
0a9c19ad chore: remove decommissioned ardenone-hub tree
ff23507f fix(devimprint): delete zombie ardenone-hub stack to stop R2 writes
c199fb44 chore(devimprint): remove ARMOR deployment from ardenone-hub
e791967a chore(devimprint): remove from ardenone-hub (migrated to ord-devimprint)
7ae83548 chore(ardenone-hub): remove Liqo federation — spot clusters now independent

$ git log -1 --format="%H %ad %s" -- 'k8s/ardenone-hub/*'
0a9c19ada9fcd71bafcf6e88b2fb67aa1785df5d Tue Jun 9 21:15:55 2026 -0400 chore: remove decommissioned ardenone-hub tree
```

Removed from `main` on 2026-06-09. A stray unmerged worktree branch
(`.claude/worktrees/agent-a54c92149120be9c7/`) still has old ardenone-hub
manifests, but that's not `main` and not synced by ArgoCD.

## What this means for design

`clusters.yaml` has 8 entries, not 9. `ardenone-hub` does not appear anywhere
in this tool. If it's ever rebuilt, adding it back is a normal `sync-clusters`
run (or a manual `clusters.yaml` edit) once its `devpod-observer/kubectl-proxy.yml`
reappears on `main` — not a special case.

## What remains unknown

Nothing load-bearing. `k8s/CLAUDE.md`'s stale table entry is a documentation
bug in a different repo, out of scope for this project to fix (flagged
separately to the user).
