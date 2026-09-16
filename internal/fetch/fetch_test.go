package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
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

func TestFromNode_KubeletVersion(t *testing.T) {
	withVersion := corev1.Node{
		Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.4"}},
	}
	if got := FromNode(withVersion).Version; got != "v1.31.4" {
		t.Errorf("Version = %q, want %q", got, "v1.31.4")
	}

	noVersion := corev1.Node{}
	if got := FromNode(noVersion).Version; got != "" {
		t.Errorf("Version = %q, want empty when NodeInfo is absent", got)
	}
}

func TestFromNode_MissingFields(t *testing.T) {
	// A node stripped of every optional field must degrade to safe defaults,
	// not panic or leak zero-value oddities into the UI.
	row := FromNode(corev1.Node{})
	if row.Name != "" {
		t.Errorf("Name = %q, want empty", row.Name)
	}
	if row.Roles != "<none>" {
		t.Errorf("Roles = %q, want %q", row.Roles, "<none>")
	}
	if row.PoolType != "" {
		t.Errorf("PoolType = %q, want empty when the instance-type label is absent", row.PoolType)
	}
	if row.Version != "" {
		t.Errorf("Version = %q, want empty", row.Version)
	}
	if row.Age != "?" {
		t.Errorf("Age = %q, want %q for a zero creation timestamp", row.Age, "?")
	}
	if row.Ready {
		t.Error("Ready = true, want false when no conditions are present")
	}
	if row.Warning != "" {
		t.Errorf("Warning = %q, want empty", row.Warning)
	}
}

func TestFormatAge_Buckets(t *testing.T) {
	// Offsets sit mid-bucket so the gap between building the timestamp and
	// reading the clock can't push a value across a boundary.
	if got := formatAge(time.Time{}); got != "?" {
		t.Errorf("formatAge(zero) = %q, want %q", got, "?")
	}

	cases := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"seconds", 30 * time.Second, "30s"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 5 * time.Hour, "5h"},
		{"days", 72 * time.Hour, "3d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := time.Now().Add(-tc.age)
			if got := formatAge(ts); got != tc.want {
				t.Errorf("formatAge(%v ago) = %q, want %q", tc.age, got, tc.want)
			}
		})
	}
}

func TestFromNode_RolesMultipleAndEmptySuffix(t *testing.T) {
	// Label-map iteration order is unspecified, so compare the role set, not
	// the joined string.
	multi := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
				"node-role.kubernetes.io/worker":        "true",
			},
		},
	}
	got := strings.Split(FromNode(multi).Roles, ",")
	want := []string{"control-plane", "worker"}
	if len(got) != len(want) {
		t.Fatalf("Roles = %q, want exactly {%s}", FromNode(multi).Roles, strings.Join(want, ","))
	}
	sort.Strings(got)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Roles = %q, want {%s} (derived from label keys, values ignored)",
				FromNode(multi).Roles, strings.Join(want, ","))
			break
		}
	}

	// The bare prefix is a role label with an empty suffix — it must be
	// skipped, not rendered as an empty role name.
	barePrefix := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{nodeRoleLabelPrefix: ""},
		},
	}
	if got := FromNode(barePrefix).Roles; got != "<none>" {
		t.Errorf("Roles = %q, want %q for a bare role-label prefix", got, "<none>")
	}
}

func TestFromNode_WarningPrecedenceAndTaintMatching(t *testing.T) {
	// The on-demand label is checked first and wins over a taint.
	both := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{pricingModelLabel: onDemandPricingModel},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: "DeletionCandidateOfClusterAutoscaler"}},
		},
	}
	if got := FromNode(both).Warning; got != "on-demand pool" {
		t.Errorf("Warning = %q, want %q (pricing label takes precedence over taints)", got, "on-demand pool")
	}

	// Taints unrelated to the autoscaler must not produce a warning.
	otherTaint := corev1.Node{
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "node.kubernetes.io/unreachable", Effect: corev1.TaintEffectNoExecute},
			},
		},
	}
	if got := FromNode(otherTaint).Warning; got != "" {
		t.Errorf("Warning = %q, want empty for a non-autoscaler taint", got)
	}

	// The autoscaler match is substring-based, so a decorated key still trips it.
	decoratedTaint := corev1.Node{
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "x" + autoscalerTaintKeySubs + "-cluster-1"},
			},
		},
	}
	if got := FromNode(decoratedTaint).Warning; got != "scale-down tainted" {
		t.Errorf("Warning = %q, want %q for a substring-matched autoscaler taint key", got, "scale-down tainted")
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
