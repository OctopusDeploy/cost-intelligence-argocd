package hook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/exporter"
	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/models"
	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/opencost"
)

// sharedPod is a pod of the "shop" namespace the ownership tests measure.
type sharedPod struct {
	name, kind, controller string
	cost                   float64
	labels                 map[string]string
}

// newShopOpenCost serves the pods of one namespace, "shop", at a whole day each.
func newShopOpenCost(t *testing.T, pods ...sharedPod) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/allocation" {
			_ = json.NewEncoder(w).Encode(opencost.SummaryAllocationResponse{Code: 200})
			return
		}
		allocations := map[string]opencost.AllocationData{}
		var total float64
		for _, p := range pods {
			total += p.cost
			allocations[p.name] = measuredDay(opencost.AllocationData{
				CPUCost: p.cost, TotalCost: p.cost,
				Properties: opencost.AllocationProperties{Pod: p.name, Namespace: "shop", ControllerKind: p.kind, Controller: p.controller, Labels: p.labels},
			})
		}
		if r.URL.Query().Get("aggregate") != "pod" {
			allocations = map[string]opencost.AllocationData{"shop": measuredDay(opencost.AllocationData{CPUCost: total, TotalCost: total})}
		}
		_ = json.NewEncoder(w).Encode(opencost.AllocationResponse{Code: 200, Data: []map[string]opencost.AllocationData{allocations}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

type listedApps []exporter.Application

func (a listedApps) List(context.Context) ([]exporter.Application, error) { return a, nil }

func shopApp(name string, resources ...models.ManagedResource) exporter.Application {
	return exporter.Application{
		Ref:         models.AppRef{Namespace: "argocd", Name: name},
		Destination: exporter.Destination{Server: exporter.LocalServer, Namespace: "shop"},
		Workloads:   resources,
	}
}

// syncApp runs the webhook of one application through the hook and returns the record it saved.
func syncApp(t *testing.T, hook *SyncHook, app exporter.Application) {
	t.Helper()
	notification := &models.SyncNotification{
		ApplicationName: app.Ref.Name, ApplicationNamespace: app.Ref.Namespace, Namespace: "shop",
		SyncResult: "Succeeded", SyncTimestamp: time.Now(), Resources: app.Workloads,
	}
	require.NoError(t, hook.OnApplicationSync(context.Background(), notification))
}

func hookFor(t *testing.T, srv *httptest.Server, saved map[string]*models.CostActual, apps ...exporter.Application) *SyncHook {
	t.Helper()
	store := &mockStorage{saveActualFn: func(_ context.Context, actual *models.CostActual) error {
		saved[actual.ApplicationName] = actual
		return nil
	}}
	return newTestHook(t, srv, store).WithApplications(listedApps(apps))
}

func routeAndDeployment() (route, deployment models.ManagedResource) {
	return models.ManagedResource{Kind: "HTTPRoute", Name: "web", Namespace: "shop"},
		models.ManagedResource{Kind: "Deployment", Name: "web", Namespace: "shop"}
}

// Not parallel: the unowned pod labels are process-wide.
func TestOnApplicationSync_TheOnlyApplicationOwnsTheLabelledBarePods(t *testing.T) {
	t.Cleanup(models.ResetOwnership)
	require.NoError(t, models.ConfigureUnownedPodLabels("kubernetes_executor=True"))
	srv := newShopOpenCost(t,
		sharedPod{name: "web-1", kind: "deployment", controller: "web", cost: 2},
		sharedPod{name: "task-1", cost: 3, labels: map[string]string{"kubernetes_executor": "True"}},
		sharedPod{name: "bare-1", cost: 5},
	)
	web := shopApp("web", models.ManagedResource{Kind: "Deployment", Name: "web", Namespace: "shop"})

	alone := map[string]*models.CostActual{}
	syncApp(t, hookFor(t, srv, alone, web), web)
	require.NotNil(t, alone["web"].App)
	assert.InDelta(t, 150.0, alone["web"].App.TotalMonthlyCost, 1e-6, "its pod and the labelled bare pod: (2+3) a day for 30 days")
	assert.InDelta(t, 300.0, alone["web"].TotalMonthlyCost, 1e-6, "the namespace has the unlabelled pod too")

	shared := map[string]*models.CostActual{}
	syncApp(t, hookFor(t, srv, shared, web, shopApp("batch", models.ManagedResource{Kind: "Deployment", Name: "batch", Namespace: "shop"})), web)
	assert.InDelta(t, 60.0, shared["web"].App.TotalMonthlyCost, 1e-6, "with another application in the namespace, a bare pod is no one's")
}

func TestOnApplicationSync_TwoApplicationsNeverClaimTheSamePods(t *testing.T) {
	route, deployment := routeAndDeployment()
	srv := newShopOpenCost(t,
		// Carries the label of both the HTTPRoute and the Deployment, and is the Deployment's.
		sharedPod{name: "web-1", kind: "deployment", controller: "web", cost: 2, labels: map[string]string{"app": "web"}},
		// An operator's pod, matched by label by two applications that each list a custom resource of
		// that name: it belongs to neither.
		sharedPod{name: "gen-0", kind: "statefulset", controller: "chi-gen-0", cost: 7, labels: map[string]string{"app": "gen"}},
		// An operator's pod only one application matches.
		sharedPod{name: "db-0", kind: "statefulset", controller: "chi-db-0", cost: 11, labels: map[string]string{"app": "db"}},
	)
	routes := shopApp("routes", route,
		models.ManagedResource{Kind: "ScaledObject", Name: "gen", Namespace: "shop"},
		models.ManagedResource{Kind: "Cluster", Name: "db", Namespace: "shop"})
	web := shopApp("web", deployment, models.ManagedResource{Kind: "Gateway", Name: "gen", Namespace: "shop"})

	saved := map[string]*models.CostActual{}
	hook := hookFor(t, srv, saved, routes, web)
	syncApp(t, hook, routes)
	syncApp(t, hook, web)

	require.NotNil(t, saved["routes"].App)
	require.NotNil(t, saved["web"].App)
	assert.InDelta(t, 11*30.0, saved["routes"].App.TotalMonthlyCost, 1e-6, "only the pod nobody else matches")
	assert.InDelta(t, 2*30.0, saved["web"].App.TotalMonthlyCost, 1e-6, "its Deployment's pod, not the one the other matches by label too")
	sum := saved["routes"].App.TotalMonthlyCost + saved["web"].App.TotalMonthlyCost
	assert.LessOrEqual(t, sum, saved["web"].TotalMonthlyCost, "the applications never add up to more than the namespace")
	assert.InDelta(t, 20*30.0, saved["web"].TotalMonthlyCost, 1e-6)
}
