# Should clustertop use k8s.io/client-go, or something lighter?

**Type:** Prior art

## What I found

The only operation this tool ever performs is `GET /api/v1/nodes` against an
already-unauthenticated in-cluster proxy (see `cluster-endpoints.md`) — no
auth negotiation, no resource discovery, no multi-group/version content
negotiation, exactly one resource type, one verb.

`k8s.io/client-go`'s `kubernetes.NewForConfig` path exists to support: auth
plugins (exec credential plugins, GCP/Azure OIDC), API discovery and
RESTMapper for arbitrary resource types, and generic REST client plumbing for
arbitrary verbs. None of that is exercised by a single hardcoded `GET` against
a known endpoint. The dependency also pulls in `golang.org/x/oauth2` and
cloud-provider credential-plugin registration code that's compiled in but
never called on this code path.

`k8s.io/api/core/v1` (a much smaller module — just the generated Go structs
matching the Kubernetes API's JSON schema, no client machinery) is sufficient
to unmarshal a `NodeList` response body correctly, since the wire format is
identical either way — client-go's HTTP client and a bare `net/http.Client`
hit the same REST endpoint and get the same JSON back.

## What this means for design

`internal/k8sclient` uses `net/http` + `encoding/json` decoding straight into
`k8s.io/api/core/v1.NodeList`. No `client-go` dependency. This is now a
**never-rule** in this repo's `CLAUDE.md` — re-adding client-go later without
revisiting this note would be a regression (bigger binary, slower builds, CVE
surface for unused code paths), not neutral.

## What remains unknown

If a future requirement needs a second resource type (e.g. Pods, for a
future pod-rollup feature noted as out of scope in `plan.md`), this decision
should be revisited then — a second hand-rolled endpoint call is still
probably fine, but three or more might tip the balance toward client-go's
generic REST client being worth its weight. Not a concern for v1.
