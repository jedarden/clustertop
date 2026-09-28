package ui

import "github.com/jedarden/clustertop/internal/config"

// pendingClusterState is the cold-start dashboard fixture: the cluster is
// configured, but no fetch has completed, so it has no snapshot, error, or
// last-fetch timestamp to render.
func pendingClusterState(name string) ClusterState {
	return ClusterState{
		Cluster: config.Cluster{Name: name},
		Status:  StatusPending,
	}
}
