package clustermap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/exporter"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestDiscovery(list func(context.Context) ([]string, error)) (*Discovery, *clock) {
	c := &clock{t: time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)}
	d := NewDiscovery(list, 10*time.Minute)
	d.now = c.now
	return d, c
}

func TestDiscovery_ReadsOnceUntilTheNamesAreStale(t *testing.T) {
	t.Parallel()

	calls := 0
	d, c := newTestDiscovery(func(context.Context) ([]string, error) { calls++; return []string{"eks-a", "eks-b"}, nil })

	assert.Equal(t, []string{"eks-a", "eks-b"}, d.IDs())
	assert.Equal(t, []string{"eks-a", "eks-b"}, d.IDs())
	assert.Equal(t, 1, calls)

	c.t = c.t.Add(11 * time.Minute)
	d.IDs()
	assert.Equal(t, 2, calls)
}

func TestDiscovery_AFailureKeepsTheLastNamesAndWaitsBeforeAsking(t *testing.T) {
	t.Parallel()

	calls, fail := 0, false
	d, c := newTestDiscovery(func(context.Context) ([]string, error) {
		calls++
		if fail {
			return nil, errors.New("aggregator down")
		}
		return []string{"eks-a"}, nil
	})
	d.IDs()
	fail = true
	c.t = c.t.Add(11 * time.Minute)

	assert.Equal(t, []string{"eks-a"}, d.IDs(), "the last names")
	assert.Equal(t, 2, calls)
	d.IDs()
	assert.Equal(t, 2, calls, "not asked again at once")
	c.t = c.t.Add(2 * time.Minute)
	d.IDs()
	assert.Equal(t, 3, calls)
}

func TestDiscovery_MatchIgnoresCase(t *testing.T) {
	t.Parallel()

	d, _ := newTestDiscovery(func(context.Context) ([]string, error) { return []string{"eks-prod-eu"}, nil })

	id, ok := d.Match("EKS-Prod-EU")
	assert.True(t, ok)
	assert.Equal(t, "eks-prod-eu", id)
	_, ok = d.Match("prod")
	assert.False(t, ok)
	_, ok = d.Match("")
	assert.False(t, ok)
}

func TestClusterFor_TakesAClusterNamedAsTheBackendNamesIt(t *testing.T) {
	t.Parallel()

	d, _ := newTestDiscovery(func(context.Context) ([]string, error) { return []string{"eks-prod-us", "eks-prod-eu"}, nil })
	m := testMap(t).WithDiscovery(d)

	id, ok := m.ClusterFor(exporter.Destination{Name: "eks-prod-us"})
	require.True(t, ok)
	assert.Equal(t, "eks-prod-us", id, "named as Kubecost names it")

	id, ok = m.ClusterFor(exporter.Destination{Name: "prod-eu"})
	require.True(t, ok)
	assert.Equal(t, "eks-prod-eu", id, "an entry of the map comes first")

	_, ok = m.ClusterFor(exporter.Destination{Name: "unknown"})
	assert.False(t, ok)
	_, ok = m.ClusterFor(exporter.Destination{Server: "https://eks-prod-us"})
	assert.False(t, ok, "a server URL is not a name")
}
