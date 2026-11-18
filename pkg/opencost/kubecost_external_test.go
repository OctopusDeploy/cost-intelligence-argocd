package opencost

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCountShared_MarksExternalCostOnlyWhenAskedFor(t *testing.T) {
	t.Parallel()

	resp := func() AllocationResponse {
		return AllocationResponse{Data: []map[string]AllocationData{{"web": {CPUCost: 1, ExternalCost: 4}}}}
	}

	off := resp()
	NewClient("http://kubecost", time.Second, nil, WithKubecost("c1")).countShared(&off)
	assert.InDelta(t, 1.0, off.Data[0]["web"].componentCost(), 1e-9, "reported, not counted")

	on := resp()
	NewClient("http://kubecost", time.Second, nil, WithKubecost("c1"), WithExternalCosts(true)).countShared(&on)
	assert.InDelta(t, 5.0, on.Data[0]["web"].componentCost(), 1e-9)

	opencost := resp()
	NewClient("http://opencost", time.Second, nil, WithExternalCosts(true)).countShared(&opencost)
	assert.InDelta(t, 1.0, opencost.Data[0]["web"].componentCost(), 1e-9, "Kubecost only")
}

func TestParseOpenCostResponse_CountsExternalCostInTheTotalAndThePods(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	resp := AllocationResponse{Data: []map[string]AllocationData{{
		"web": {CPUCost: 1, ExternalCost: 4, Start: start, End: end, countExternal: true, Properties: AllocationProperties{Pod: "web"}},
	}}}

	got := parseOpenCostResponse(resp, start, end).AllocationTotals
	assert.True(t, got.ExternalCostIncluded)
	assert.InDelta(t, 4.0*30, got.ExternalCost, 1e-9)
	assert.InDelta(t, 5.0*30, got.TotalCost, 1e-9)

	pods := parsePodBreakdown(resp)
	if assert.Len(t, pods, 1) {
		assert.InDelta(t, 4.0*30, pods[0].ExternalCost, 1e-9)
		assert.InDelta(t, 5.0*30, pods[0].TotalCost, 1e-9)
	}
}
