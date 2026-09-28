# Dashboard smoke fixtures

The test-only dashboard smoke fixtures live in `testdata/dashboard-smoke/`.
The intended cluster configuration path is
`testdata/dashboard-smoke/clusters.yaml`; its node response fixtures are
`alpha-nodes.json` and `beta-nodes.json` in the same directory.

The repository-root `clusters.yaml` is the production cluster inventory. It is
outside this fixture contract and must remain unchanged by dashboard smoke
fixture work. Keep this contract limited to this directory and its fixtures;
it does not call for production code or configuration changes.
