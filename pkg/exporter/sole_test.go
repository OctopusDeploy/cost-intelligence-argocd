package exporter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/models"
)

func TestSoleApplication(t *testing.T) {
	app := func(name, destination string, workloadNamespaces ...string) Application {
		a := Application{
			Ref:         models.AppRef{Namespace: "argocd", Name: name},
			Destination: Destination{Server: "https://kubernetes.default.svc", Namespace: destination},
		}
		for _, ns := range workloadNamespaces {
			a.Workloads = append(a.Workloads, models.ManagedResource{Kind: "Deployment", Name: name, Namespace: ns})
		}
		return a
	}
	remote := app("far", "airflow")
	remote.Destination = Destination{Server: "https://elsewhere.example.com", Namespace: "airflow"}

	tests := []struct {
		name string
		apps []Application
		want string
		ok   bool
	}{
		{"none", []Application{app("web", "shop")}, "", false},
		{"by destination", []Application{app("airflow", "airflow"), app("web", "shop")}, "airflow", true},
		{"by workload", []Application{app("airflow", "default", "airflow")}, "airflow", true},
		{"several", []Application{app("a", "airflow"), app("b", "default", "airflow")}, "", false},
		{"another cluster does not count", []Application{app("airflow", "airflow"), remote}, "airflow", true},
		{"only another cluster", []Application{remote}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sole, ok := SoleApplication(tt.apps, "airflow")
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, sole.Name)
		})
	}
}
