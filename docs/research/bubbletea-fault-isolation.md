# How does the TUI guarantee one dead cluster can't block or blank the rest?

**Type:** Specification

## What I found

This was the core non-functional requirement driving the design (a previous
cluster, `iad-native-ads`, was fully unreachable for a period before being
decommissioned — see `declarative-config/k8s/CLAUDE.md`'s note on the
incident: unreachable endpoints generated continuous reconcile errors for
anything sharing the same controller). A background design agent worked out
the concrete Bubble Tea mechanics for avoiding the equivalent failure mode
here:

- `Init()` fires `tea.Batch(fetchAllCmd(...), tickCmd(15s))`. `tea.Batch`
  runs each contained `tea.Cmd` concurrently, each on its own goroutine —
  this is a property of the Bubble Tea runtime itself, not something this
  project has to build.
- Every per-cluster fetch is wrapped in its own `context.WithTimeout(ctx, 10*time.Second)`.
  `internal/k8sclient.FetchNodes` always returns `(nil, err)` on deadline
  exceeded, dial failure, non-200, or decode failure — never panics, never
  blocks past the timeout.
- Each fetch's `tea.Cmd` closure always returns exactly one `fetchResultMsg`
  (`{ClusterName, Nodes, Err}`), success or failure — there's no code path
  that returns nothing or blocks the Bubble Tea event loop, since the timeout
  bounds the one blocking call (`http.Client.Do`) inside the closure.
- `ClusterState.Nodes` is only overwritten in `Update()` on a *successful*
  fetch. An error result flips `Status` to `StatusError` but leaves the last
  known-good `Nodes` slice untouched — so a cluster that's flaky rather than
  fully dead shows "stale, N seconds old" instead of blanking.

## What this means for design

`Status` must be a tri-state enum (`Pending`/`OK`/`Error`), not a bool —
collapsing "never fetched yet" and "confirmed unreachable" into one state
would make cold-start and genuine failure indistinguishable in the UI, which
matters for trusting what the dashboard is telling you at a glance.

## Timeout validation and decision (2026-09-27)

A fresh concurrent request pass against all eight configured endpoints decoded
valid `NodeList` responses. Healthy clusters completed in 0.36–0.88s;
`iad-ci` took 9.195s and `ord-devimprint` took 45.712s. The earlier
measurements in `docs/research/cluster-endpoints.md` likewise put `iad-ci` at
6.5–9.7s and observed `ord-devimprint` at 15.978s.

The default is therefore raised from 5s to **10s**. This covers the observed
`iad-ci` response while preserving 5s of headroom inside the 15s refresh
interval. `ord-devimprint` can exceed an entire refresh period, so it remains
expected to surface as an isolated timeout during those slow responses rather
than allowing long fetches to accumulate across refresh cycles. This is a
deliberate global budget; a per-cluster timeout configuration is not justified
by the healthy-cluster measurements and would hide the endpoint-side latency.
