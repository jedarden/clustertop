# Dashboard smoke fixtures

The test-only dashboard smoke fixtures live in `testdata/dashboard-smoke/`.
The intended cluster configuration path is
`testdata/dashboard-smoke/clusters.yaml`; its node response fixtures are
`alpha-nodes.json` and `beta-nodes.json` in the same directory.

The alpha fixture represents an on-demand `compute1-4` node and must render
the `⚠ on-demand pool` status warning. The beta fixture represents an
ordinary spot `compute1-8` node and must not render a warning marker. The
end-to-end dashboard test in `main_test.go` exercises this contract through
the HTTP fixtures, built binary, and PTY-rendered UI.

The repository-root `clusters.yaml` is the production cluster inventory. It is
outside this fixture contract and must remain unchanged by dashboard smoke
fixture work. Keep this contract limited to this directory and its fixtures;
it does not call for production code or configuration changes.
