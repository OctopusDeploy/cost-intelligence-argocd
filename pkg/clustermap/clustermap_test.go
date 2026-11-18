package clustermap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/exporter"
)

const euServer = "https://ABC123.gr7.eu-west-1.eks.amazonaws.com"

func testMap(t *testing.T) *Map {
	t.Helper()
	m, err := New("eks-hub", []Cluster{
		{ID: "eks-prod-eu", Server: euServer, Name: "prod-eu"},
		{ID: "eks-staging", Name: "staging"},
		{ID: "eks-dev", Server: "https://dev.example.com"},
	})
	require.NoError(t, err)
	return m
}

func TestClusterFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		dest   exporter.Destination
		wantID string
		wantOK bool
	}{
		{name: "no destination is the local cluster", dest: exporter.Destination{}, wantID: "eks-hub", wantOK: true},
		{name: "in-cluster by server", dest: exporter.Destination{Server: exporter.LocalServer}, wantID: "eks-hub", wantOK: true},
		{name: "in-cluster by name", dest: exporter.Destination{Name: exporter.LocalName}, wantID: "eks-hub", wantOK: true},
		{name: "by server", dest: exporter.Destination{Server: euServer}, wantID: "eks-prod-eu", wantOK: true},
		{name: "by name", dest: exporter.Destination{Name: "prod-eu"}, wantID: "eks-prod-eu", wantOK: true},
		{name: "a cluster mapped by name only", dest: exporter.Destination{Name: "staging"}, wantID: "eks-staging", wantOK: true},
		{name: "a cluster mapped by server only", dest: exporter.Destination{Server: "https://dev.example.com"}, wantID: "eks-dev", wantOK: true},
		{name: "server unknown, name known", dest: exporter.Destination{Server: "https://other", Name: "prod-eu"}, wantID: "eks-prod-eu", wantOK: true},
		{name: "unmapped server", dest: exporter.Destination{Server: "https://unknown"}},
		{name: "unmapped name", dest: exporter.Destination{Name: "unknown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id, ok := testMap(t).ClusterFor(tt.dest)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantID, id)
		})
	}
}

func TestNew_Invalid(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		local    string
		clusters []Cluster
		want     string
	}{
		"no local cluster":  {local: "", want: "local cluster is required"},
		"no id":             {local: "hub", clusters: []Cluster{{Name: "a"}}, want: "id is required"},
		"no server or name": {local: "hub", clusters: []Cluster{{ID: "a"}}, want: "server or name is required"},
		"server in two":     {local: "hub", clusters: []Cluster{{ID: "a", Server: "s"}, {ID: "b", Server: "s"}}, want: `server "s" is mapped to both`},
		"name in two":       {local: "hub", clusters: []Cluster{{ID: "a", Name: "n"}, {ID: "b", Name: "n"}}, want: `name "n" is mapped to both`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := New(tt.local, tt.clusters)

			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestNew_TheSameClusterMayHaveSeveralEntries(t *testing.T) {
	t.Parallel()

	m, err := New("hub", []Cluster{{ID: "a", Server: "s"}, {ID: "a", Name: "n"}})

	require.NoError(t, err)
	id, ok := m.ClusterFor(exporter.Destination{Name: "n"})
	assert.True(t, ok)
	assert.Equal(t, "a", id)
}

func TestParse(t *testing.T) {
	t.Parallel()

	m, err := Parse("hub", []byte(`{"clusters":[{"id":"eks-prod-eu","server":"`+euServer+`","name":"prod-eu"}]}`))

	require.NoError(t, err)
	id, ok := m.ClusterFor(exporter.Destination{Name: "prod-eu"})
	assert.True(t, ok)
	assert.Equal(t, "eks-prod-eu", id)
}

func TestParse_Invalid(t *testing.T) {
	t.Parallel()

	for name, data := range map[string]string{
		"not json":      `clusters: []`,
		"unknown field": `{"clusters":[{"id":"a","name":"n","cluster":"typo"}]}`,
		"unknown key":   `{"cluster":[]}`,
	} {
		_, err := Parse("hub", []byte(data))
		require.ErrorContains(t, err, "invalid cluster map", name)
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("no file is the local cluster only", func(t *testing.T) {
		t.Parallel()

		m, err := Load("hub", "")

		require.NoError(t, err)
		_, ok := m.ClusterFor(exporter.Destination{Name: "prod-eu"})
		assert.False(t, ok)
	})
	t.Run("a file", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "clusters.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"clusters":[{"id":"a","name":"n"}]}`), 0o600))

		m, err := Load("hub", path)

		require.NoError(t, err)
		id, ok := m.ClusterFor(exporter.Destination{Name: "n"})
		assert.True(t, ok)
		assert.Equal(t, "a", id)
	})
	t.Run("a missing file", func(t *testing.T) {
		t.Parallel()

		_, err := Load("hub", filepath.Join(t.TempDir(), "absent.json"))

		require.ErrorContains(t, err, "reading cluster map")
	})
}

func TestClusters_ListsTheEntries(t *testing.T) {
	t.Parallel()

	got := testMap(t).Clusters()

	require.Len(t, got, 3)
	assert.Equal(t, Cluster{ID: "eks-prod-eu", Server: euServer, Name: "prod-eu"}, got[0])
}

func TestMap_ClusterFor_NameIgnoresCase(t *testing.T) {
	t.Parallel()

	m, err := New("hub", []Cluster{{ID: "va", Name: "VA/PREX"}})
	require.NoError(t, err)

	id, ok := m.ClusterFor(exporter.Destination{Name: "va/prex"})

	assert.True(t, ok)
	assert.Equal(t, "va", id)
}
