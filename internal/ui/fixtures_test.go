package ui

import (
	"testing"

	"github.com/jedarden/clustertop/internal/config"
)

// The pending-cluster fixture contract — full text in
// docs/notes/pending-cluster-fixture-contract.md:
//
// pendingClusterState is the cold-start dashboard fixture: a configured
// cluster in the uninitialized StatusPending state, before its first fetch
// result arrives. It carries no first-fetch result data — no node snapshot,
// no fetch error, no last-fetch timestamp — and is not in flight. The
// dashboard render-test package (this one, internal/ui) is the fixture's
// consumer; TestPendingClusterFixtureContract enforces every clause.
func pendingClusterState(name string) ClusterState {
	return ClusterState{
		Cluster: config.Cluster{Name: name},
		Status:  StatusPending,
	}
}

// TestPendingClusterFixtureContract pins the contract: the fixture is the
// uninitialized StatusPending state, holds no first-fetch result data, and
// stays identical to the shape NewModel gives a real cluster at cold start —
// if the model's initialization changes, this fixture must change with it or
// every pending render test is testing a state the dashboard can't reach.
func TestPendingClusterFixtureContract(t *testing.T) {
	cs := pendingClusterState("iad-ci")

	if cs.Status != StatusPending {
		t.Errorf("fixture status = %v, want the uninitialized StatusPending state", cs.Status)
	}
	if cs.Cluster.Name != "iad-ci" {
		t.Errorf("fixture cluster name = %q, want the configured name passed in", cs.Cluster.Name)
	}
	if len(cs.Nodes) != 0 {
		t.Errorf("fixture carries %d node rows; no first-fetch result data may be present", len(cs.Nodes))
	}
	if cs.Err != nil {
		t.Errorf("fixture carries error %v; no first-fetch result data may be present", cs.Err)
	}
	if !cs.LastFetch.IsZero() {
		t.Errorf("fixture LastFetch = %v; a pending cluster has never fetched", cs.LastFetch)
	}
	if cs.Fetching {
		t.Error("fixture Fetching = true; the base pending fixture is not in flight")
	}

	cold := newTestModel(cs.Cluster.Name).Clusters[0]
	if cold.Status != cs.Status || cold.Fetching != cs.Fetching ||
		len(cold.Nodes) != len(cs.Nodes) ||
		(cold.Err == nil) != (cs.Err == nil) ||
		cold.LastFetch.IsZero() != cs.LastFetch.IsZero() {
		t.Errorf("fixture drifted from NewModel's cold-start state: fixture=%+v cold-start=%+v", cs, cold)
	}
}
