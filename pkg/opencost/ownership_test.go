package opencost

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/models"
)

func ownershipPods() *PodAllocations {
	start := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	alloc := func(pod, kind, controller string, labels map[string]string) AllocationData {
		return AllocationData{
			Start: start, End: start.Add(24 * time.Hour), Minutes: 24 * 60, CPUCost: 1, TotalCost: 1,
			Properties: AllocationProperties{Namespace: "airflow", Pod: pod, ControllerKind: kind, Controller: controller, Labels: labels},
		}
	}
	return &PodAllocations{
		start: start, end: start.Add(24 * time.Hour),
		resp: AllocationResponse{Data: []map[string]AllocationData{{
			"web":    alloc("web-abc-1", "deployment", "web", nil),
			"task":   alloc("task-1", "", "", map[string]string{"kubernetes_executor": "True"}),
			"bare":   alloc("bare-1", "", "", nil),
			"worker": alloc("worker-1", "job", "worker", map[string]string{"team": "data", "project_runner_gitlab_com_name": "etl"}),
		}}},
	}
}

func TestForApplication_UnownedPodsGoToTheSoleApplication(t *testing.T) {
	t.Cleanup(models.ResetOwnership)
	require.NoError(t, models.ConfigureUnownedPodLabels("kubernetes_executor=True"))
	resources := models.ManagedResources{{Kind: "Deployment", Name: "web", Namespace: "airflow"}}

	pods := ownershipPods()
	names := func(m *models.AppMeasurement) (n []string) {
		for _, p := range m.Pods {
			n = append(n, p.Name)
		}
		return n
	}
	assert.Equal(t, []string{"web-abc-1"}, names(pods.ForApplication(resources)), "not the sole application")

	pods.SoleApplication(true)
	assert.ElementsMatch(t, []string{"web-abc-1", "task-1"}, names(pods.ForApplication(resources)), "the sole application owns the labelled unowned pod, not every bare pod")
	assert.InDelta(t, 2.0/24, pods.HourlyCostOf(resources).CPU, 1e-9, "the hourly cost counts it too")
}

func TestPods_GroupLabels(t *testing.T) {
	pods := ownershipPods()
	assert.Empty(t, pods.Pods()[0].Group, "no grouping labels")

	client := NewClient("http://x", 0, nil, WithPodGroupLabels([]string{" team ", "project.runner.gitlab.com/name"}))
	pods.groupLabels = client.groupLabels
	groups := map[string]string{}
	for _, p := range pods.Pods() {
		groups[p.Name] = p.Group
	}
	assert.Equal(t, "data / etl", groups["worker-1"])
	assert.Empty(t, groups["web-abc-1"])
}

func TestParseOrphanedPods_KeepsLabels(t *testing.T) {
	start := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	pods := parseOrphanedPods(AllocationResponse{Data: []map[string]AllocationData{{
		"task": {
			Start: start, End: start.Add(24 * time.Hour), Minutes: 24 * 60, CPUCost: 1, TotalCost: 1,
			Properties: AllocationProperties{Namespace: "airflow", Pod: "task-1", Labels: map[string]string{"a": "b"}},
		},
	}}})
	require.Len(t, pods, 1)
	assert.Equal(t, map[string]string{"a": "b"}, pods[0].Labels)
}

func TestForApplication_ApplicationsSharingANamespaceNeverClaimTheSamePods(t *testing.T) {
	start := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	alloc := func(pod, kind, controller string, cost float64, labels map[string]string) AllocationData {
		return AllocationData{
			Start: start, End: start.Add(24 * time.Hour), Minutes: 24 * 60, CPUCost: cost, TotalCost: cost,
			Properties: AllocationProperties{Namespace: "shop", Pod: pod, ControllerKind: kind, Controller: controller, Labels: labels},
		}
	}
	pods := &PodAllocations{
		start: start, end: start.Add(24 * time.Hour),
		resp: AllocationResponse{Data: []map[string]AllocationData{{
			"web": alloc("web-1", "deployment", "web", 2, map[string]string{"app": "web"}),
			"gen": alloc("gen-0", "statefulset", "chi-gen-0", 7, map[string]string{"app": "gen"}),
			"db":  alloc("db-0", "statefulset", "chi-db-0", 11, map[string]string{"app": "db"}),
		}}},
	}
	routes := models.ManagedResources{
		{Kind: "HTTPRoute", Name: "web", Namespace: "shop"}, {Kind: "ScaledObject", Name: "gen", Namespace: "shop"}, {Kind: "Cluster", Name: "db", Namespace: "shop"},
	}
	web := models.ManagedResources{{Kind: "Deployment", Name: "web", Namespace: "shop"}, {Kind: "Gateway", Name: "gen", Namespace: "shop"}}

	cost := func(own, peers models.ManagedResources) float64 {
		pods.Peers(peers)
		return pods.HourlyCostOf(own).CPU * 24
	}

	assert.InDelta(t, 20.0, cost(routes, nil), 1e-9, "without the peers, the route takes the Deployment's pod and the generated pod")
	assert.InDelta(t, 11.0, cost(routes, web), 1e-9)
	assert.InDelta(t, 2.0, cost(web, routes), 1e-9)

	pods.Peers(web)
	measured := pods.ForApplication(routes)
	assert.InDelta(t, 11.0*30, measured.TotalMonthlyCost, 1e-9, "the application's measurement follows the same rule")
}
