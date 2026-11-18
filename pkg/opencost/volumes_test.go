package opencost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/models"
)

const runnersNamespace = "ci"

// runnersWithVolumes is a namespace whose only entry besides one pod is the unmounted volumes, three
// of them: two claimed by one application, one by none.
func runnersWithVolumes() *PodAllocations {
	pods := &PodAllocations{
		namespace: runnersNamespace,
		resp: AllocationResponse{Data: []map[string]AllocationData{{
			"runner-1": {
				Start: day, End: day.Add(24 * 60 * 60e9), Minutes: 24 * 60, CPUCost: 1, TotalCost: 1,
				Properties: AllocationProperties{Namespace: runnersNamespace, Pod: "runner-1", ControllerKind: "deployment", Controller: "runner"},
			},
			// Kubecost keeps no namespace on this entry.
			"ci-unmounted-pvcs": {
				Start: day, End: day.Add(24 * 60 * 60e9), Minutes: 24 * 60, PVCost: 6, TotalCost: 6,
				Properties: AllocationProperties{Pod: "ci-unmounted-pvcs"},
			},
		}}},
		volumeCosts: map[string]float64{"pv-a": 2, "pv-b": 3, "pv-c": 1},
		claimOf: map[string]claimRef{
			"pv-a": {name: "builds-1"},
			"pv-b": {name: "builds-2"},
			"pv-c": {name: "other"},
		},
	}
	return pods
}

var runnerClaims = models.ManagedResources{
	{Kind: "Deployment", Name: "runner", Namespace: runnersNamespace},
	{Kind: "PersistentVolumeClaim", Name: "builds-1", Namespace: runnersNamespace},
	{Kind: "PersistentVolumeClaim", Name: "builds-2", Namespace: runnersNamespace},
}

func TestForApplication_OwnsTheVolumesItsClaimsName(t *testing.T) {
	t.Parallel()

	got := runnersWithVolumes().ForApplication(runnerClaims)

	assert.InDelta(t, (2+3)*30, got.OpenCostOutput.AllocationTotals.StorageCost, 1e-9)
	assert.InDelta(t, 1*30, got.OpenCostOutput.AllocationTotals.CPUCost, 1e-9)
	assert.InDelta(t, (1+2+3)*30, got.TotalMonthlyCost, 1e-9)
}

func TestForApplication_WithoutClaimsLeavesVolumesToTheNamespace(t *testing.T) {
	t.Parallel()

	got := runnersWithVolumes().ForApplication(runnerClaims.PodOwners())

	assert.InDelta(t, 0.0, got.OpenCostOutput.AllocationTotals.StorageCost, 1e-9)
}

func TestForApplication_SoleApplicationOwnsEveryUnmountedVolume(t *testing.T) {
	t.Parallel()

	pods := runnersWithVolumes()
	pods.SoleApplication(true)

	// Its claims name two of the three volumes; with no claims at all, it owns them just the same.
	for _, resources := range []models.ManagedResources{runnerClaims, runnerClaims.PodOwners()} {
		got := pods.ForApplication(resources)

		assert.InDelta(t, 6*30, got.OpenCostOutput.AllocationTotals.StorageCost, 1e-9)
		assert.InDelta(t, (1+6)*30, got.TotalMonthlyCost, 1e-9)
	}
	assert.InDelta(t, 6.0/24, pods.HourlyCostOf(runnerClaims.PodOwners()).Storage, 1e-9)
}

func TestForApplication_SoleApplicationWithoutAnUnmountedEntryChangesNothing(t *testing.T) {
	t.Parallel()

	pods := runnersWithVolumes()
	delete(pods.resp.Data[0], "ci-unmounted-pvcs")
	pods.SoleApplication(true)

	got := pods.ForApplication(runnerClaims)

	assert.InDelta(t, 0.0, got.OpenCostOutput.AllocationTotals.StorageCost, 1e-9)
	assert.InDelta(t, 1*30, got.TotalMonthlyCost, 1e-9)
}

func TestForApplication_VolumesAreCappedAtTheEntry(t *testing.T) {
	t.Parallel()

	pods := runnersWithVolumes()
	pods.volumeCosts["pv-a"] = 100

	got := pods.ForApplication(runnerClaims)

	assert.InDelta(t, 6*30, got.OpenCostOutput.AllocationTotals.StorageCost, 1e-9)
}

