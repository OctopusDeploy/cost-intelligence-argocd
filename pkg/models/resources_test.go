package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedResources_OwnsPod(t *testing.T) {
	t.Parallel()

	resources := ManagedResources{
		{Kind: "Deployment", Name: "web", Namespace: "shop"},
		{Kind: "StatefulSet", Name: "db", Namespace: "shop"},
		{Kind: "CronJob", Name: "report", Namespace: "shop"},
		{Kind: "Rollout", Name: "api", Namespace: "shop"},
		{Kind: "Pod", Name: "debug", Namespace: "shop"},
		{Kind: "DaemonSet", Name: "agent"},
		{Kind: "Cluster", Name: "pg", Namespace: "shop"},
		{Kind: "Workflow", Name: "etl", Namespace: "shop"},
	}

	tests := []struct {
		name                                 string
		namespace, kind, controller, podName string
		want                                 bool
	}{
		{"deployment", "shop", "deployment", "web", "web-7d4b8c-xk", true},
		{"kind compared case-insensitively", "shop", "Deployment", "web", "web-7d4b8c-xk", true},
		{"same name, other kind", "shop", "statefulset", "web", "web-0", false},
		{"same name, other namespace", "billing", "deployment", "web", "web-7d4b8c-xk", false},
		{"statefulset", "shop", "statefulset", "db", "db-0", true},
		{"job of the cronjob", "shop", "job", "report-29312345", "report-29312345-abcde", true},
		{"cronjob reported as itself", "shop", "cronjob", "report", "report-29312345-abcde", true},
		{"job named like the cronjob", "shop", "job", "report-weekly", "report-weekly-abcde", false},
		{"rollout reported as itself", "shop", "rollout", "api", "api-6f7c9-x", true},
		{"replicaset of the rollout", "shop", "replicaset", "api-6f7c9b8d5", "api-6f7c9b8d5-x", true},
		{"replicaset named like the rollout", "shop", "replicaset", "api-gateway", "api-gateway-x", false},
		{"replicaset of the deployment", "shop", "replicaset", "web-7d4b8c9f5", "web-7d4b8c9f5-xk", true},
		{"replicaset named like the deployment", "shop", "replicaset", "web-api", "web-api-xk", false},
		{"deployment pod reported without controller", "shop", "", "", "web-7d4b8c9f5-xk2lm", true},
		{"statefulset pod reported without controller", "shop", "", "", "db-0", true},
		{"pod named like the deployment but not its", "shop", "", "", "web-api-xk2lm", false},
		{"cloudnativepg cluster pod", "shop", "cluster", "pg", "pg-1", true},
		{"cloudnativepg cluster pod reported without controller", "shop", "", "", "pg-2", true},
		{"cloudnativepg cluster of another kind", "shop", "statefulset", "pg", "pg-1", false},
		{"custom resource owning pods directly", "shop", "workflow", "etl", "etl-step-1", true},
		{"bare pod", "shop", "", "", "debug", true},
		{"bare pod of another name", "shop", "", "", "debug-2", false},
		{"pod of that name with a controller", "shop", "job", "cleanup", "debug", false},
		{"resource listed without a namespace", "anywhere", "daemonset", "agent", "agent-x", true},
		{"unmanaged controller", "shop", "deployment", "argocd-redis", "argocd-redis-x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, resources.OwnsPod(tt.namespace, tt.kind, tt.controller, tt.podName))
		})
	}
}

func TestManagedResources_OwnerOfLabeled(t *testing.T) {
	t.Parallel()

	resources := ManagedResources{
		{Kind: "ClickHouseInstallation", Name: "clickhouse-prdata", Namespace: "data"},
		{Kind: "ZookeeperCluster", Name: "zookeeper-prdata", Namespace: "data"},
		{Kind: "Deployment", Name: "web", Namespace: "data"},
	}
	chPod := map[string]string{"clickhouse_altinity_com_chi": "clickhouse-prdata", "app": "clickhouse"}
	zkPod := map[string]string{"app": "zookeeper-prdata", "release": "zookeeper-prdata", "kind": "ZookeeperMember"}

	owner, ok := resources.OwnerOfLabeled("data", "statefulset", "chi-clickhouse-prdata-replicated-0-1", "chi-clickhouse-prdata-replicated-0-1-0", chPod)
	assert.True(t, ok)
	assert.Equal(t, "clickhouse-prdata", owner.Name)

	// Labels as the backend may report them, unsanitised.
	assert.True(t, resources.OwnsPodLabeled("data", "statefulset", "chi-x", "chi-x-0", map[string]string{"clickhouse.altinity.com/chi": "clickhouse-prdata"}))

	owner, ok = resources.OwnerOfLabeled("data", "statefulset", "zookeeper-prdata", "zookeeper-prdata-1", zkPod)
	assert.True(t, ok)
	assert.Equal(t, "zookeeper-prdata", owner.Name)

	assert.False(t, resources.OwnsPodLabeled("other", "statefulset", "chi-x", "chi-x-0", chPod), "other namespace")
	assert.False(t, resources.OwnsPodLabeled("data", "statefulset", "chi-x", "chi-x-0", nil), "no labels")
	assert.False(t, resources.OwnsPodLabeled("data", "statefulset", "x", "x-0", map[string]string{"app": "web"}), "a workload is not claimed by label")
	assert.False(t, resources.OwnsPodLabeled("data", "statefulset", "x", "x-0", map[string]string{"clickhouse_altinity_com_chi": "other"}))
}

