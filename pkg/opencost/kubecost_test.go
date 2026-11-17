package opencost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordQuery serves body and records the query of the request it received.
func recordQuery(t *testing.T, body string) (*httptest.Server, *url.Values) {
	t.Helper()
	got := &url.Values{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

const emptyAllocation = `{"code":200,"data":[{}]}`

func TestPrepare_OpenCostSendsTheParametersItAlwaysDid(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, emptyAllocation)
	client := NewClient(srv.URL, 5*time.Second, nil)

	_, _ = client.GetNamespaceCost(context.Background(), "web")

	assert.Equal(t, `namespace:"web"`, got.Get("filter"))
	assert.Equal(t, "true", got.Get("includeRawAllocation"))
	assert.False(t, got.Has("includeIdle"), "OpenCost leaves idle out by default")
	assert.False(t, got.Has("accumulate"))
}

func TestPrepare_KubecostAccumulatesAndLeavesIdleOut(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, emptyAllocation)
	client := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-prod-1"))

	_, _ = client.GetNamespaceCost(context.Background(), "web")

	assert.Equal(t, "true", got.Get("accumulate"), "without it a 24h window comes back as one set per day")
	assert.Equal(t, "false", got.Get("includeIdle"), "Kubecost includes idle unless told not to")
	assert.False(t, got.Has("includeRawAllocation"), "Kubecost documents no such parameter")
}

func TestPrepare_KubecostScopesEveryQueryToItsCluster(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, emptyAllocation)
	client := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-prod-1"))

	_, _ = client.GetNamespaceCost(context.Background(), "web")

	assert.Equal(t, `cluster:"eks-prod-1"+namespace:"web"`, got.Get("filter"),
		"a namespace of another cluster with the same name must not be summed in")
}

func TestPrepare_KubecostAddsTheClusterFilterWhenThereIsNoOther(t *testing.T) {
	t.Parallel()

	params := url.Values{}
	c := NewClient("http://kubecost", time.Second, nil, WithKubecost("eks-prod-1"))

	c.prepare(context.Background(), params, false)

	assert.Equal(t, `cluster:"eks-prod-1"`, params.Get("filter"))
}

func TestPrepare_KubecostDailyKeepsTheSetsOfEachDay(t *testing.T) {
	t.Parallel()

	params := url.Values{}
	params.Set("step", "1d")
	c := NewClient("http://kubecost", time.Second, nil, WithKubecost("eks-prod-1"))

	c.prepare(context.Background(), params, true)

	assert.False(t, params.Has("accumulate"), "the daily history reads one set per day")
	assert.False(t, params.Has("step"), "Kubecost documents step only for its compute endpoint")
	assert.Equal(t, "false", params.Get("includeIdle"))
}

func TestPrepare_KubecostWithoutClusterIDLeavesTheFilterAlone(t *testing.T) {
	t.Parallel()

	params := url.Values{}
	params.Set("filter", `namespace:"web"`)
	c := NewClient("http://kubecost", time.Second, nil, WithKubecost(""))

	c.prepare(context.Background(), params, false)

	assert.Equal(t, `namespace:"web"`, params.Get("filter"))
}

func TestGetClusterOverhead_KubecostAccumulatesAndScopesToTheCluster(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, `{"code":200,"data":{}}`)
	client := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-prod-1"))

	_, err := client.GetClusterOverhead(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "true", got.Get("accumulate"), "without it Kubecost answers with a list of daily sets")
	assert.Equal(t, `cluster:"eks-prod-1"`, got.Get("filter"))
}

func TestGetClusterOverhead_OpenCostSendsNoKubecostParameters(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, `{"code":200,"data":{}}`)
	client := NewClient(srv.URL, 5*time.Second, nil)

	_, err := client.GetClusterOverhead(context.Background())

	require.NoError(t, err)
	assert.False(t, got.Has("accumulate"))
	assert.False(t, got.Has("filter"))
}

