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
- Every per-cluster fetch is wrapped in its own `context.WithTimeout(ctx, 5*time.Second)`.
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

## What remains unknown

Whether 5s is the right per-cluster timeout in practice — it's a reasonable
starting default (well under the 15s refresh interval, generous for a
same-tailnet HTTP round trip) but not empirically tuned. Adjust after running
against the real fleet if any cluster's proxy pod is consistently slower than
that under normal (non-incident) conditions.