func TestManagedResources_OpenTelemetryOperator(t *testing.T) {
	t.Parallel()

	resources := ManagedResources{
		{Kind: "OpenTelemetryCollector", Name: "otel", Namespace: "obs"},
		{Kind: "TargetAllocator", Name: "otel", Namespace: "obs"},
	}

	assert.True(t, resources.OwnsPod("obs", "deployment", "otel-collector", "otel-collector-7d4b8c9f5-xk2lm"))
	assert.True(t, resources.OwnsPod("obs", "replicaset", "otel-collector-7d4b8c9f5", "otel-collector-7d4b8c9f5-xk2lm"))
	assert.True(t, resources.OwnsPod("obs", "daemonset", "otel-collector", "otel-collector-abcde"))
	assert.True(t, resources.OwnsPod("obs", "deployment", "otel-targetallocator", "otel-targetallocator-7d4b8c9f5-xk2lm"))
	assert.True(t, resources.OwnsPod("obs", "", "", "otel-targetallocator-7d4b8c9f5-xk2lm"), "no controller reported")
	assert.True(t, resources.OwnsPod("obs", "", "", "otel-collector-0"), "statefulset pod, no controller reported")
	assert.False(t, resources.OwnsPod("obs", "deployment", "otel-collector-gateway", "otel-collector-gateway-7d4b8c9f5-xk2lm"))
	assert.False(t, resources.OwnsPod("other", "deployment", "otel-collector", "otel-collector-7d4b8c9f5-xk2lm"))

	assert.True(t, resources.OwnsPodLabeled("obs", "deployment", "x", "x-1", map[string]string{"app.kubernetes.io/instance": "obs.otel"}), "instance label is <namespace>.<name>")
}

func TestUnownedPodLabels(t *testing.T) {
	require.Error(t, ConfigureUnownedPodLabels("=Airflow"))
	require.NoError(t, ConfigureUnownedPodLabels("managedby=Airflow"))
	t.Cleanup(ResetOwnership)

	resources := ManagedResources{{Kind: "Deployment", Name: "airflow-scheduler", Namespace: "airflow"}}
	bare := map[string]string{"managedby": "Airflow"}

	assert.True(t, resources.OwnsUnownedPod("airflow", "", "", bare))
	assert.True(t, resources.OwnsUnownedPod("airflow", "pod", "dbt-build-x1", bare), "a pod is its own controller")
	assert.False(t, resources.OwnsUnownedPod("other", "", "", bare), "other namespace")
	assert.False(t, resources.OwnsUnownedPod("airflow", "", "", map[string]string{"managedby": "Helm"}), "other value")
	assert.False(t, resources.OwnsUnownedPod("airflow", "deployment", "foo", bare), "has a controller")
	assert.False(t, ManagedResources{}.OwnsUnownedPod("airflow", "", "", bare), "no resource deploys there")
	assert.False(t, resources.OwnsPodLabeled("airflow", "", "", "dbt-build-x1", bare), "never by the labels alone: only the sole application owns these")
}

func TestConfigureOwnership(t *testing.T) {
	// Not parallel: it changes the shared rules, which the cleanup restores.
	t.Cleanup(ResetOwnership)
	require.Error(t, ConfigureOwnership("=Foo", ""))
	require.Error(t, ConfigureOwnership("foo.io/cluster=", ""))
	require.NoError(t, ConfigureOwnership("foo.io/cluster=Foo|FooShard, example.com/owner", "Widget"))

	resources := ManagedResources{{Kind: "Foo", Name: "f1", Namespace: "ns"}, {Kind: "Bar", Name: "b1", Namespace: "ns"}}
	assert.True(t, resources.OwnsPodLabeled("ns", "statefulset", "gen", "gen-0", map[string]string{"foo.io/cluster": "f1"}))
	assert.False(t, resources.OwnsPodLabeled("ns", "statefulset", "gen", "gen-0", map[string]string{"foo.io/cluster": "b1"}), "wrong kind")
	assert.True(t, resources.OwnsPodLabeled("ns", "statefulset", "gen", "gen-0", map[string]string{"example.com/owner": "b1"}))
	assert.Equal(t, ManagedResources{}, ManagedResources{{Kind: "Widget", Name: "w"}}.PodOwners())
}

