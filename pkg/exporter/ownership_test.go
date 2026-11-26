package exporter

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/models"
	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/opencost"
)

// podCost is a pod of the "shop" namespace with a CPU cost for the whole window.
type podCost struct {
	name, kind, controller string
	cpu                    float64
	labels                 map[string]string
}

// openCostSource measures through a real OpenCost client against a server with the pods of "shop", so
// that the exporter runs the ownership rules the API does.
type openCostSource struct{ client *opencost.Client }

func (s openCostSource) RecentCosts(ctx context.Context, namespace string, window time.Duration) (Costs, error) {
	return s.client.GetRecentPodAllocations(ctx, namespace, window)
}

func shopCosts(t *testing.T, pods ...podCost) openCostSource {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		end := time.Now()
		allocations := map[string]opencost.AllocationData{}
		for _, p := range pods {
			allocations[p.name] = opencost.AllocationData{
				Start: end.Add(-time.Hour), End: end, Minutes: 60, CPUCost: p.cpu, TotalCost: p.cpu,
				Properties: opencost.AllocationProperties{Pod: p.name, Namespace: "shop", ControllerKind: p.kind, Controller: p.controller, Labels: p.labels},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(opencost.AllocationResponse{Code: 200, Data: []map[string]opencost.AllocationData{allocations}})
	}))
	t.Cleanup(srv.Close)
	return openCostSource{opencost.NewClient(srv.URL, 5*time.Second, nil)}
}

// cpuByApplication is what the exporter exports for CPU, by Application name.
func cpuByApplication(t *testing.T, source CostSource, apps ...Application) map[string]float64 {
	t.Helper()
	e := New(&fakeApps{apps: apps}, source, &fakeEstimates{}, time.Minute, log.New(io.Discard, "", 0), prometheus.NewRegistry())
	cpu := map[string]float64{}
	for _, s := range e.costSamples(context.Background(), apps) {
		if s.desc == hourlyCostDesc && s.labels[2] == "cpu" {
			cpu[s.labels[1]] += s.value
		}
	}
	return cpu
}

func shopApplication(name string, resources ...models.ManagedResource) Application {
	return Application{
		Ref:         models.AppRef{Namespace: "argocd", Name: name},
		Destination: Destination{Server: LocalServer, Namespace: "shop"},
		Workloads:   resources,
	}
}

// Not parallel: the unowned pod labels are process-wide.
func TestCostSamples_TheOnlyApplicationOwnsTheLabelledBarePods(t *testing.T) {
	t.Cleanup(models.ResetOwnership)
	require.NoError(t, models.ConfigureUnownedPodLabels("kubernetes_executor=True"))
	source := shopCosts(t,
		podCost{name: "web-1", kind: "deployment", controller: "web", cpu: 2},
		podCost{name: "task-1", cpu: 3, labels: map[string]string{"kubernetes_executor": "True"}},
		podCost{name: "bare-1", cpu: 5},
	)
	web := shopApplication("web", workload("Deployment", "web", "shop"))
	batch := shopApplication("batch", workload("Deployment", "batch", "shop"))

	assert.InDelta(t, 5.0, cpuByApplication(t, source, web)["web"], 1e-9, "alone in the namespace: its pod and the labelled bare pod")
	assert.InDelta(t, 2.0, cpuByApplication(t, source, web, batch)["web"], 1e-9, "with another application: a bare pod is no one's")
}

func TestCostSamples_TwoApplicationsNeverClaimTheSamePods(t *testing.T) {
	source := shopCosts(t,
		podCost{name: "web-1", kind: "deployment", controller: "web", cpu: 2, labels: map[string]string{"app": "web"}},
		podCost{name: "gen-0", kind: "statefulset", controller: "chi-gen-0", cpu: 7, labels: map[string]string{"app": "gen"}},
		podCost{name: "db-0", kind: "statefulset", controller: "chi-db-0", cpu: 11, labels: map[string]string{"app": "db"}},
	)
	routes := shopApplication("routes",
		workload("HTTPRoute", "web", "shop"), workload("ScaledObject", "gen", "shop"), workload("Cluster", "db", "shop"))
	web := shopApplication("web", workload("Deployment", "web", "shop"), workload("Gateway", "gen", "shop"))

	cpu := cpuByApplication(t, source, routes, web)

	assert.InDelta(t, 11.0, cpu["routes"], 1e-9, "only the pod no other application matches")
	assert.InDelta(t, 2.0, cpu["web"], 1e-9, "its Deployment's pod, and not the pod both match by label")
	assert.LessOrEqual(t, cpu["routes"]+cpu["web"], 20.0, "never more than the namespace")
}

func TestPeerWorkloads(t *testing.T) {
	t.Parallel()

	web := shopApplication("web", workload("Deployment", "web", "shop"))
	routes := shopApplication("routes", workload("HTTPRoute", "web", "shop"), workload("Deployment", "other", "elsewhere"))
	remote := shopApplication("remote", workload("Deployment", "far", "shop"))
	remote.Destination = Destination{Server: "https://elsewhere.example.com", Namespace: "shop"}
	claims := shopApplication("claims", workload("PersistentVolumeClaim", "web", "shop"))
	apps := []Application{web, routes, remote, shopApplication("config"), claims}

	assert.Equal(t, models.ManagedResources{workload("HTTPRoute", "web", "shop")}, PeerWorkloads(apps, LocalOnly{}, "", "shop", web.Ref),
		"the others' resources in the namespace: not its own, not another cluster's, not another namespace's, no claims")
	assert.Equal(t, models.ManagedResources{workload("Deployment", "web", "shop")}, PeerWorkloads(apps, LocalOnly{}, "", "shop", routes.Ref),
		"an application without workloads adds none")
	assert.Equal(t, models.ManagedResources{}, PeerWorkloads([]Application{web}, LocalOnly{}, "", "shop", web.Ref), "never nil")
}