func TestForApplication_ClaimInAnotherNamespaceIsNotOwned(t *testing.T) {
	t.Parallel()

	pods := runnersWithVolumes()
	pods.claimOf["pv-a"] = claimRef{namespace: "elsewhere", name: "builds-1"}

	got := pods.ForApplication(runnerClaims)

	assert.InDelta(t, 3*30, got.OpenCostOutput.AllocationTotals.StorageCost, 1e-9)
}

func TestHourlyCostOf_CountsTheClaimedVolumes(t *testing.T) {
	t.Parallel()

	got := runnersWithVolumes().HourlyCostOf(runnerClaims)

	assert.InDelta(t, 5.0/24, got.Storage, 1e-9)
}

func TestVolumeClaimsOf_LeavesOutAClaimMoreThanOneDiskHas(t *testing.T) {
	t.Parallel()

	got := volumeClaimsOf(AssetsResponse{Data: map[string]AssetData{
		"a": {Type: "Disk", ClaimName: "builds-1", VolumeName: "pv-a"},
		"b": {Type: "Disk", ClaimName: "shared", VolumeName: "pv-b"},
		"c": {Type: "Disk", ClaimName: "shared", VolumeName: "pv-c"},
		"d": {Type: "Disk", ClaimName: "named", ClaimNamespace: "ci", Properties: AssetProperties{Name: "pv-d"}},
		"e": {Type: "Disk", ClaimName: "named", ClaimNamespace: "other", Properties: AssetProperties{Name: "pv-e"}},
		"f": {Type: "Node", ClaimName: "x", VolumeName: "pv-f"},
		"g": {Type: "Disk", VolumeName: "pv-g"},
	}})

	assert.Equal(t, map[string]claimRef{
		"pv-a": {name: "builds-1"},
		"pv-d": {namespace: "ci", name: "named"},
		"pv-e": {namespace: "other", name: "named"},
	}, got.byVolume)
}

func TestPvName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "pvc-1", pvName("cluster=eks:name=pvc-1"))
	assert.Equal(t, "pvc-1", pvName("pvc-1"))
}

func TestPodAllocations_AttributesUnmountedVolumesFromTheAssets(t *testing.T) {
	t.Parallel()

	var assetCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/assets":
			assetCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":200,"data":{"d1":{"type":"Disk","claimName":"builds-1","volumeName":"pv-a","totalCost":2},"d2":{"type":"Disk","claimName":"other","volumeName":"pv-c","totalCost":1}}}`))
		default:
			_, _ = w.Write([]byte(`{"code":200,"data":[{
				"ci-unmounted-pvcs":{"start":"2026-10-02T00:00:00Z","end":"2026-10-03T00:00:00Z","minutes":1440,"pvCost":3,"properties":{"pod":"ci-unmounted-pvcs"},
					"pvs":{"cluster=c:name=pv-a":{"cost":2},"cluster=c:name=pv-c":{"cost":1}}},
				"runner-1":{"start":"2026-10-02T00:00:00Z","end":"2026-10-03T00:00:00Z","minutes":1440,"cpuCost":1,"properties":{"namespace":"ci","pod":"runner-1","controllerKind":"deployment","controller":"runner"}}
			}]}`))
		}
	}))
	t.Cleanup(server.Close)
	client := NewClient(server.URL, 0, nil, WithKubecost("c"))

	first, err := client.GetPodAllocations(context.Background(), runnersNamespace)
	require.NoError(t, err)
	_, err = client.GetPodAllocations(context.Background(), runnersNamespace)
	require.NoError(t, err)

	app := first.ForApplication(runnerClaims)
	assert.InDelta(t, 2*30, app.OpenCostOutput.AllocationTotals.StorageCost, 1e-9)
	assert.EqualValues(t, 1, assetCalls.Load(), "the claims are read once for a while")
	assert.Nil(t, first.resp.Data[0]["ci-unmounted-pvcs"].PVs, "the volume list is not kept")
}

func TestPodAllocations_VolumesStayInTheNamespaceWhenTheAssetsFail(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/assets" {
			http.Error(w, "no", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"data":[{"ci-unmounted-pvcs":{"start":"2026-10-02T00:00:00Z","end":"2026-10-03T00:00:00Z","minutes":1440,"pvCost":3,"properties":{"pod":"ci-unmounted-pvcs"},"pvs":{"cluster=c:name=pv-a":{"cost":3}}}}]}`))
	}))
	t.Cleanup(server.Close)

	pods, err := NewClient(server.URL, 0, nil, WithKubecost("c")).GetPodAllocations(context.Background(), runnersNamespace)

	require.NoError(t, err)
	assert.InDelta(t, 0.0, pods.ForApplication(runnerClaims).OpenCostOutput.AllocationTotals.StorageCost, 1e-9)
}