func TestConfigureOwnership_FailureChangesNothingAndBuiltinsStayIntact(t *testing.T) {
	// Not parallel: it changes the shared rules, which the cleanup restores.
	t.Cleanup(ResetOwnership)

	// The first entry is valid, the second is not: neither may apply.
	require.Error(t, ConfigureOwnership("foo.io/cluster=Foo,=Bad", "Widget"))
	resources := ManagedResources{{Kind: "Foo", Name: "f1", Namespace: "ns"}}
	assert.False(t, resources.OwnsPodLabeled("ns", "statefulset", "gen", "gen-0", map[string]string{"foo.io/cluster": "f1"}))
	assert.Len(t, ManagedResources{{Kind: "Widget", Name: "w"}}.PodOwners(), 1)

	require.NoError(t, ConfigureOwnership("foo.io/cluster=Foo", "Widget"))
	assert.True(t, resources.OwnsPodLabeled("ns", "statefulset", "gen", "gen-0", map[string]string{"foo.io/cluster": "f1"}))

	ResetOwnership()
	assert.False(t, resources.OwnsPodLabeled("ns", "statefulset", "gen", "gen-0", map[string]string{"foo.io/cluster": "f1"}))
	assert.NotContains(t, builtinKindOwnerLabels, labelKey("foo.io/cluster"))
	assert.NotContains(t, builtinNonPodOwnerKinds, "widget")
}

func TestManagedResources_PodOwners(t *testing.T) {
	t.Parallel()

	resources := ManagedResources{
		{Kind: "Service", Name: "redis"},
		{Kind: "Deployment", Name: "redis"},
		{Kind: "ConfigMap", Name: "redis-config"},
		{Kind: "CronJob", Name: "test-cronjob"},
		{Kind: "Cluster", Name: "pg"},
		{Kind: "Workflow", Name: "etl"},
	}

	assert.Equal(t, ManagedResources{{Kind: "Deployment", Name: "redis"}, {Kind: "CronJob", Name: "test-cronjob"}, {Kind: "Cluster", Name: "pg"}, {Kind: "Workflow", Name: "etl"}}, resources.PodOwners())
	assert.Equal(t, ManagedResources{}, ManagedResources(nil).PodOwners(), "never nil")

	odd := ManagedResources{{Kind: "Widget", Name: "Not A Name"}, {Kind: "Deployment", Name: "Bad Name"}}
	assert.Equal(t, ManagedResources{{Kind: "Deployment", Name: "Bad Name"}}, odd.PodOwners(), "a custom resource with a free-form name is skipped, a workload's is kept to fail Valid")
	assert.False(t, odd.PodOwners().Valid())
}

func TestManagedResources_MeasuredAddsClaimsButNeverAsPodOwners(t *testing.T) {
	t.Parallel()

	resources := ManagedResources{
		{Kind: "Service", Name: "web"},
		{Kind: "Deployment", Name: "web"},
		{Kind: "PersistentVolumeClaim", Name: "builds-1", Namespace: "ci"},
		{Kind: "PersistentVolumeClaim", Name: "elsewhere", Namespace: "other"},
	}

	assert.Equal(t, ManagedResources{resources[1], resources[2], resources[3]}, resources.Measured())
	assert.Equal(t, ManagedResources{resources[1]}, resources.Measured().PodOwners())
	assert.Equal(t, map[string]bool{"builds-1": true}, resources.Claims("ci"))
}

func TestManagedResources_Valid(t *testing.T) {
	t.Parallel()

	assert.True(t, ManagedResources{{Kind: "Deployment", Name: "redis"}, {Kind: "Pod", Name: "a.b-c"}}.Valid())
	assert.True(t, ManagedResources{}.Valid())
	assert.False(t, ManagedResources{{Kind: "Deployment", Name: "Redis"}}.Valid())
	assert.False(t, ManagedResources{{Kind: "Deployment", Name: ""}}.Valid())
	assert.False(t, ManagedResources{{Kind: "Deployment", Name: `redis","x`}}.Valid())
}

func TestCostActual_ForApplication(t *testing.T) {
	t.Parallel()

	record := CostActual{
		ID:               "r1",
		TotalMonthlyCost: 9.55,
		Pods:             []PodCost{{Name: "argocd-redis-1"}, {Name: "redis-1"}},
		Efficiency:       &EfficiencyMetrics{CPUEfficiency: 0.02},
	}

	namespace := record.ForApplication()
	assert.Equal(t, ScopeNamespace, namespace.Scope)
	assert.InDelta(t, 9.55, namespace.TotalMonthlyCost, 1e-9)

	record.App = &AppMeasurement{
		TotalMonthlyCost: 0.63,
		OpenCostOutput:   OpenCostMetrics{AllocationTotals: AllocationTotal{TotalCost: 0.63}},
		Efficiency:       &EfficiencyMetrics{CPUEfficiency: 0.96},
		Pods:             []PodCost{{Name: "redis-1"}},
	}
	app := record.ForApplication()
	assert.Equal(t, ScopeApplication, app.Scope)
	assert.Equal(t, "r1", app.ID)
	assert.InDelta(t, 0.63, app.TotalMonthlyCost, 1e-9)
	assert.InDelta(t, 0.63, app.OpenCostOutput.AllocationTotals.TotalCost, 1e-9)
	assert.InDelta(t, 0.96, app.Efficiency.CPUEfficiency, 1e-9)
	assert.Equal(t, []PodCost{{Name: "redis-1"}}, app.Pods)
	assert.Nil(t, app.App, "the stored measurement is not sent twice")
	assert.InDelta(t, 9.55, record.TotalMonthlyCost, 1e-9, "the record itself is unchanged")
}
