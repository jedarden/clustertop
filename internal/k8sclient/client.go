// Package k8sclient talks to a cluster's read-only kubectl-proxy endpoint.
// Deliberately not k8s.io/client-go — see docs/research/k8s-client-choice.md.
// One resource type, one verb, no auth: plain net/http plus the upstream API
// types for correct JSON field names.
package k8sclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
)

// FetchNodes fetches the full node list from a cluster's kubectl-proxy
// endpoint. The context alone bounds the call — callers are expected to wrap
// it in a per-cluster timeout (see internal/fetch).
func FetchNodes(ctx context.Context, endpoint string) (*corev1.NodeList, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/v1/nodes", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cluster returned HTTP %d", resp.StatusCode)
	}

	var list corev1.NodeList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode nodelist: %w", err)
	}
	return &list, nil
}
