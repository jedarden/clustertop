# Node box-grid layout — mockups for review

**Resolved 2026-08-01.** All five open questions below were decided and
implemented (`internal/ui/box.go`, `internal/ui/view.go`); `docs/plan/plan.md`
§1 now carries the canonical mockup and the final answers. This file stays as
the design-history record — the rejected alternatives and the reasoning for
each pick.

Answers: (1) 3a — unreachable collapses to a single error line. (2) box width
scales to available terminal width, kept consistent across every box in the
render (not fixed, not per-box content-sized). (3) node names truncate to fit
whatever width that turns out to be. (4) single-node clusters still get a
full bordered section. (5) color carries status (and aggregate cluster
health via the border color).

Supersedes the per-cluster **table** layout in `docs/plan/plan.md` §1. Replaces
one row per node with one **bordered box** per node; nodes belonging to the
same cluster sit inside that cluster's outer border. Chosen shape (per
2026-08-01 design check-in):

- **Arrangement:** wrap-to-fill-width — boxes flow left to right and wrap to a
  new row when the terminal runs out of columns, same idea as flexbox.
- **Box content:** full detail — name, status (+ warning if any), roles,
  pool/type, version, age.

Everything below is a plain-text approximation for reviewing the *shape* of
the design — exact spacing/alignment will be produced by `lipgloss` in the
real implementation, not hand-typed monospace.

## Legend

```
● Ready       — node is Ready
⬤ NotReady    — node is NotReady (filled circle, distinct silhouette from ●)
⚠ <text>      — extra warning appended to status (e.g. scale-down taint)
UNREACHABLE   — cluster fetch failed; see "unreachable cluster" example below
```

## Example 1 — healthy cluster, wide terminal (~120 cols)

`apexalgo-iad`, 3/3 Ready, all boxes fit on one row:

```
┌─ apexalgo-iad ─────────────────────────────────────────── 3/3 Ready ──┐
│ ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐     │
│ │ memory1-30-a      │  │ memory1-30-b      │  │ memory1-30-c      │   │
│ │ ● Ready           │  │ ● Ready           │  │ ● Ready           │   │
│ │ roles: <none>     │  │ roles: <none>     │  │ roles: <none>     │   │
│ │ pool: memory1-30  │  │ pool: memory1-30  │  │ pool: memory1-30  │   │
│ │ v1.33.0           │  │ v1.33.0           │  │ v1.33.0           │   │
│ │ age: 45d          │  │ age: 45d          │  │ age: 45d          │   │
│ └──────────────────┘  └──────────────────┘  └──────────────────┘     │
└─────────────────────────────────────────────────────────────────────┘
```

## Example 2 — degraded cluster, wraps to a second row

`iad-ci`, 5/6 Ready, 6 nodes at 3-per-row wraps once. Long node names are
truncated with `…` to keep box width fixed (see open question below):

```
┌─ iad-ci ───────────────────────────────────────────────── 5/6 Ready ⚠ ┐
│ ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐     │
│ │ prod-inst…640125  │  │ prod-inst…130218  │  │ prod-inst…710219  │   │
│ │ ● Ready           │  │ ● Ready           │  │ ● Ready           │   │
│ │ roles: <none>     │  │ roles: <none>     │  │ roles: <none>     │   │
│ │ pool: compute1-8  │  │ pool: compute1-8  │  │ pool: compute1-8  │   │
│ │ v1.33.0           │  │ v1.33.0           │  │ v1.33.0           │   │
│ │ age: 32h          │  │ age: 32h          │  │ age: 32h          │   │
│ └──────────────────┘  └──────────────────┘  └──────────────────┘     │
│ ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐     │
│ │ prod-inst…130660  │  │ prod-inst…550731  │  │ prod-inst…560732  │   │
│ │ ⬤ NotReady ⚠ taint │  │ ● Ready           │  │ ● Ready           │   │
│ │ roles: <none>     │  │ roles: <none>     │  │ roles: <none>     │   │
│ │ pool: compute1-4  │  │ pool: compute1-4  │  │ pool: compute1-4  │   │
│ │ v1.33.0           │  │ v1.33.0           │  │ v1.33.0           │   │
│ │ age: 24h          │  │ age: 24h          │  │ age: 23h          │   │
│ └──────────────────┘  └──────────────────┘  └──────────────────┘     │
└─────────────────────────────────────────────────────────────────────┘
```