func TestAssetsResponse_ReadsASetOrAListOfSets(t *testing.T) {
	t.Parallel()

	asset := `"eks/fee":{"type":"ClusterManagement","properties":{"provider":"AWS","cluster":"eks-prod-1"},"totalCost":2.4}`
	tests := map[string]string{
		"one set":      `{"code":200,"data":{` + asset + `}}`,
		"a list":       `{"code":200,"data":[{` + asset + `}]}`,
		"several sets": `{"code":200,"data":[{},{` + asset + `}]}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got AssetsResponse
			require.NoError(t, got.UnmarshalJSON([]byte(body)))

			require.Len(t, got.Data, 1)
			assert.Equal(t, "ClusterManagement", got.Data["eks/fee"].Type)
			assert.InDelta(t, 2.4, got.Data["eks/fee"].TotalCost, 1e-9)
		})
	}
}

func TestAssetsResponse_NoDataIsEmpty(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`{"code":200}`, `{"code":200,"data":null}`, `{"code":200,"data":[]}`} {
		var got AssetsResponse
		require.NoError(t, got.UnmarshalJSON([]byte(body)), body)
		assert.Empty(t, got.Data, body)
	}
}

func TestWithCluster_ReplacesTheClientsOwnCluster(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, emptyAllocation)
	client := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-prod-1"))

	_, _ = client.GetNamespaceCost(WithCluster(context.Background(), "eks-prod-eu"), "web")

	assert.Equal(t, `cluster:"eks-prod-eu"+namespace:"web"`, got.Get("filter"))
}

func TestWithCluster_EmptyKeepsTheClientsOwnCluster(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, emptyAllocation)
	client := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-prod-1"))

	_, _ = client.GetNamespaceCost(WithCluster(context.Background(), ""), "web")

	assert.Equal(t, `cluster:"eks-prod-1"+namespace:"web"`, got.Get("filter"))
}

func TestWithCluster_OpenCostIgnoresIt(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, emptyAllocation)
	client := NewClient(srv.URL, 5*time.Second, nil)

	_, _ = client.GetNamespaceCost(WithCluster(context.Background(), "eks-prod-eu"), "web")

	assert.Equal(t, `namespace:"web"`, got.Get("filter"))
}

func TestWithHeader_SendsTheKeyOnEveryRequest(t *testing.T) {
	t.Parallel()

	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{}}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-prod-1"), WithHeader("Authorization", "Bearer s3cret"))

	_, _ = client.GetNamespaceCost(context.Background(), "web")
	_, _ = client.GetClusterOverhead(context.Background())

	assert.Equal(t, []string{"Bearer s3cret", "Bearer s3cret"}, got)
}

func TestWithHeader_KeepsTheTransportItWraps(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, emptyAllocation)
	used := false
	inner := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})
	client := NewClient(srv.URL, 5*time.Second, nil, WithHeader("X-Api-Key", "k"), WithTransport(inner))

	_, _ = client.GetNamespaceCost(context.Background(), "web")

	assert.True(t, used, "the instrumented transport still sees the request")
	assert.Equal(t, `namespace:"web"`, got.Get("filter"))
}

func TestNewClient_WithoutKeySendsNoHeader(t *testing.T) {
	t.Parallel()

	var header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(emptyAllocation))
	}))
	t.Cleanup(srv.Close)

	_, _ = NewClient(srv.URL, 5*time.Second, nil, WithHeader("Authorization", "")).GetNamespaceCost(context.Background(), "web")

	assert.Empty(t, header)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGetDataQuality_KubecostHasNoPricingSourceEndpoint(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	t.Cleanup(srv.Close)

	quality, err := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-prod-1")).GetDataQuality(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "unknown", quality.PricingSource)
	assert.NotContains(t, quality.Message, "404")
	assert.False(t, called, "nothing to ask Kubecost")
}

func TestListClusters_NamesEveryClusterKubecostKnows(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, `{"code":200,"data":[{"eks-hub":{},"eks-eu":{},"__idle__":{}},{"eks-eu":{}}]}`)
	client := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-hub"))

	clusters, err := client.ListClusters(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"eks-eu", "eks-hub"}, clusters)
	assert.Equal(t, "cluster", got.Get("aggregate"))
	assert.False(t, got.Has("filter"), "not scoped to the client's own cluster")
}

func TestListClusters_OpenCostKnowsOnlyItsOwn(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, emptyAllocation)

	clusters, err := NewClient(srv.URL, 5*time.Second, nil).ListClusters(context.Background())

	require.NoError(t, err)
	assert.Empty(t, clusters)
	assert.Empty(t, *got, "nothing is asked")
}

func TestListClusters_ReportsAnAggregatorError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	t.Cleanup(srv.Close)

	_, err := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-hub")).ListClusters(context.Background())

	assert.ErrorContains(t, err, "403")
}

func TestGetClusterOverhead_FollowsTheClusterOfTheRequest(t *testing.T) {
	t.Parallel()

	srv, got := recordQuery(t, `{"code":200,"data":{}}`)
	client := NewClient(srv.URL, 5*time.Second, nil, WithKubecost("eks-hub"))

	_, err := client.GetClusterOverhead(WithCluster(context.Background(), "eks-eu"))

	require.NoError(t, err)
	assert.Equal(t, `cluster:"eks-eu"`, got.Get("filter"))
}
