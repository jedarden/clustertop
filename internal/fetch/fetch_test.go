package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jedarden/clustertop/internal/config"
)

func TestFromNode_ReadyStates(t *testing.T) {
	cases := []struct {
		name  string
		conds []corev1.NodeCondition
		want  bool
	}{
		{"true", []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, true},
		{"false", []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}, false},
		{"unknown", []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionUnknown}}, false},
		{"missing", []corev1.NodeCondition{{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse}}, false},
		{"no conditions at all", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := corev1.Node{Status: corev1.NodeStatus{Conditions: tc.conds}}
			row := FromNode(n)
			if row.Ready != tc.want {
				t.Errorf("Ready = %v, want %v", row.Ready, tc.want)
			}
		})
	}
}

func TestFromNode_Roles(t *testing.T) {
	withRole := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""},
		},
	}
	if got := FromNode(withRole).Roles; got != "control-plane" {
		t.Errorf("Roles = %q, want %q", got, "control-plane")
	}

	noRole := corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"unrelated": "x"}}}
	if got := FromNode(noRole).Roles; got != "<none>" {
		t.Errorf("Roles = %q, want %q", got, "<none>")
	}
}

func TestFromNode_OnDemandWarning(t *testing.T) {
	n := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"servers.ngpc.rxt.io/type": "ondemand"},
		},
	}
	if got := FromNode(n).Warning; got != "on-demand pool" {
		t.Errorf("Warning = %q, want %q", got, "on-demand pool")
	}
}

func TestFromNode_PoolTypeReadsInstanceTypeNotPricingModel(t *testing.T) {
	// Regression test for a bug caught by live smoke-testing: these are two
	// different labels (see docs/research/node-metadata-keys.md). A spot
	// node (the overwhelmingly common case) must still show its shape, not
	// "spot", in PoolType.
	n := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"node.kubernetes.io/instance-type": "compute1-4",
				"servers.ngpc.rxt.io/type":         "spot",
			},
		},
	}
	row := FromNode(n)
	if row.PoolType != "compute1-4" {
		t.Errorf("PoolType = %q, want %q (must read instance-type, not pricing model)", row.PoolType, "compute1-4")
	}
	if row.Warning != "" {
		t.Errorf("Warning = %q, want empty for a spot (non-ondemand) node", row.Warning)
	}
}

func TestFromNode_AutoscalerTaintWarning(t *testing.T) {
	n := corev1.Node{
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "DeletionCandidateOfClusterAutoscaler", Effect: corev1.TaintEffectPreferNoSchedule},
			},
		},
	}
	if got := FromNode(n).Warning; got != "scale-down tainted" {
		t.Errorf("Warning = %q, want %q", got, "scale-down tainted")
	}
}

func TestFromNode_NoWarningByDefault(t *testing.T) {
	n := corev1.Node{}
	if got := FromNode(n).Warning; got != "" {
		t.Errorf("Warning = %q, want empty", got)
	}
}

func TestFetchClusterNodes_TimeoutIsolatesTheCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte(`{"items": []}`))
	}))
	defer srv.Close()

	c := config.Cluster{Name: "slow-cluster", Endpoint: srv.URL}

	start := time.Now()
	_, err := FetchClusterNodes(context.Background(), c, 100*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed > time.Second {
		t.Fatalf("FetchClusterNodes did not respect the timeout: took %v", elapsed)
	}
}

func TestFetchClusterNodes_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items": [{"metadata": {"name": "node-a"}}]}`))
	}))
	defer srv.Close()

	c := config.Cluster{Name: "ok-cluster", Endpoint: srv.URL}
	rows, err := FetchClusterNodes(context.Background(), c, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "node-a" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}