## Example 3 — unreachable cluster

**Open question (flagged, not yet decided):** two options —

**3a. Collapse to a single error line** (today's behavior, unchanged):

```
┌─ iad-kalshi ──────────────────────────────────────── UNREACHABLE ─────┐
│ ⚠ dial tcp: context deadline exceeded (last seen 4m ago)              │
└─────────────────────────────────────────────────────────────────────┘
```

**3b. Keep last-known boxes visible, dimmed, stamped stale** (consistent with
the "stale-but-visible" fault-isolation rule already in `model.go` — nodes are
only overwritten on a *successful* fetch, so the boxes already exist):

```
┌─ iad-kalshi ── stale 4m ago ─────────────────────────── UNREACHABLE ⚠ ┐
│ ┌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┐  ┌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┐                          │
│ ╎ compute1-4-a      ╎  ╎ compute1-4-b      ╎                          │
│ ╎ ● Ready (stale)   ╎  ╎ ● Ready (stale)   ╎                          │
│ ╎ roles: <none>     ╎  ╎ roles: <none>     ╎                          │
│ ╎ pool: compute1-4  ╎  ╎ pool: compute1-4  ╎                          │
│ ╎ v1.33.0           ╎  ╎ v1.33.0           ╎                          │
│ ╎ age: 5d            ╎  ╎ age: 5d           ╎                          │
│ └╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┘  └╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┘                          │
└─────────────────────────────────────────────────────────────────────┘
```

3b is more information-dense (matches "what were the last nodes I saw" at a
glance) but is more code — a dashed/dimmed box style plus a "stale" cue on
every node line. 3a is what's already built. **Needs your call before I
implement either.**

## Example 4 — narrow terminal (~60 cols), same cluster reflows to 1-per-row

`iad-ci` at 60 cols — full-detail boxes no longer fit 3-across, so they stack
single-column instead of dropping fields (contrast with the old table's
narrow mode, which dropped POOL/TYPE and VERSION columns instead of
reflowing):

```
┌─ iad-ci ──────────────────────── 5/6 Ready ⚠ ┐
│ ┌──────────────────┐                         │
│ │ prod-inst…640125  │                         │
│ │ ● Ready           │                         │
│ │ roles: <none>     │                         │
│ │ pool: compute1-8  │                         │
│ │ v1.33.0           │                         │
│ │ age: 32h          │                         │
│ └──────────────────┘                         │
│ ┌──────────────────┐                         │
│ │ prod-inst…130218  │                         │
│ │ ● Ready           │                         │
│ │ ...               │                         │
│ └──────────────────┘                         │
└───────────────────────────────────────────────┘
```

## Open questions to vet alongside the layout itself

1. **Unreachable-cluster rendering** — 3a (collapse to error line) vs 3b
   (dimmed stale boxes)?
2. **Fixed vs content-based box width** — examples above use a fixed
   ~20-char interior width so every box in a cluster lines up. An
   alternative is sizing each box to its own longest field (tighter, but
   boxes in the same row won't line up when node names vary in length).
3. **Node-name truncation length** — examples truncate to `prod-inst…640125`
   (first 9 + last 6 chars). Fine, or prefer a different scheme (e.g. drop
   the common `prod-instance-` prefix entirely across all iad-ci/rs-manager/
   iad-kalshi/iad-options/ord-devimprint boxes, which share that naming
   convention)?
4. **Very small clusters** — `ardenone-manager` is a single k3s node. Is a
   1-node cluster still worth its own bordered section (as shown throughout),
   or should single-node clusters render more compactly?
5. **Color** — none of the above shows color (plain text mockup). Real
   implementation would color the status glyph (green ●, red ⬤, yellow ⚠) via
   `lipgloss` — confirm that's wanted, and whether the outer cluster border
   should also tint red/yellow when any node inside is unhealthy.

Once these are settled I'll update `docs/plan/plan.md` §1's mockup to match,
then rework `internal/ui/table.go` (retire `bubbles/table` per-cluster grids
don't need a scrolling table widget, just `lipgloss` box composition) and
`view.go`'s `renderClusterSection`.
