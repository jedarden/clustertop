// Package fetch turns a cluster's raw node list into display-ready rows,
// with per-cluster fetch timeout isolation applied. It owns the
// corev1.Node -> NodeRow mapping — internal/ui depends on this package,
// never the reverse (see docs/plan/plan.md section 2/4).
package fetch

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/jedarden/clustertop/internal/config"
	"github.com/jedarden/clustertop/internal/k8sclient"
)

const (
	nodeRoleLabelPrefix = "node-role.kubernetes.io/"
	// instanceTypeLabel gives the node's shape (compute1-4, compute1-8,
	// memory1-30, ...). pricingModelLabel gives spot vs ondemand — a
	// DIFFERENT label; conflating the two was a bug caught by live testing,
	// see docs/research/node-metadata-keys.md.
	instanceTypeLabel      = "node.kubernetes.io/instance-type"
	pricingModelLabel      = "servers.ngpc.rxt.io/type"
	onDemandPricingModel   = "ondemand"
	autoscalerTaintKeySubs = "DeletionCandidateOfClusterAutoscaler"
)

// NodeRow is one cluster node, ready to render.
type NodeRow struct {
	Name     string
	Roles    string
	PoolType string
	Version  string
	Age      string
	Ready    bool
	Warning  string // "" | "on-demand pool" | "scale-down tainted"
}

// FromNode maps a raw corev1.Node into a display-ready NodeRow.
func FromNode(n corev1.Node) NodeRow {
	row := NodeRow{
		Name:     n.Name,
		Roles:    rolesOf(n),
		PoolType: n.Labels[instanceTypeLabel],
		Version:  n.Status.NodeInfo.KubeletVersion,
		Age:      formatAge(n.CreationTimestamp.Time),
		Ready:    isReady(n),
		Warning:  warningFor(n),
	}
	return row
}

func rolesOf(n corev1.Node) string {
	var roles []string
	for label := range n.Labels {
		if strings.HasPrefix(label, nodeRoleLabelPrefix) {
			role := strings.TrimPrefix(label, nodeRoleLabelPrefix)
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	return strings.Join(roles, ",")
}

func isReady(n corev1.Node) bool {
	for _, cond := range n.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func warningFor(n corev1.Node) string {
	if n.Labels[pricingModelLabel] == onDemandPricingModel {
		return "on-demand pool"
	}
	for _, taint := range n.Spec.Taints {
		if strings.Contains(taint.Key, autoscalerTaintKeySubs) {
			return "scale-down tainted"
		}
	}
	return ""
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// FetchClusterNodes fetches and maps one cluster's nodes, bounded by timeout.
// Always returns (rows, err) — never panics, never blocks past timeout.
func FetchClusterNodes(ctx context.Context, c config.Cluster, timeout time.Duration) ([]NodeRow, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	list, err := k8sclient.FetchNodes(ctx, c.Endpoint)
	if err != nil {
		return nil, err
	}

	rows := make([]NodeRow, 0, len(list.Items))
	for _, n := range list.Items {
		rows = append(rows, FromNode(n))
	}
	return rows, nil
}
