# Pending cluster fixture contract

The render-test contract for a configured cluster **before its first fetch
result** — the cold start every dashboard session begins in. Resolved
2026-10-17; enforced by `TestPendingClusterFixtureContract` in
`internal/ui/fixtures_test.go`.

## State: uninitialized / pending

The fixture is the uninitialized **pending** state:
`ui.StatusPending` (a `ui.ClusterStatus` value defined in
`internal/ui/model.go`), the zero value `NewModel` assigns every configured
cluster. It renders as
`<name> — connecting…` inside a gray border.

The status is deliberately tri-state, not bool: pending must stay visually
distinct from `StatusError`/`UNREACHABLE`, or a cold start is
indistinguishable from a real incident (see
`docs/research/bubbletea-fault-isolation.md`).

## No first-fetch result data

A pending fixture may contain **no first-fetch result data**:

| Field | Value | Why |
|---|---|---|
| `Nodes` | nil / empty | no successful fetch has produced a snapshot yet |
| `Err` | nil | no failed fetch has produced an error yet |
| `LastFetch` | zero `time.Time` | never fetched; must never render as "stale" |
| `Fetching` | false | the base fixture is not in flight; tests wanting the in-flight variant set it themselves as an overlay |

## Definition and enforcement

`pendingClusterState` in `internal/ui/fixtures_test.go` is the fixture;
`TestPendingClusterFixtureContract` (same file) enforces the state and
no-data clauses and asserts the fixture stays identical to the shape
`NewModel` gives a real cluster at cold start — the fixture must never drift
into a state the dashboard cannot reach.

## Consumer

The **dashboard render-test package `internal/ui`** is the fixture's
consumer. Current consumers within the package: all three pending render
cases in `view_test.go` — `TestRenderClusterSection_PendingHasNoBodyLines`,
the gray-border case of `TestRenderClusterSection_AggregateBorderColors`, and
`TestRenderBody_PendingTransitionsToHealthyAfterFirstResult`, which renders
the fixture through `Model.View` for the full-dashboard
pending→first-result transition. `model_test.go` exercises the same state
through `NewModel` rather than the fixture helper.

## Relationship to the dashboard smoke fixture contract

`testdata/dashboard-smoke/README.md` defines the separate main-package smoke
fixture contract (`testdata/dashboard-smoke/` HTTP payload fixtures). That
contract covers the packaged node-response fixtures; this one covers the
in-package pending render fixture. Neither calls for production code or
configuration changes.
