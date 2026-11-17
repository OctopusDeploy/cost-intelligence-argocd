package opencost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestApplySharing_SendsOnlyTenancyToKubecost(t *testing.T) {
	none := NewClient("http://kubecost", time.Second, nil, WithKubecost("c1"))
	params := url.Values{}
	none.applySharing(params)
	if len(params) != 0 {
		t.Fatalf("nothing configured, got %v", params)
	}

	both := NewClient("http://kubecost", time.Second, nil, WithKubecost("c1"),
		WithSharedCosts(true, []string{" kube-system ", "", "monitoring"}))
	params = url.Values{}
	both.applySharing(params)
	if params.Get("shareTenancyCosts") != "true" {
		t.Errorf("shareTenancyCosts = %q", params.Get("shareTenancyCosts"))
	}
	if params.Has("shareNamespaces") {
		t.Errorf("Kubecost pools shared namespaces across clusters, so it must not be asked: %v", params)
	}
	if got := strings.Join(both.shareNamespaces, ","); got != "kube-system,monitoring" {
		t.Errorf("shareNamespaces = %q", got)
	}
}

func TestApplySharing_OpenCostNeverSends(t *testing.T) {
	c := NewClient("http://opencost", time.Second, nil, WithSharedCosts(true, []string{"kube-system"}))
	params := url.Values{}
	c.applySharing(params)
	if len(params) != 0 {
		t.Fatalf("OpenCost must not get Kubecost parameters, got %v", params)
	}
}

func sharedResponse() AllocationResponse {
	return AllocationResponse{Data: []map[string]AllocationData{{
		"web": {CPUCost: 1, SharedCost: 0.5},
	}}}
}

func TestCountShared_AddsSharedCostOnlyWhenAskedFor(t *testing.T) {
	plain := NewClient("http://kubecost", time.Second, nil, WithKubecost("c1"))
	resp := sharedResponse()
	plain.countShared(&resp)
	if got := resp.Data[0]["web"].componentCost(); got != 1 {
		t.Errorf("without sharing = %v, want 1", got)
	}

	shared := NewClient("http://kubecost", time.Second, nil, WithKubecost("c1"), WithSharedCosts(true, nil))
	resp = sharedResponse()
	shared.countShared(&resp)
	if got := resp.Data[0]["web"].componentCost(); got != 1.5 {
		t.Errorf("with sharing = %v, want 1.5", got)
	}
}

func TestShareNamespacesInCluster_SpreadsOnlyThisClustersNamespace(t *testing.T) {
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_, _ = w.Write([]byte(`{"code":200,"data":[{
"kube-system":{"cpuCost":30},
"web":{"cpuCost":10},
"api":{"cpuCost":30},
"__idle__":{"cpuCost":99}}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, time.Second, nil, WithKubecost("c1"), WithSharedCosts(false, []string{"kube-system"}))
	resp := AllocationResponse{Data: []map[string]AllocationData{{"web": {CPUCost: 10}, "__idle__": {CPUCost: 5}}}}
	c.shareNamespacesInCluster(context.Background(), &resp, "web", "2026-01-01T00:00:00Z,2026-01-02T00:00:00Z", false)

	if got := query.Get("filter"); got != `cluster:"c1"` {
		t.Errorf("the pool must be read for this cluster only, filter = %q", got)
	}
	if query.Has("shareNamespaces") {
		t.Errorf("shareNamespaces must not be sent: %v", query)
	}
	// kube-system 30 over the other namespaces' 40: web (10) carries 7.5.
	web := resp.Data[0]["web"]
	if web.SharedCost != 7.5 || web.componentCost() != 17.5 {
		t.Errorf("web shared %v total %v, want 7.5 and 17.5", web.SharedCost, web.componentCost())
	}
	if got := web.SharedCostBreakdown["Namespace kube-system"].TotalCost; got != 7.5 {
		t.Errorf("breakdown names the namespace: %v", web.SharedCostBreakdown)
	}
	if idle := resp.Data[0]["__idle__"]; idle.SharedCost != 0 {
		t.Errorf("idle must carry no shared cost, got %v", idle.SharedCost)
	}
}

func TestShareNamespacesInCluster_ASharedNamespaceReceivesNothing(t *testing.T) {
	c := NewClient("http://unreachable.invalid", time.Second, nil, WithKubecost("c1"), WithSharedCosts(false, []string{"kube-system"}))
	resp := AllocationResponse{Data: []map[string]AllocationData{{"p": {CPUCost: 1}}}}
	c.shareNamespacesInCluster(context.Background(), &resp, "kube-system", "w", false)
	if got := resp.Data[0]["p"].componentCost(); got != 1 {
		t.Errorf("cost = %v, want 1", got)
	}
}

func TestParseOpenCostResponse_LabelsSharedCostIncludedInTheTotal(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	web := AllocationData{CPUCost: 10, SharedCost: 7.5, SharedCostBreakdown: map[string]SharedCostItem{"Namespace kube-system": {TotalCost: 7.5}}, Start: start, End: end}

	web.countShared = true
	got := parseOpenCostResponse(AllocationResponse{Data: []map[string]AllocationData{{"web": web}}}, start, end).AllocationTotals
	if !got.SharedCostIncluded || got.SharedCost != 7.5*30 || got.TotalCost != 17.5*30 {
		t.Errorf("included: flag %v shared %v total %v", got.SharedCostIncluded, got.SharedCost, got.TotalCost)
	}
	if got.SharedCostBreakdown["Namespace kube-system"] != 7.5*30 {
		t.Errorf("breakdown = %v", got.SharedCostBreakdown)
	}

	web.countShared = false
	got = parseOpenCostResponse(AllocationResponse{Data: []map[string]AllocationData{{"web": web}}}, start, end).AllocationTotals
	if got.SharedCostIncluded || got.TotalCost != 10*30 {
		t.Errorf("not included: flag %v total %v", got.SharedCostIncluded, got.TotalCost)
	}
}
