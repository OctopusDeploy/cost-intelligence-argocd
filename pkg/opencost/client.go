package opencost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"math"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/models"
)

type Client struct {
	baseURL string
	client  *http.Client
	logger  *log.Logger

	// kubecost adapts requests to Kubecost's allocation and assets APIs; see WithKubecost.
	kubecost bool
	// clusterID restricts every query to one cluster, as a Kubecost aggregator answers for all of them.
	clusterID string
	// header is sent with every request, for a backend behind an API key. Its value is a secret.
	headerName, headerValue string
	// groupLabels are the pod labels, sanitized as OpenCost reports them, whose values name a pod's group.
	groupLabels []string
	// shareTenancy and shareNamespaces ask Kubecost to spread cluster-level (tenancy) costs and the
	// cost of the named namespaces over the other allocations; see WithSharedCosts.
	shareTenancy    bool
	shareNamespaces []string

	// cloudCostURL is where Kubecost's /cloudCost/status is read; empty uses baseURL.
	cloudCostURL string

	// volumeMu guards volumeCache: the claims of a cluster's volumes, by cluster, read once for a while.
	volumeMu    sync.Mutex
	volumeCache map[string]volumeClaims
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithTransport sends the client's requests through transport, for example to instrument them.
func WithTransport(transport http.RoundTripper) ClientOption {
	return func(c *Client) { c.client.Transport = transport }
}

// WithKubecost makes the client query a Kubecost API instead of OpenCost's: base URL ending in /model,
// one accumulated set per window, no idle allocation, and only the cluster clusterID. Kubecost with an
// aggregator answers for every cluster it knows, so clusterID is required to keep a namespace that
// exists in several clusters from being summed across them.
func WithKubecost(clusterID string) ClientOption {
	return func(c *Client) {
		c.kubecost = true
		c.clusterID = clusterID
	}
}

// WithCloudCostURL sets the base URL of the Kubecost service that serves /cloudCost/status. The
// aggregator doesn't (it answers 403 or 404), so with one as the client's base URL the pricing source
// is read here; for example http://kubecost-cloud-cost.kubecost:9005. Empty uses the base URL.
func WithCloudCostURL(u string) ClientOption {
	return func(c *Client) { c.cloudCostURL = strings.TrimRight(strings.TrimSpace(u), "/") }
}

// WithSharedCosts has a Kubecost query spread costs no application owns over the allocations it
// returns, in proportion to their cost, and counts that share in the totals: tenancy costs (the
// cluster-level costs Kubecost reports as "Tenancy Costs") when tenancy is true, and the cost of each
// namespace in namespaces (for example kube-system). Kubecost spreads the tenancy costs; the
// namespaces are spread here, within the cluster being read, because Kubecost would pool them across
// every cluster. Without it nothing is shared and a namespace costs only what its own pods, volumes
// and network cost. A shared namespace is not measured itself.
func WithSharedCosts(tenancy bool, namespaces []string) ClientOption {
	return func(c *Client) {
		c.shareTenancy = tenancy
		for _, namespace := range namespaces {
			if namespace = strings.TrimSpace(namespace); namespace != "" {
				c.shareNamespaces = append(c.shareNamespaces, namespace)
			}
		}
	}
}

// WithPodGroupLabels names the pod labels (as Kubernetes spells them, for example
// project.runner.gitlab.com/name) whose values form PodCost.Group: the values present, joined by " / ".
// A pod with none has no group.
func WithPodGroupLabels(labels []string) ClientOption {
	return func(c *Client) {
		for _, label := range labels {
			if label = strings.TrimSpace(label); label != "" {
				c.groupLabels = append(c.groupLabels, sanitizeLabelKey(label))
			}
		}
	}
}

// sanitizeLabelKey spells a label key as OpenCost reports it: every character that is not a letter,
// a digit or an underscore becomes an underscore.
func sanitizeLabelKey(key string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, key)
}

// WithHeader sends name: value with every request, for example an API key. It is never logged.
func WithHeader(name, value string) ClientOption {
	return func(c *Client) { c.headerName, c.headerValue = name, value }
}

// headerTransport adds a header to each request it forwards, on a copy, as RoundTrippers must.
type headerTransport struct {
	next        http.RoundTripper
	name, value string
}

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set(t.name, t.value)
	return t.next.RoundTrip(clone)
}

func NewClient(baseURL string, timeout time.Duration, logger *log.Logger, opts ...ClientOption) *Client {
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	c := &Client{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.headerName != "" && c.headerValue != "" {
		next := c.client.Transport
		if next == nil {
			next = http.DefaultTransport
		}
		c.client.Transport = headerTransport{next: next, name: c.headerName, value: c.headerValue}
	}
	return c
}

type AllocationResponse struct {
	Code int                         `json:"code"`
	Data []map[string]AllocationData `json:"data"`
}

// AllocationProperties mirrors OpenCost's AllocationProperties struct.
// services is []string and labels/namespaceLabels are map[string]string,
// so this cannot be decoded as map[string]string.
type AllocationProperties struct {
	Cluster         string            `json:"cluster,omitempty"`
	Node            string            `json:"node,omitempty"`
	Container       string            `json:"container,omitempty"`
	Controller      string            `json:"controller,omitempty"`
	ControllerKind  string            `json:"controllerKind,omitempty"`
	Namespace       string            `json:"namespace,omitempty"`
	Pod             string            `json:"pod,omitempty"`
	ProviderID      string            `json:"providerID,omitempty"`
	Services        []string          `json:"services,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	NamespaceLabels map[string]string `json:"namespaceLabels,omitempty"`
}

type RawAllocationOnly struct {
	CPUCoreUsageMax float64 `json:"cpuCoreUsageMax"`
	RAMByteUsageMax float64 `json:"ramByteUsageMax"`
}

// SharedCostItem is one entry in the sharedCostBreakdown map returned by OpenCost.
// Each entry represents a shared namespace/controller and its cost contribution.
type SharedCostItem struct {
	TotalCost float64 `json:"totalCost"`
}

type AllocationData struct {
	Start                        time.Time                 `json:"start"`
	End                          time.Time                 `json:"end"`
	Minutes                      float64                   `json:"minutes"`
	CPUCoreHours                 float64                   `json:"cpuCoreHours"`
	CPUCoreRequestAverage        float64                   `json:"cpuCoreRequestAverage"`
	CPUCoreUsageAverage          float64                   `json:"cpuCoreUsageAverage"`
	CPUCoreLimitAverage          float64                   `json:"cpuCoreLimitAverage"`
	CPUCost                      float64                   `json:"cpuCost"`
	GPUHours                     float64                   `json:"gpuHours"`
	GPUCost                      float64                   `json:"gpuCost"`
	NetworkCost                  float64                   `json:"networkCost"`
	NetworkCrossZoneCost         float64                   `json:"networkCrossZoneCost"`
	NetworkCrossRegionCost       float64                   `json:"networkCrossRegionCost"`
	NetworkInternetCost          float64                   `json:"networkInternetCost"`
	NetworkNatGatewayEgressCost  float64                   `json:"networkNatGatewayEgressCost"`
	NetworkNatGatewayIngressCost float64                   `json:"networkNatGatewayIngressCost"`
	LoadBalancerCost             float64                   `json:"loadBalancerCost"`
	PVCost                       float64                   `json:"pvCost"`
	RAMByteHours                 float64                   `json:"ramByteHours"`
	RAMByteRequestAverage        float64                   `json:"ramByteRequestAverage"`
	RAMByteUsageAverage          float64                   `json:"ramByteUsageAverage"`
	RAMByteLimitAverage          float64                   `json:"ramByteLimitAverage"`
	RAMCost                      float64                   `json:"ramCost"`
	TotalCost                    float64                   `json:"totalCost"`
	SharedCost                   float64                   `json:"sharedCost"`
	SharedCostBreakdown          map[string]SharedCostItem `json:"sharedCostBreakdown,omitempty"`
	ExternalCost                 float64                   `json:"externalCost"`
	CarbonCost                   float64                   `json:"carbonCost"`
	// PVs are the volumes of an allocation by key, read for the unmounted volumes entry only and
	// cleared once read (unmountedVolumeCosts): a namespace can have hundreds, which are not stored.
	PVs               map[string]PVAllocation `json:"pvs,omitempty"`
	Properties        AllocationProperties    `json:"properties"`
	RawAllocationOnly *RawAllocationOnly      `json:"rawAllocationOnly,omitempty"`

	// countShared adds SharedCost to what the allocation costs. Set when the request asked for costs to
	// be shared (see Client.countShared); otherwise SharedCost is only reported, as before.
	countShared bool
}

// PVAllocation is the cost of one volume within an allocation.
type PVAllocation struct {
	Cost float64 `json:"cost"`
}

// componentCost is what an allocation costs in the resources this service reports: CPU, GPU, memory,
// storage, network and load balancers, the parts AllocationTotals.TotalCost adds up. OpenCost's own
// totalCost is not used: it rounds every field to five decimals on its own, so once a short period is
// projected onto a month it drifts from the sum of the parts shown beside it, and it carries shared
// and external costs that no view shows.
func (a AllocationData) componentCost() float64 {
	return a.baseCost() + a.countedShared()
}

// baseCost is componentCost without any shared cost.
func (a AllocationData) baseCost() float64 {
	return a.CPUCost + a.GPUCost + a.RAMCost + a.PVCost + a.NetworkCost + a.LoadBalancerCost
}

// countedShared is the share of tenancy and shared-namespace costs this allocation carries, when the
// request asked for it.
func (a AllocationData) countedShared() float64 {
	if !a.countShared {
		return 0
	}
	return a.SharedCost
}

// applySharing asks Kubecost to share its tenancy costs, on a request whose answer is priced (not a
// summary). Shared namespaces are not asked of Kubecost: it pools them across every cluster it knows
// (see shareNamespacesInCluster).
func (c *Client) applySharing(params url.Values) {
	if c.kubecost && c.shareTenancy {
		params.Set("shareTenancyCosts", "true")
	}
}

// countShared marks the allocations of a response so the tenancy cost Kubecost shared counts in the
// totals, when this client asked for it.
func (c *Client) countShared(resp *AllocationResponse) {
	if !c.kubecost || !c.shareTenancy {
		return
	}
	for _, set := range resp.Data {
		for key, alloc := range set {
			alloc.countShared = true
			set[key] = alloc
		}
	}
}

// shareNamespacesInCluster spreads the cost of the shared namespaces over the allocations of resp, in
// proportion to their cost, within the cluster being read. Kubecost's own shareNamespaces would pool
// the namespace of every cluster and spread it over all of them, so a cluster would pay for the
// kube-system of the others. The pool is read for the same window, one set per day when daily. If it
// cannot be read the response is left unshared, and a measured namespace that is itself shared
// receives nothing.
func (c *Client) shareNamespacesInCluster(ctx context.Context, resp *AllocationResponse, namespace, window string, daily bool) {
	if !c.kubecost || len(c.shareNamespaces) == 0 || slices.Contains(c.shareNamespaces, namespace) {
		return
	}
	params := url.Values{}
	params.Add("window", window)
	params.Add("aggregate", "namespace")
	c.prepare(ctx, params, daily)
	requestURL := fmt.Sprintf("%s/allocation?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost GET %s", requestURL)

	pool, err := c.readAllocations(ctx, requestURL)
	if err != nil {
		c.logf("WARN opencost: reading the cluster's costs to share namespaces: %v", err)
		return
	}
	if len(pool.Data) != len(resp.Data) {
		c.logf("WARN opencost: shared namespaces not applied: %d sets for the pool, %d for the query", len(pool.Data), len(resp.Data))
		return
	}
	for i, set := range resp.Data {
		sharedBy := map[string]float64{}
		var shared, others float64
		for key, alloc := range pool.Data[i] {
			switch {
			case isVirtualAllocation(key):
			case slices.Contains(c.shareNamespaces, key):
				shared += alloc.baseCost()
				sharedBy[key] += alloc.baseCost()
			default:
				others += alloc.baseCost()
			}
		}
		if shared <= 0 || others <= 0 {
			continue
		}
		for key, alloc := range set {
			if isVirtualAllocation(key) {
				continue
			}
			fraction := alloc.baseCost() / others
			alloc.SharedCost += shared * fraction
			breakdown := make(map[string]SharedCostItem, len(alloc.SharedCostBreakdown)+len(sharedBy))
			maps.Copy(breakdown, alloc.SharedCostBreakdown)
			for ns, cost := range sharedBy {
				name := "Namespace " + ns
				breakdown[name] = SharedCostItem{TotalCost: breakdown[name].TotalCost + cost*fraction}
			}
			alloc.SharedCostBreakdown = breakdown
			alloc.countShared = true
			set[key] = alloc
		}
	}
}

func (c *Client) readAllocations(ctx context.Context, requestURL string) (*AllocationResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencost request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, string(body))
	}
	var result AllocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode opencost response: %w", err)
	}
	return &result, nil
}

// prepare adapts the parameters of an allocation request to the provider. OpenCost's are left as
// they are. Kubecost differs in defaults that change the numbers without failing the request:
// idle is included (OpenCost: not), and a window comes back as one set per day unless accumulated.
// It documents neither includeRawAllocation nor step on this endpoint, so they are not sent. daily
// keeps the per-day sets, which the daily history reads one by one.
func (c *Client) prepare(ctx context.Context, params url.Values, daily bool) {
	if !c.kubecost {
		return
	}
	params.Set("includeIdle", "false")
	if !daily {
		params.Set("accumulate", "true")
	}
	params.Del("includeRawAllocation")
	params.Del("step")
	c.scopeToCluster(ctx, params)
}

type clusterKey struct{}

// WithCluster has the queries made with the returned context read one cluster of a Kubecost aggregator,
// instead of the client's own. An empty id leaves ctx as it is.
func WithCluster(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, clusterKey{}, id)
}

// clusterOf is the cluster a Kubecost query reads: the one ctx names, else the client's own.
func (c *Client) clusterOf(ctx context.Context) string {
	if id, ok := ctx.Value(clusterKey{}).(string); ok {
		return id
	}
	return c.clusterID
}

// scopeToCluster adds the cluster to the filter, joined to any filter already there with "+", Kubecost's AND.
func (c *Client) scopeToCluster(ctx context.Context, params url.Values) {
	clusterID := c.clusterOf(ctx)
	if clusterID == "" {
		return
	}
	filter := fmt.Sprintf("cluster:%q", clusterID)
	if existing := params.Get("filter"); existing != "" {
		filter += "+" + existing
	}
	params.Set("filter", filter)
}

func (c *Client) logf(format string, args ...any) {
	if c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

// monthHours is the length of the month costs are projected onto: 30 days.
const monthHours = 30 * 24

// defaultMonthlyFactor projects a full 24h window onto a 30-day month. It applies when
// OpenCost does not say which period it measured.
const defaultMonthlyFactor = 30.0

// monthlyFactor returns the multiplier that projects a cost OpenCost measured between start
// and end onto a 30-day month. OpenCost reports a namespace or pod only from when it first
// ran, so a namespace created an hour ago carries an hour of cost, not a day's: scaling by
// the full 24h window would put its monthly cost 24 times too low.
func monthlyFactor(start, end time.Time) float64 {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return defaultMonthlyFactor
	}
	return monthHours / end.Sub(start).Hours()
}

// MinMeasuredPeriod is the least OpenCost data a cost is projected from. OpenCost rounds costs to
// five decimals, and seconds of data project onto a month by a factor in the hundreds of thousands,
// so the rounding alone becomes dollars. Even above it the rounding is cents, which is why a
// namespace is measured from the same pods as its applications (PodAllocations.Namespace).
const MinMeasuredPeriod = 5 * time.Minute

// measuredPeriod returns the period OpenCost has data for across a response: the earliest
// start and latest end of its allocations. Pods are scaled by this shared period rather
// than each by its own, so a pod replaced during the window is not projected onto a whole
// month alongside its replacement, and the pods still add up to the namespace total. A namespace's
// claimed volumes that no pod mounts are left out: OpenCost dates that entry from the start of the
// window, whenever the volumes were created, so a namespace a few minutes old would be projected as
// if it had run all day, far below its own applications. Their cost still counts.
func measuredPeriod(resp AllocationResponse) (start, end time.Time) {
	for _, allocations := range resp.Data {
		for key, alloc := range allocations {
			if isVirtualAllocation(key) || isUnmountedVolumes(alloc) {
				continue
			}
			if !alloc.Start.IsZero() && (start.IsZero() || alloc.Start.Before(start)) {
				start = alloc.Start
			}
			if alloc.End.After(end) {
				end = alloc.End
			}
		}
	}
	return start, end
}

// last24h returns the 24 hours up to now, the period every point-in-time query covers.
func last24h() (start, end time.Time) {
	end = time.Now()
	return end.Add(-24 * time.Hour), end
}

// windowParam writes start and end as OpenCost's window parameter, in UTC: OpenCost rejects a
// window carrying a UTC offset (2026-10-01T18:43:58+03:00) as an "illegal window", so a backend
// whose local time zone is not UTC (TZ set on the pod, a laptop) would fail every query.
func windowParam(start, end time.Time) string {
	return start.UTC().Format(time.RFC3339) + "," + end.UTC().Format(time.RFC3339)
}

// isVirtualAllocation reports whether key names one of OpenCost's virtual entries, such as
// __idle__ or __unallocated__, rather than a real namespace, pod or label value.
func isVirtualAllocation(key string) bool {
	return len(key) > 2 && key[0] == '_' && key[len(key)-1] == '_'
}

// isUnmountedVolumes reports whether alloc is the entry OpenCost lists a namespace's claimed
// volumes that no pod mounts under, as one more pod (<namespace>-unmounted-pvcs).
func isUnmountedVolumes(alloc AllocationData) bool {
	ns := alloc.Properties.Namespace
	return ns != "" && alloc.Properties.Pod == ns+"-unmounted-pvcs"
}

func (c *Client) GetNamespaceCost(ctx context.Context, namespace string) (*models.OpenCostMetrics, error) {
	startTime, endTime := last24h()

	params := url.Values{}
	params.Add("window", windowParam(startTime, endTime))
	params.Add("aggregate", "namespace")
	params.Add("filter", fmt.Sprintf("namespace:%q", namespace))
	params.Add("includeRawAllocation", "true")

	c.prepare(ctx, params, false)
	c.applySharing(params)
	requestURL := fmt.Sprintf("%s/allocation?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost GET %s", requestURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencost request failed: %w", err)
	}
	defer resp.Body.Close()

	c.logf("DEBUG opencost namespace cost response: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, string(body))
	}

	var result AllocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode opencost response: %w", err)
	}
	c.countShared(&result)
	c.shareNamespacesInCluster(ctx, &result, namespace, params.Get("window"), false)

	metrics := parseOpenCostResponse(result, startTime, endTime)
	return metrics, nil
}

// SummaryAllocationResponse mirrors the real OpenCost /allocation/summary wire format:
//
//	{ "code": 200, "data": { "sets": [ { "allocations": { "<ns>": { ... } } } ] } }
type SummaryAllocationResponse struct {
	Code int                           `json:"code"`
	Data SummaryAllocationResponseData `json:"data"`
}

type SummaryAllocationResponseData struct {
	Sets []SummaryAllocationSet `json:"sets"`
}

type SummaryAllocationSet struct {
	Allocations map[string]SummaryAllocationData `json:"allocations"`
}

type SummaryAllocationData struct {
	Start                 time.Time `json:"start"`
	End                   time.Time `json:"end"`
	CPUCoreRequestAverage float64   `json:"cpuCoreRequestAverage"`
	CPUCoreUsageAverage   float64   `json:"cpuCoreUsageAverage"`
	CPUCoreLimitAverage   float64   `json:"cpuCoreLimitAverage"`
	CPUCost               float64   `json:"cpuCost"`
	RAMByteRequestAverage float64   `json:"ramByteRequestAverage"`
	RAMByteUsageAverage   float64   `json:"ramByteUsageAverage"`
	RAMByteLimitAverage   float64   `json:"ramByteLimitAverage"`
	RAMCost               float64   `json:"ramCost"`
}

func (c *Client) GetNamespaceSummary(ctx context.Context, namespace string) (*models.EfficiencyMetrics, error) {
	params := url.Values{}
	params.Add("window", windowParam(last24h()))
	params.Add("aggregate", "namespace")
	params.Add("filter", fmt.Sprintf("namespace:%q", namespace))

	c.prepare(ctx, params, false)
	requestURL := fmt.Sprintf("%s/allocation/summary?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost GET %s", requestURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencost summary request failed: %w", err)
	}
	defer resp.Body.Close()

	c.logf("DEBUG opencost summary response: %d", resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opencost summary returned status %d: %s", resp.StatusCode, string(body))
	}

	c.logf("DEBUG opencost summary body: %s", body)

	var result SummaryAllocationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode opencost summary response: %w", err)
	}

	metrics := parseSummaryResponse(result)
	c.logf("DEBUG opencost summary parsed: sets=%d → metrics=%v", len(result.Data.Sets), metrics != nil)
	return metrics, nil
}

// PodAllocations is a namespace's pod-level allocation over the last 24 hours, from which both the
// namespace's pods and one application's share of them are measured.
type PodAllocations struct {
	resp       AllocationResponse
	start, end time.Time
	// volumeCosts is what each unmounted volume cost, by PV name; claimOf names the claim of a PV. An
	// application that lists a claim owns that volume's cost (see owned). Both are empty when the
	// namespace has no unmounted volume or the claims could not be read.
	volumeCosts map[string]float64
	claimOf     map[string]claimRef
	// namespace is the one queried: the namespace of a pod whose allocation carries none (Kubecost
	// keeps only the properties every entry shares), for a resource that names its own.
	namespace string
	// groupLabels are the client's, see WithPodGroupLabels.
	groupLabels []string
	// soleApplication is set when exactly one application deploys to the namespace: every volume no pod
	// mounts is then its, listed in its resources or not. With several, only the claims an application
	// lists are its; the rest stay in the namespace's cost.
	soleApplication bool
}

// SoleApplication tells the measurement whether one application is the only one deploying to the
// namespace, so that it owns the volumes no pod mounts. Call it before measuring an application.
func (p *PodAllocations) SoleApplication(sole bool) {
	p.soleApplication = sole
}

func (c *Client) GetPodAllocations(ctx context.Context, namespace string) (*PodAllocations, error) {
	startTime, endTime := last24h()
	return c.podAllocations(ctx, namespace, startTime, endTime)
}

// GetRecentPodAllocations returns the pods of a namespace over the window up to now, for the cost
// rate of the applications deploying there.
func (c *Client) GetRecentPodAllocations(ctx context.Context, namespace string, window time.Duration) (*PodAllocations, error) {
	endTime := time.Now()
	return c.podAllocations(ctx, namespace, endTime.Add(-window), endTime)
}

func (c *Client) podAllocations(ctx context.Context, namespace string, startTime, endTime time.Time) (*PodAllocations, error) {
	params := url.Values{}
	params.Add("window", windowParam(startTime, endTime))
	params.Add("aggregate", "pod")
	params.Add("filter", fmt.Sprintf("namespace:%q", namespace))
	// Peak usage per pod, for an application's efficiency.
	params.Add("includeRawAllocation", "true")

	c.prepare(ctx, params, false)
	c.applySharing(params)
	requestURL := fmt.Sprintf("%s/allocation?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost GET %s", requestURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencost request failed: %w", err)
	}
	defer resp.Body.Close()

	c.logf("DEBUG opencost pod breakdown response: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, string(body))
	}

	var result AllocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode opencost response: %w", err)
	}
	c.countShared(&result)
	c.shareNamespacesInCluster(ctx, &result, namespace, params.Get("window"), false)

	pods := &PodAllocations{resp: result, start: startTime, end: endTime, namespace: namespace, groupLabels: c.groupLabels}
	pods.volumeCosts = unmountedVolumeCosts(result, namespace)
	if len(pods.volumeCosts) > 0 {
		claims, err := c.volumeClaims(ctx)
		if err != nil {
			// The volumes stay in the namespace's cost, unattributed to any application.
			c.logf("WARN opencost: reading volume claims: %v", err)
		}
		pods.claimOf = claims.byVolume
	}
	return pods, nil
}

// isUnmountedEntry is isUnmountedVolumes for an entry that may carry no namespace, as Kubecost's pod
// allocations keep only the properties every entry shares: the pod's name then says whose it is.
func isUnmountedEntry(alloc AllocationData, namespace string) bool {
	if isUnmountedVolumes(alloc) {
		return true
	}
	return namespace != "" && alloc.Properties.Namespace == "" && alloc.Properties.Pod == namespace+"-unmounted-pvcs"
}

// unmountedVolumeCosts reads what each volume of the unmounted volumes entry cost, by PV name, and
// clears every allocation's volume list.
func unmountedVolumeCosts(resp AllocationResponse, namespace string) map[string]float64 {
	costs := map[string]float64{}
	for _, allocations := range resp.Data {
		for key, alloc := range allocations {
			if len(alloc.PVs) == 0 {
				continue
			}
			if isUnmountedEntry(alloc, namespace) {
				for pv, v := range alloc.PVs {
					costs[pvName(pv)] += v.Cost
				}
			}
			alloc.PVs = nil
			allocations[key] = alloc
		}
	}
	return costs
}

// pvName is the PV name in a volume key, which Kubecost and OpenCost spell cluster=<c>:name=<pv>.
func pvName(key string) string {
	if i := strings.LastIndex(key, "name="); i >= 0 {
		return key[i+len("name="):]
	}
	return key
}

// GetContainerAllocations returns every container of every pod in a namespace over the last 24
// hours, with its requests, limits, usage and cost.
func (c *Client) GetContainerAllocations(ctx context.Context, namespace string) ([]models.ContainerAllocation, error) {
	startTime, endTime := last24h()

	params := url.Values{}
	params.Add("window", windowParam(startTime, endTime))
	params.Add("aggregate", "pod,container")
	params.Add("filter", fmt.Sprintf("namespace:%q", namespace))

	c.prepare(ctx, params, false)
	requestURL := fmt.Sprintf("%s/allocation?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost GET %s", requestURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencost request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, string(body))
	}

	var result AllocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode opencost response: %w", err)
	}
	return parseContainerAllocations(result), nil
}

// parseContainerAllocations keeps the allocations that are a real container of a real pod.
func parseContainerAllocations(resp AllocationResponse) []models.ContainerAllocation {
	containers := []models.ContainerAllocation{}
	for _, allocations := range resp.Data {
		for key, alloc := range allocations {
			props := alloc.Properties
			if isVirtualAllocation(key) || props.Pod == "" || props.Container == "" {
				continue
			}
			containers = append(containers, models.ContainerAllocation{
				Pod:             props.Pod,
				Container:       props.Container,
				ControllerKind:  props.ControllerKind,
				Controller:      props.Controller,
				Hours:           alloc.Minutes / 60,
				CPURequestCores: alloc.CPUCoreRequestAverage,
				CPUUsageCores:   alloc.CPUCoreUsageAverage,
				CPULimitCores:   alloc.CPUCoreLimitAverage,
				RAMRequestBytes: alloc.RAMByteRequestAverage,
				RAMUsageBytes:   alloc.RAMByteUsageAverage,
				RAMLimitBytes:   alloc.RAMByteLimitAverage,
				CPUCoreHours:    alloc.CPUCoreHours,
				RAMByteHours:    alloc.RAMByteHours,
				CPUCost:         alloc.CPUCost,
				RAMCost:         alloc.RAMCost,
			})
		}
	}
	sort.Slice(containers, func(i, j int) bool {
		if containers[i].Pod != containers[j].Pod {
			return containers[i].Pod < containers[j].Pod
		}
		return containers[i].Container < containers[j].Container
	})
	return containers
}

// Pods returns every pod in the namespace with its monthly cost, and its group when grouping labels
// are configured.
func (p *PodAllocations) Pods() []models.PodCost {
	pods := parsePodBreakdown(p.resp)
	if len(p.groupLabels) == 0 {
		return pods
	}
	groups := podGroups(p.resp, p.groupLabels)
	for i := range pods {
		pods[i].Group = groups[pods[i].Name]
	}
	return pods
}

// podGroups maps each pod to the values of labels it carries, joined by " / ". A pod with none is absent.
func podGroups(resp AllocationResponse, labels []string) map[string]string {
	groups := make(map[string]string)
	for _, allocations := range resp.Data {
		for key, alloc := range allocations {
			name := key
			if alloc.Properties.Pod != "" {
				name = alloc.Properties.Pod
			}
			if groups[name] != "" {
				continue
			}
			var values []string
			for _, label := range labels {
				if value := alloc.Properties.Labels[label]; value != "" {
					values = append(values, value)
				}
			}
			if len(values) > 0 {
				groups[name] = strings.Join(values, " / ")
			}
		}
	}
	return groups
}

// Efficiency measures every pod in the namespace as podEfficiency measures an application's: its
// idle cost is the unused share of each pod's request, projected onto a month from the period the
// namespace's pods ran. Nil without pods.
func (p *PodAllocations) Efficiency() *models.EfficiencyMetrics {
	return podEfficiency(p.resp)
}

// Namespace measures the whole namespace from its pods: their monthly cost and its breakdown, with
// the claimed volumes no pod mounts, which OpenCost lists in the namespace as one more pod
// (<namespace>-unmounted-pvcs). OpenCost rounds every allocation to five decimals on its own, so the
// namespace read once and read pod by pod differ by that rounding, which a short period projects onto
// a month as cents: measured from the same allocations, rounding never puts an application measured
// over the namespace's period above it. Without the raw allocations, as the pods are stored on their
// own. Nil when the answer measured no period, such as a namespace with no pods yet.
func (p *PodAllocations) Namespace() *models.OpenCostMetrics {
	metrics := parseOpenCostResponse(p.resp, p.start, p.end)
	if metrics.MeasuredHours == 0 {
		return nil
	}
	metrics.ResourceDetails = nil
	return metrics
}

// ForApplication measures the pods the application's resources own: their monthly cost and its
// breakdown, their efficiency and the pods themselves, projected onto a month from the period those
// pods ran rather than the namespace's, so a new application in an older namespace is not
// under-reported.
func (p *PodAllocations) ForApplication(resources models.ManagedResources) *models.AppMeasurement {
	owned := p.owned(resources)
	metrics := parseOpenCostResponse(owned, p.start, p.end)
	// The raw allocations are already stored once, with the namespace's measurement.
	metrics.ResourceDetails = nil

	return &models.AppMeasurement{
		Controllers:      resources,
		TotalMonthlyCost: metrics.AllocationTotals.TotalCost,
		OpenCostOutput:   *metrics,
		Efficiency:       podEfficiency(owned),
		Pods:             p.grouped(p.named(parsePodBreakdown(owned), resources), owned),
	}
}

// grouped sets the group (team, project) of the pods that carry the grouping labels, as Pods does for
// the namespace's, so an application's CI job pods read per team and project instead of one by one.
func (p *PodAllocations) grouped(pods []models.PodCost, resp AllocationResponse) []models.PodCost {
	if len(p.groupLabels) == 0 {
		return pods
	}
	groups := podGroups(resp, p.groupLabels)
	for i := range pods {
		pods[i].Group = groups[pods[i].Name]
	}
	return pods
}

// named gives a pod the provider reported without a controller the one that owns it, so the pods group
// under their controllers as OpenCost's do.
func (p *PodAllocations) named(pods []models.PodCost, resources models.ManagedResources) []models.PodCost {
	for i, pod := range pods {
		if pod.Controller != "" {
			continue
		}
		ns := pod.Namespace
		if ns == "" {
			ns = p.namespace
		}
		if owner, ok := resources.OwnerOf(ns, "", "", pod.Name); ok && !strings.EqualFold(owner.Kind, "pod") {
			pods[i].Controller, pods[i].ControllerKind = owner.Name, owner.Kind
		}
	}
	return pods
}

// HourlyCost is the cost per hour of a set of pods, by resource. The resources add up to the pods'
// cost, as OpenCost's total does: shared costs are not part of it. CPUIdle and MemoryIdle are the parts
// of CPU and Memory paid for requested capacity that went unused.
type HourlyCost struct {
	CPU, Memory, GPU, Storage, Network, LoadBalancer float64
	CPUIdle, MemoryIdle                              float64
}

// HourlyCostOf returns the average cost per hour of the pods the resources own, over the period the
// whole response measured. Every application in a namespace is divided by the same period, so their
// rates add up to the namespace's, and a pod that ran for part of it counts for that part.
func (p *PodAllocations) HourlyCostOf(resources models.ManagedResources) HourlyCost {
	hours := p.end.Sub(p.start).Hours()
	if start, end := measuredPeriod(p.resp); !start.IsZero() && end.After(start) {
		hours = end.Sub(start).Hours()
	}
	var cost HourlyCost
	if hours <= 0 {
		return cost
	}
	for _, allocations := range p.owned(resources).Data {
		for _, a := range allocations {
			cost.CPU += a.CPUCost
			cost.Memory += a.RAMCost
			cost.GPU += a.GPUCost
			cost.Storage += a.PVCost
			cost.Network += a.NetworkCost
			cost.LoadBalancer += a.LoadBalancerCost
			cost.CPUIdle += unusedCost(a.CPUCost, a.CPUCoreRequestAverage, a.CPUCoreUsageAverage)
			cost.MemoryIdle += unusedCost(a.RAMCost, a.RAMByteRequestAverage, a.RAMByteUsageAverage)
		}
	}
	for _, v := range []*float64{&cost.CPU, &cost.Memory, &cost.GPU, &cost.Storage, &cost.Network, &cost.LoadBalancer, &cost.CPUIdle, &cost.MemoryIdle} {
		*v /= hours
	}
	return cost
}

// owned is ownedAllocations, plus the volumes the application's PersistentVolumeClaims name that no pod
// mounts. OpenCost lists those, for the whole namespace, as one more pod; here the entry holds only the
// cost of the volumes the application lists, so each claim's cost is counted once, under the application
// that deploys it. A claim is matched by its name and, when the provider reports it, its namespace, and
// only when no other claim in the cluster has that name: a name that repeats could be any of them.
func (p *PodAllocations) owned(resources models.ManagedResources) AllocationResponse {
	owned := ownedAllocations(p.resp, p.namespace, resources.PodOwners(), p.soleApplication)
	claims := resources.Claims(p.namespace)
	var cost float64
	if !p.soleApplication {
		if len(claims) == 0 || len(p.volumeCosts) == 0 {
			return owned
		}
		for pv, c := range p.volumeCosts {
			if ref, ok := p.claimOf[pv]; ok && claims[ref.name] && (ref.namespace == "" || ref.namespace == p.namespace) {
				cost += c
			}
		}
		if cost <= 0 {
			return owned
		}
	}
	for i, allocations := range p.resp.Data {
		for _, alloc := range allocations {
			if !isUnmountedEntry(alloc, p.namespace) {
				continue
			}
			// Capped at the entry: rounding must not put the application above its namespace. The
			// namespace's only application owns the whole entry.
			if p.soleApplication {
				cost = alloc.PVCost
			} else {
				cost = math.Min(cost, alloc.PVCost)
			}
			if cost <= 0 {
				return owned
			}
			owned.Data[i][p.namespace+"-unmounted-pvcs"] = AllocationData{
				Start:      alloc.Start,
				End:        alloc.End,
				Minutes:    alloc.Minutes,
				PVCost:     cost,
				TotalCost:  cost,
				Properties: AllocationProperties{Cluster: alloc.Properties.Cluster, Namespace: p.namespace, Pod: p.namespace + "-unmounted-pvcs"},
			}
			return owned
		}
	}
	return owned
}

// ownedAllocations keeps the pod allocations the resources own, dropping OpenCost's virtual entries. The
// namespace's only application (sole) also owns its pods without a controller that carry an unowned pod
// label: with several applications each would claim them.
func ownedAllocations(resp AllocationResponse, namespace string, resources models.ManagedResources, sole bool) AllocationResponse {
	owned := AllocationResponse{Code: resp.Code, Data: make([]map[string]AllocationData, 0, len(resp.Data))}
	for _, allocations := range resp.Data {
		kept := make(map[string]AllocationData)
		for key, alloc := range allocations {
			if isVirtualAllocation(key) {
				continue
			}
			pod := alloc.Properties.Pod
			if pod == "" {
				pod = key
			}
			ns := alloc.Properties.Namespace
			if ns == "" {
				ns = namespace
			}
			if resources.OwnsPodLabeled(ns, alloc.Properties.ControllerKind, alloc.Properties.Controller, pod, alloc.Properties.Labels) ||
				sole && resources.OwnsUnownedPod(ns, alloc.Properties.ControllerKind, alloc.Properties.Controller, alloc.Properties.Labels) {
				kept[key] = alloc
			}
		}
		owned.Data = append(owned.Data, kept)
	}
	return owned
}

// podEfficiency measures how much of what a set of pods requests they use, and what the unused part
// of the request costs. Returns nil without pods.
//
// Requests, usage and limits are averaged over the period the pods ran, weighting each pod by its
// minutes, so a pod replaced during the window and its replacement count as one. Peaks are summed,
// which overstates them when pods replaced each other rather than ran side by side: a recommendation
// sized from them errs towards headroom.
//
// OpenCost's own idle fields (cpuCostIdle, ramCostIdle) are never computed: up to 1.121.3 they are only
// copied and summed, so they are zero on every allocation. A pod using less than it requests pays for
// its request, so its idle cost is the unused share of its cost.
func podEfficiency(resp AllocationResponse) *models.EfficiencyMetrics {
	periodStart, periodEnd := measuredPeriod(resp)
	periodMinutes := 24 * 60.0
	if !periodStart.IsZero() && periodEnd.After(periodStart) {
		periodMinutes = periodEnd.Sub(periodStart).Minutes()
	}

	var e models.EfficiencyMetrics
	var cpuCost, ramCost, cpuIdle, ramIdle float64
	pods := 0
	for _, allocations := range resp.Data {
		for key, a := range allocations {
			if isVirtualAllocation(key) {
				continue
			}
			pods++
			share := 1.0
			if a.Minutes > 0 {
				share = math.Min(a.Minutes/periodMinutes, 1)
			}
			e.CPUCoresRequested += a.CPUCoreRequestAverage * share
			e.CPUCoresUsed += a.CPUCoreUsageAverage * share
			e.CPUCoresLimit += a.CPUCoreLimitAverage * share
			e.RAMBytesRequested += a.RAMByteRequestAverage * share
			e.RAMBytesUsed += a.RAMByteUsageAverage * share
			e.RAMBytesLimit += a.RAMByteLimitAverage * share
			if a.RawAllocationOnly != nil {
				e.CPUCoresUsedMax += a.RawAllocationOnly.CPUCoreUsageMax
				e.RAMBytesUsedMax += a.RawAllocationOnly.RAMByteUsageMax
			}
			cpuCost += a.CPUCost
			ramCost += a.RAMCost
			cpuIdle += unusedCost(a.CPUCost, a.CPUCoreRequestAverage, a.CPUCoreUsageAverage)
			ramIdle += unusedCost(a.RAMCost, a.RAMByteRequestAverage, a.RAMByteUsageAverage)
		}
	}
	if pods == 0 {
		return nil
	}

	setRequestEfficiency(&e, cpuCost, ramCost)

	monthlyMultiplier := monthlyFactor(periodStart, periodEnd)
	e.CPUIdleCost = cpuIdle * monthlyMultiplier
	e.RAMIdleCost = ramIdle * monthlyMultiplier
	return &e
}

// setRequestEfficiency sets how much of their CPU and memory requests pods use, from the requests and
// usage already in e. The total is weighted by cost, as OpenCost weighs its own. A resource the pods
// request none of has no efficiency, so it stays out of the total rather than counting as 0%, or as
// 100% as OpenCost counts it; without any request the total is 0.
func setRequestEfficiency(e *models.EfficiencyMetrics, cpuCost, ramCost float64) {
	var weighted, weight float64
	if e.CPUCoresRequested > 0 {
		e.CPUEfficiency = e.CPUCoresUsed / e.CPUCoresRequested
		weighted += e.CPUEfficiency * cpuCost
		weight += cpuCost
	}
	if e.RAMBytesRequested > 0 {
		e.RAMEfficiency = e.RAMBytesUsed / e.RAMBytesRequested
		weighted += e.RAMEfficiency * ramCost
		weight += ramCost
	}
	if weight > 0 {
		e.TotalEfficiency = weighted / weight
	}
}

// unusedCost is the part of a cost paid for requested capacity that went unused.
func unusedCost(cost, request, usage float64) float64 {
	if request <= 0 || usage >= request {
		return 0
	}
	return cost * (request - usage) / request
}

func (c *Client) GetOrphanedPods(ctx context.Context, namespace string) ([]models.OrphanedPod, error) {
	params := url.Values{}
	params.Add("window", windowParam(last24h()))
	params.Add("aggregate", "pod")
	params.Add("filter", fmt.Sprintf("namespace:%q", namespace))

	c.prepare(ctx, params, false)
	requestURL := fmt.Sprintf("%s/allocation?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost GET %s", requestURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencost request failed: %w", err)
	}
	defer resp.Body.Close()

	c.logf("DEBUG opencost orphaned pods response: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, string(body))
	}

	var result AllocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode opencost response: %w", err)
	}

	return parseOrphanedPods(result), nil
}

func (c *Client) GetChargeback(ctx context.Context, labelKey, namespace string) ([]models.ChargebackEntry, error) {
	params := url.Values{}
	params.Add("window", windowParam(last24h()))
	params.Add("aggregate", "label:"+labelKey)
	if namespace != "" {
		params.Add("filter", fmt.Sprintf("namespace:%q", namespace))
	}

	c.prepare(ctx, params, false)
	allocURL := fmt.Sprintf("%s/allocation?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost chargeback alloc GET %s", allocURL)

	allocReq, err := http.NewRequestWithContext(ctx, http.MethodGet, allocURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create allocation request: %w", err)
	}
	allocResp, err := c.client.Do(allocReq)
	if err != nil {
		return nil, fmt.Errorf("opencost allocation request failed: %w", err)
	}
	defer allocResp.Body.Close()

	c.logf("DEBUG opencost chargeback alloc response: %d", allocResp.StatusCode)

	if allocResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(allocResp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", allocResp.StatusCode, string(body))
	}
	var allocResult AllocationResponse
	if err := json.NewDecoder(allocResp.Body).Decode(&allocResult); err != nil {
		return nil, fmt.Errorf("failed to decode allocation response: %w", err)
	}

	c.prepare(ctx, params, false)
	summaryURL := fmt.Sprintf("%s/allocation/summary?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost chargeback summary GET %s", summaryURL)

	summaryReq, err := http.NewRequestWithContext(ctx, http.MethodGet, summaryURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create summary request: %w", err)
	}
	summaryResp, err := c.client.Do(summaryReq)
	if err != nil {
		return nil, fmt.Errorf("opencost summary request failed: %w", err)
	}
	defer summaryResp.Body.Close()

	c.logf("DEBUG opencost chargeback summary response: %d", summaryResp.StatusCode)

	var summaryResult SummaryAllocationResponse
	if summaryResp.StatusCode == http.StatusOK {
		_ = json.NewDecoder(summaryResp.Body).Decode(&summaryResult)
	}

	return parseChargebackEntries(allocResult, summaryResult), nil
}

func parseChargebackEntries(alloc AllocationResponse, summary SummaryAllocationResponse) []models.ChargebackEntry {
	// Efficiency by label value, measured as an application's is (setRequestEfficiency) rather than
	// read from OpenCost, which counts pods that request nothing as 100% efficient.
	type usageAgg struct {
		e                models.EfficiencyMetrics
		cpuCost, ramCost float64
	}
	usage := make(map[string]*usageAgg)
	for _, set := range summary.Data.Sets {
		for labelVal, a := range set.Allocations {
			if isVirtualAllocation(labelVal) {
				continue
			}
			u, ok := usage[labelVal]
			if !ok {
				u = &usageAgg{}
				usage[labelVal] = u
			}
			u.e.CPUCoresRequested += a.CPUCoreRequestAverage
			u.e.CPUCoresUsed += a.CPUCoreUsageAverage
			u.e.RAMBytesRequested += a.RAMByteRequestAverage
			u.e.RAMBytesUsed += a.RAMByteUsageAverage
			u.cpuCost += a.CPUCost
			u.ramCost += a.RAMCost
		}
	}

	// Each label value is projected from its own period, the way a namespace is: the response covers
	// workloads of every age, so the period it shares is the oldest one's, and a value whose pods
	// started an hour ago would show an hour of cost spread over a day. A value with less than
	// MinMeasuredPeriod of data is left out until it has more, as a namespace is.
	type costAgg struct {
		cpu, ram, gpu, pv, network, lb, total float64
		start, end                            time.Time
	}
	costs := make(map[string]*costAgg)

	for _, allocations := range alloc.Data {
		for labelVal, a := range allocations {
			// Skip OpenCost virtual entries (__unallocated__, __idle__, etc.)
			if isVirtualAllocation(labelVal) {
				continue
			}
			if _, ok := costs[labelVal]; !ok {
				costs[labelVal] = &costAgg{}
			}
			c := costs[labelVal]
			c.cpu += a.CPUCost
			c.ram += a.RAMCost
			c.gpu += a.GPUCost
			c.pv += a.PVCost
			c.network += a.NetworkCost
			c.lb += a.LoadBalancerCost
			c.total += a.componentCost()
			if !a.Start.IsZero() && (c.start.IsZero() || a.Start.Before(c.start)) {
				c.start = a.Start
			}
			if a.End.After(c.end) {
				c.end = a.End
			}
		}
	}

	entries := make([]models.ChargebackEntry, 0, len(costs))
	for labelVal, cost := range costs {
		if !cost.start.IsZero() && cost.end.After(cost.start) && cost.end.Sub(cost.start) < MinMeasuredPeriod {
			continue
		}
		monthlyMultiplier := monthlyFactor(cost.start, cost.end)
		var efficiency float64
		var noRequests bool
		if u, ok := usage[labelVal]; ok {
			setRequestEfficiency(&u.e, u.cpuCost, u.ramCost)
			efficiency = u.e.TotalEfficiency
			noRequests = u.e.CPUCoresRequested <= 0 && u.e.RAMBytesRequested <= 0
		}
		entries = append(entries, models.ChargebackEntry{
			LabelValue:  labelVal,
			CPUCost:     cost.cpu * monthlyMultiplier,
			RAMCost:     cost.ram * monthlyMultiplier,
			GPUCost:     cost.gpu * monthlyMultiplier,
			StorageCost: cost.pv * monthlyMultiplier,
			NetworkCost: cost.network * monthlyMultiplier,
			LBCost:      cost.lb * monthlyMultiplier,
			TotalCost:   cost.total * monthlyMultiplier,
			Efficiency:  efficiency,
			NoRequests:  noRequests,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].TotalCost > entries[j].TotalCost
	})

	return entries
}

// AutocompleteResponse mirrors the wire format of OpenCost's /allocation/autocomplete.
type AutocompleteResponse struct {
	Code int `json:"code"`
	Data struct {
		Data []string `json:"data"`
	} `json:"data"`
}

// GetLabelKeys returns the label keys carried by pods over the last 24 hours (the chargeback
// window), in a namespace or, when namespace is empty, across the cluster. OpenCost reports them
// sanitized (app.kubernetes.io/name becomes app_kubernetes_io_name), a form aggregate=label:
// accepts as it is. Labels nobody would charge back by are left out; see isBookkeepingLabel.
// Needs OpenCost 1.120.3 or later.
func (c *Client) GetLabelKeys(ctx context.Context, namespace string) ([]string, error) {
	params := url.Values{}
	params.Add("window", windowParam(last24h()))
	params.Add("field", "label")
	if namespace != "" {
		params.Add("filter", fmt.Sprintf("namespace:%q", namespace))
	}

	c.prepare(ctx, params, false)
	requestURL := fmt.Sprintf("%s/allocation/autocomplete?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost label keys GET %s", requestURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create autocomplete request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencost autocomplete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, string(body))
	}

	var result AutocompleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode autocomplete response: %w", err)
	}

	return chargebackLabelKeys(result.Data.Data), nil
}

// chargebackLabelKeys drops bookkeeping labels from keys and returns the rest sorted, without
// duplicates. It never returns nil, so an empty result encodes as [].
func chargebackLabelKeys(keys []string) []string {
	kept := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if key == "" || seen[key] || isBookkeepingLabel(key) {
			continue
		}
		seen[key] = true
		kept = append(kept, key)
	}
	sort.Strings(kept)
	return kept
}

// bookkeepingLabelPrefixes are the sanitized prefixes of labels Kubernetes, cloud providers and
// controllers set for their own use. OpenCost merges node and namespace labels into a pod's
// labels, so these show up next to the ones teams set. app_kubernetes_io_ is not among them:
// app.kubernetes.io/name and its siblings are the usual way to name an app.
var bookkeepingLabelPrefixes = []string{
	"kubernetes_io_",
	"beta_kubernetes_io_",
	"node_kubernetes_io_",
	"node_role_kubernetes_io_",
	"topology_kubernetes_io_",
	"failure_domain_beta_kubernetes_io_",
	"apps_kubernetes_io_",
	"batch_kubernetes_io_",
	"statefulset_kubernetes_io_",
	"eks_amazonaws_com_",
	"cloud_google_com_",
	"kubernetes_azure_com_",
	"karpenter_",
	"svccontroller_k3s_cattle_io_",
}

// bookkeepingLabels are whole sanitized keys set by controllers or OpenCost itself.
var bookkeepingLabels = map[string]bool{
	"controller_revision_hash": true,
	"controller_uid":           true,
	"job_name":                 true,
	"pod_template_generation":  true,
	"pod_template_hash":        true,
	"providerID":               true,
}

func isBookkeepingLabel(key string) bool {
	if bookkeepingLabels[key] {
		return true
	}
	for _, prefix := range bookkeepingLabelPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func parseOrphanedPods(resp AllocationResponse) []models.OrphanedPod {
	const minCostThreshold = 0.001
	monthlyMultiplier := monthlyFactor(measuredPeriod(resp))

	seen := make(map[string]bool)
	var pods []models.OrphanedPod

	for _, allocations := range resp.Data {
		for podKey, alloc := range allocations {
			// A pod is orphaned only when it uses no CPU or RAM
			if alloc.CPUCoreUsageAverage > 0 || alloc.RAMByteUsageAverage > 0 {
				continue
			}
			// Skip if no meaningful cost remains (pod fully wound down)
			if alloc.componentCost()*monthlyMultiplier < minCostThreshold {
				continue
			}
			// Skip OpenCost virtual entries like __idle__ or __unallocated__
			if isVirtualAllocation(podKey) {
				continue
			}

			name := podKey
			if alloc.Properties.Pod != "" {
				name = alloc.Properties.Pod
			}
			if seen[name] {
				continue
			}
			seen[name] = true

			pods = append(pods, models.OrphanedPod{
				Name:           name,
				Namespace:      alloc.Properties.Namespace,
				Controller:     alloc.Properties.Controller,
				ControllerKind: alloc.Properties.ControllerKind,
				CPUCost:        alloc.CPUCost * monthlyMultiplier,
				RAMCost:        alloc.RAMCost * monthlyMultiplier,
				StorageCost:    alloc.PVCost * monthlyMultiplier,
				TotalCost:      alloc.componentCost() * monthlyMultiplier,
				Labels:         alloc.Properties.Labels,
			})
		}
	}

	sort.Slice(pods, func(i, j int) bool {
		return pods[i].TotalCost > pods[j].TotalCost
	})

	return pods
}

// dayWindow renders the UTC days from through to (both included; only their dates count) as
// OpenCost's window parameter. A range reaching past now ends at now, so the current day comes back
// partly measured.
func dayWindow(from, to, now time.Time) string {
	day := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	start := day(from.UTC())
	end := day(to.UTC()).AddDate(0, 0, 1)
	if limit := now.UTC().Truncate(time.Minute); end.After(limit) {
		end = limit
	}
	if !end.After(start) {
		end = start.Add(time.Minute)
	}
	return start.Format(time.RFC3339) + "," + end.Format(time.RFC3339)
}

// GetDailyTimeSeries returns a namespace's cost for each UTC day from through to. With resources, it covers only the pods
// they own: the series is then queried per controller, which OpenCost reports with each controller's
// kind and name. Pods without a controller fall under OpenCost's unallocated entry there, so an
// application's bare pods are left out of its series.
func (c *Client) GetDailyTimeSeries(
	ctx context.Context, namespace string, from, to time.Time, resources models.ManagedResources,
) ([]models.DailyCost, error) {
	aggregate := "namespace"
	if resources != nil {
		aggregate = "controller"
	}

	params := url.Values{}
	params.Add("window", dayWindow(from, to, time.Now()))
	params.Add("step", "1d")
	params.Add("aggregate", aggregate)
	params.Add("filter", fmt.Sprintf("namespace:%q", namespace))

	c.prepare(ctx, params, true)
	c.applySharing(params)
	requestURL := fmt.Sprintf("%s/allocation?%s", c.baseURL, params.Encode())
	c.logf("DEBUG opencost GET %s", requestURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencost daily request failed: %w", err)
	}
	defer resp.Body.Close()

	c.logf("DEBUG opencost daily timeseries response: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, string(body))
	}

	var result AllocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode opencost daily response: %w", err)
	}
	c.countShared(&result)
	c.shareNamespacesInCluster(ctx, &result, namespace, params.Get("window"), true)

	if resources != nil {
		result = ownedControllers(result, namespace, resources)
	}
	return parseDailyTimeSeries(result), nil
}

// ownedControllers keeps the controller allocations the resources own. A day on which none of them
// ran is left with no entries, and the series skips it.
func ownedControllers(resp AllocationResponse, namespace string, resources models.ManagedResources) AllocationResponse {
	owned := AllocationResponse{Code: resp.Code, Data: make([]map[string]AllocationData, 0, len(resp.Data))}
	for _, allocations := range resp.Data {
		kept := make(map[string]AllocationData)
		for key, alloc := range allocations {
			if isVirtualAllocation(key) {
				continue
			}
			ns := alloc.Properties.Namespace
			if ns == "" {
				ns = namespace
			}
			if resources.OwnsPod(ns, alloc.Properties.ControllerKind, alloc.Properties.Controller, "") {
				kept[key] = alloc
			}
		}
		owned.Data = append(owned.Data, kept)
	}
	return owned
}

func parseDailyTimeSeries(resp AllocationResponse) []models.DailyCost {
	dailyCosts := make([]models.DailyCost, 0, len(resp.Data))

	for _, allocations := range resp.Data {
		var date string
		var totalCPU, totalMem, totalNetwork, totalStorage, totalCost float64
		for _, alloc := range allocations {
			if date == "" && !alloc.Start.IsZero() {
				date = alloc.Start.UTC().Format("2006-01-02")
			}
			totalCPU += alloc.CPUCost
			totalMem += alloc.RAMCost
			totalNetwork += alloc.NetworkCost
			totalStorage += alloc.PVCost
			totalCost += alloc.componentCost()
		}
		if date == "" {
			continue
		}

		dailyCosts = append(dailyCosts, models.DailyCost{
			Date:        date,
			TotalCost:   totalCost,
			CPUCost:     totalCPU,
			MemoryCost:  totalMem,
			NetworkCost: totalNetwork,
			StorageCost: totalStorage,
		})
	}

	sort.Slice(dailyCosts, func(i, j int) bool {
		return dailyCosts[i].Date < dailyCosts[j].Date
	})

	return dailyCosts
}

func parsePodBreakdown(resp AllocationResponse) []models.PodCost {
	podMap := make(map[string]*models.PodCost)

	for _, allocations := range resp.Data {
		for podKey, alloc := range allocations {
			if isVirtualAllocation(podKey) {
				continue
			}
			name := podKey
			if alloc.Properties.Pod != "" {
				name = alloc.Properties.Pod
			}

			if existing, ok := podMap[name]; ok {
				existing.CPUCost += alloc.CPUCost
				existing.RAMCost += alloc.RAMCost
				existing.SharedCost += alloc.countedShared()
				existing.TotalCost += alloc.componentCost()
			} else {
				podMap[name] = &models.PodCost{
					Name:           name,
					Namespace:      alloc.Properties.Namespace,
					Controller:     alloc.Properties.Controller,
					ControllerKind: alloc.Properties.ControllerKind,
					CPUCost:        alloc.CPUCost,
					RAMCost:        alloc.RAMCost,
					SharedCost:     alloc.countedShared(),
					TotalCost:      alloc.componentCost(),
				}
			}
		}
	}

	// Every pod OpenCost reports is listed, however little it cost: a short job costs cents, and
	// leaving it out hides a pod the application launched.
	monthlyMultiplier := monthlyFactor(measuredPeriod(resp))
	pods := make([]models.PodCost, 0, len(podMap))
	for _, p := range podMap {
		pods = append(pods, models.PodCost{
			Name:           p.Name,
			Namespace:      p.Namespace,
			Controller:     p.Controller,
			ControllerKind: p.ControllerKind,
			CPUCost:        p.CPUCost * monthlyMultiplier,
			RAMCost:        p.RAMCost * monthlyMultiplier,
			SharedCost:     p.SharedCost * monthlyMultiplier,
			TotalCost:      p.TotalCost * monthlyMultiplier,
		})
	}
	return pods
}

// parseSummaryResponse reads a namespace's requests, usage and efficiency. It has no idle cost: the
// summary's idle fields are always zero (see podEfficiency), so the namespace's idle cost is measured
// from its pods (PodAllocations.Efficiency).
func parseSummaryResponse(resp SummaryAllocationResponse) *models.EfficiencyMetrics {
	var agg SummaryAllocationData
	var count int

	for _, set := range resp.Data.Sets {
		for _, alloc := range set.Allocations {
			if !alloc.Start.IsZero() && (agg.Start.IsZero() || alloc.Start.Before(agg.Start)) {
				agg.Start = alloc.Start
			}
			if alloc.End.After(agg.End) {
				agg.End = alloc.End
			}
			agg.CPUCoreRequestAverage += alloc.CPUCoreRequestAverage
			agg.CPUCoreUsageAverage += alloc.CPUCoreUsageAverage
			agg.CPUCoreLimitAverage += alloc.CPUCoreLimitAverage
			agg.RAMByteRequestAverage += alloc.RAMByteRequestAverage
			agg.RAMByteUsageAverage += alloc.RAMByteUsageAverage
			agg.RAMByteLimitAverage += alloc.RAMByteLimitAverage
			agg.CPUCost += alloc.CPUCost
			agg.RAMCost += alloc.RAMCost
			count++
		}
	}

	if count == 0 {
		return nil
	}

	e := &models.EfficiencyMetrics{
		CPUCoresRequested: agg.CPUCoreRequestAverage,
		CPUCoresUsed:      agg.CPUCoreUsageAverage,
		RAMBytesRequested: agg.RAMByteRequestAverage,
		RAMBytesUsed:      agg.RAMByteUsageAverage,
		CPUCoresLimit:     agg.CPUCoreLimitAverage,
		RAMBytesLimit:     agg.RAMByteLimitAverage,
	}
	setRequestEfficiency(e, agg.CPUCost, agg.RAMCost)
	return e
}

func parseOpenCostResponse(resp AllocationResponse, startTime, endTime time.Time) *models.OpenCostMetrics {
	metrics := &models.OpenCostMetrics{
		// In UTC, like the window the request sent (see windowParam).
		Window: []string{
			startTime.UTC().Format(time.RFC3339),
			endTime.UTC().Format(time.RFC3339),
		},
		ResourceDetails: make(map[string]any),
	}

	var totalCPU, totalMem, totalStorage, totalGPU, totalNetwork, totalLB, totalCarbon, totalShared float64
	var totalNetXZ, totalNetXR, totalNetInternet, totalNetNat, totalSharedCounted float64
	var peakCPU, peakRAM float64
	sharedBreakdown := make(map[string]float64)

	for i, allocations := range resp.Data {
		key := fmt.Sprintf("window-%d", i)
		for _, alloc := range allocations {
			if !alloc.Start.IsZero() {
				key = alloc.Start.UTC().Format(time.RFC3339)
			}
			break
		}
		metrics.ResourceDetails[key] = allocations
		for allocKey, alloc := range allocations {
			// Idle capacity is not the namespace's cost, and Kubecost lists __idle__ even
			// when includeIdle is false.
			if isVirtualAllocation(allocKey) {
				continue
			}
			totalCPU += alloc.CPUCost
			totalMem += alloc.RAMCost
			totalStorage += alloc.PVCost
			totalGPU += alloc.GPUCost
			totalNetwork += alloc.NetworkCost
			totalLB += alloc.LoadBalancerCost
			totalCarbon += alloc.CarbonCost
			totalShared += alloc.SharedCost
			totalSharedCounted += alloc.countedShared()
			totalNetXZ += alloc.NetworkCrossZoneCost
			totalNetXR += alloc.NetworkCrossRegionCost
			totalNetInternet += alloc.NetworkInternetCost
			totalNetNat += alloc.NetworkNatGatewayEgressCost + alloc.NetworkNatGatewayIngressCost
			if alloc.RawAllocationOnly != nil {
				peakCPU += alloc.RawAllocationOnly.CPUCoreUsageMax
				peakRAM += alloc.RawAllocationOnly.RAMByteUsageMax
			}
			for name, item := range alloc.SharedCostBreakdown {
				sharedBreakdown[name] += item.TotalCost
			}
		}
	}

	// Project onto a 30-day month from the period OpenCost actually measured.
	measuredStart, measuredEnd := measuredPeriod(resp)
	monthlyMultiplier := monthlyFactor(measuredStart, measuredEnd)
	if !measuredStart.IsZero() && measuredEnd.After(measuredStart) {
		metrics.MeasuredHours = measuredEnd.Sub(measuredStart).Hours()
	}

	var scaledSharedBreakdown map[string]float64
	if len(sharedBreakdown) > 0 {
		scaledSharedBreakdown = make(map[string]float64, len(sharedBreakdown))
		for k, v := range sharedBreakdown {
			scaledSharedBreakdown[k] = v * monthlyMultiplier
		}
	}

	metrics.AllocationTotals = models.AllocationTotal{
		CPUCost:                totalCPU * monthlyMultiplier,
		MemoryCost:             totalMem * monthlyMultiplier,
		StorageCost:            totalStorage * monthlyMultiplier,
		GPUCost:                totalGPU * monthlyMultiplier,
		NetworkCost:            totalNetwork * monthlyMultiplier,
		NetworkCrossZoneCost:   totalNetXZ * monthlyMultiplier,
		NetworkCrossRegionCost: totalNetXR * monthlyMultiplier,
		NetworkInternetCost:    totalNetInternet * monthlyMultiplier,
		NetworkNatGatewayCost:  totalNetNat * monthlyMultiplier,
		LoadBalancerCost:       totalLB * monthlyMultiplier,
		TotalCost:              (totalCPU + totalMem + totalStorage + totalGPU + totalNetwork + totalLB + totalSharedCounted) * monthlyMultiplier,
		CarbonCost:             totalCarbon * monthlyMultiplier,
		SharedCost:             totalShared * monthlyMultiplier,
		SharedCostIncluded:     totalSharedCounted > 0,
		SharedCostBreakdown:    scaledSharedBreakdown,
	}

	metrics.PeakCPUCores = peakCPU
	metrics.PeakRAMBytes = peakRAM

	return metrics
}

// GetDataQuality calls OpenCost's /pricingSourceStatus endpoint and returns a
// DataQuality struct. Errors and unexpected responses are surfaced as
// PricingSource="unknown" rather than failing the caller — data quality is
// advisory, not load-bearing.
func (c *Client) GetDataQuality(ctx context.Context) (*models.DataQuality, error) {
	if c.kubecost {
		return c.kubecostDataQuality(ctx), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/pricingSourceStatus", http.NoBody)
	if err != nil {
		return &models.DataQuality{PricingSource: "unknown", Message: "failed to build request"}, nil
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return &models.DataQuality{PricingSource: "unknown", Message: "OpenCost unreachable"}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.logf("DEBUG opencost pricingSourceStatus: %d %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return &models.DataQuality{PricingSource: "unknown", Message: fmt.Sprintf("status %d", resp.StatusCode)}, nil
	}

	// The data field is a plain string message in most OpenCost versions.
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return &models.DataQuality{PricingSource: "unknown", Message: "unreadable response"}, nil
	}

	var msg string
	_ = json.Unmarshal(envelope.Data, &msg)

	return &models.DataQuality{
		PricingSource: classifyPricingSource(msg),
		Message:       msg,
	}, nil
}

// classifyPricingSource maps a raw OpenCost status message to one of three tiers:
// "real" (cloud-billing prices), "estimated" (on-demand defaults), "unknown".
func classifyPricingSource(msg string) string {
	if msg == "" {
		return "unknown"
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "default") || strings.Contains(lower, "list price") {
		return "estimated"
	}
	return "real"
}

// kubecostDataQuality reads the pricing source from Kubecost's cloud integrations
// (/cloudCost/status): a cloud billing integration (for example the AWS CUR) that is active and valid
// means prices come from the bill ("real"); integrations that exist but don't work, or none, mean
// on-demand list prices ("estimated" or, when Kubecost can't be asked, "unknown"). Kubecost has no
// pricingSourceStatus like OpenCost's.
func (c *Client) kubecostDataQuality(ctx context.Context) *models.DataQuality {
	unknown := func(msg string) *models.DataQuality {
		return &models.DataQuality{PricingSource: "unknown", Message: msg}
	}
	base := c.cloudCostURL
	if base == "" {
		base = c.baseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/cloudCost/status", http.NoBody)
	if err != nil {
		return unknown("failed to build request")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return unknown("Kubecost unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return unknown(fmt.Sprintf("Kubecost cloud integration status unavailable (status %d); set KUBECOST_CLOUD_COST_URL to the service that serves /cloudCost/status", resp.StatusCode))
	}
	var status struct {
		Data []struct {
			Provider string `json:"provider"`
			Active   bool   `json:"active"`
			Valid    bool   `json:"valid"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return unknown("unreadable response")
	}
	if len(status.Data) == 0 {
		return &models.DataQuality{PricingSource: "estimated", Message: "No cloud billing integration is configured in Kubecost; on-demand list prices are used"}
	}
	for _, integration := range status.Data {
		if integration.Active && integration.Valid {
			return &models.DataQuality{PricingSource: "real", Message: integration.Provider + " billing integration is active in Kubecost"}
		}
	}
	return &models.DataQuality{PricingSource: "estimated", Message: "No Kubecost cloud billing integration is active and valid; on-demand list prices are used"}
}

// AssetsResponse mirrors the top-level shape of GET /assets: one set of assets for the
// window, keyed by their properties joined with "/". OpenCost ignores `aggregate` here,
// so an asset is found by its type, not by its key.
type AssetsResponse struct {
	Code int                  `json:"code"`
	Data map[string]AssetData `json:"data"`
}

// UnmarshalJSON reads data as one set of assets, or as a list of sets, which Kubecost answers with
// unless the request accumulates. The sets of a list are merged; their keys don't repeat an asset
// across days that were accumulated, and a repeated key keeps the last.
func (r *AssetsResponse) UnmarshalJSON(b []byte) error {
	var raw struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	r.Code = raw.Code
	r.Data = map[string]AssetData{}
	data := strings.TrimSpace(string(raw.Data))
	switch {
	case data == "" || data == "null":
		return nil
	case strings.HasPrefix(data, "["):
		var sets []map[string]AssetData
		if err := json.Unmarshal(raw.Data, &sets); err != nil {
			return err
		}
		for _, set := range sets {
			maps.Copy(r.Data, set)
		}
		return nil
	default:
		return json.Unmarshal(raw.Data, &r.Data)
	}
}

type AssetProperties struct {
	Provider string `json:"provider"`
	Cluster  string `json:"cluster"`
	Name     string `json:"name"`
}

type AssetData struct {
	Start      time.Time       `json:"start"`
	End        time.Time       `json:"end"`
	Type       string          `json:"type"`
	Properties AssetProperties `json:"properties"`
	TotalCost  float64         `json:"totalCost"`
	// ClaimName, ClaimNamespace and VolumeName are a disk's: the PersistentVolumeClaim bound to it
	// (Kubecost leaves its namespace empty) and the PV's name.
	ClaimName      string `json:"claimName"`
	ClaimNamespace string `json:"claimNamespace"`
	VolumeName     string `json:"volumeName"`
}

// claimRef is the PersistentVolumeClaim of a volume; namespace is empty when the provider does not say.
type claimRef struct{ namespace, name string }

// volumeClaims maps each PV of a cluster to its claim, for the claims no other volume of the cluster has.
type volumeClaims struct {
	byVolume map[string]claimRef
	readAt   time.Time
}

// volumeClaimsTTL is how long a cluster's claims are kept: they change as volumes are created, and
// every namespace measured reads them.
const volumeClaimsTTL = 10 * time.Minute

// volumeClaims reads the claim of every volume in the cluster from the disks the provider lists, once
// per volumeClaimsTTL.
func (c *Client) volumeClaims(ctx context.Context) (volumeClaims, error) {
	cluster := c.clusterOf(ctx)
	c.volumeMu.Lock()
	defer c.volumeMu.Unlock()
	if cached, ok := c.volumeCache[cluster]; ok && time.Since(cached.readAt) < volumeClaimsTTL {
		return cached, nil
	}

	params := url.Values{}
	params.Set("window", "24h")
	if c.kubecost {
		params.Set("accumulate", "true")
		if cluster != "" {
			params.Set("filter", fmt.Sprintf("cluster:%q", cluster))
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/assets?"+params.Encode(), http.NoBody)
	if err != nil {
		return volumeClaims{}, fmt.Errorf("creating assets request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return volumeClaims{}, fmt.Errorf("executing assets request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return volumeClaims{}, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, body)
	}
	var result AssetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return volumeClaims{}, fmt.Errorf("decoding assets response: %w", err)
	}

	claims := volumeClaimsOf(result)
	if c.volumeCache == nil {
		c.volumeCache = map[string]volumeClaims{}
	}
	c.volumeCache[cluster] = claims
	return claims, nil
}

// volumeClaimsOf maps PV names to their claims, leaving out a claim more than one disk has: without the
// namespace of each, it could be any of them.
func volumeClaimsOf(assets AssetsResponse) volumeClaims {
	uses := map[claimRef]int{}
	names := map[string]int{}
	for _, asset := range assets.Data {
		if strings.EqualFold(asset.Type, "Disk") && asset.ClaimName != "" {
			uses[claimRef{asset.ClaimNamespace, asset.ClaimName}]++
			names[asset.ClaimName]++
		}
	}
	claims := volumeClaims{byVolume: map[string]claimRef{}, readAt: time.Now()}
	for _, asset := range assets.Data {
		if !strings.EqualFold(asset.Type, "Disk") || asset.ClaimName == "" {
			continue
		}
		ref := claimRef{asset.ClaimNamespace, asset.ClaimName}
		pv := asset.VolumeName
		if pv == "" {
			pv = asset.Properties.Name
		}
		if pv != "" && uses[ref] == 1 && (ref.namespace != "" || names[ref.name] == 1) {
			claims.byVolume[pv] = ref
		}
	}
	return claims
}

// ListClusters returns the names of the clusters a Kubecost aggregator has cost for over the last day,
// sorted. It is not scoped to a cluster: the point is to see them all. OpenCost knows only its own.
func (c *Client) ListClusters(ctx context.Context) ([]string, error) {
	if !c.kubecost {
		return nil, nil
	}
	params := url.Values{}
	params.Set("window", "24h")
	params.Set("aggregate", "cluster")
	params.Set("accumulate", "true")
	params.Set("includeIdle", "false")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/allocation?"+params.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating clusters request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing clusters request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, body)
	}
	var result AllocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding clusters response: %w", err)
	}
	seen := map[string]bool{}
	for _, set := range result.Data {
		for key := range set {
			if key != "" && !isVirtualAllocation(key) {
				seen[key] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(seen)), nil
}

// GetClusterOverhead fetches the ClusterManagement asset (EKS/GKE/AKS control-plane
// fee) from OpenCost /assets and returns the monthly cost. Returns an empty
// ClusterOverhead (cost = 0) when no ClusterManagement entry exists, which is
// normal for self-managed clusters.
func (c *Client) GetClusterOverhead(ctx context.Context) (*models.ClusterOverhead, error) {
	u, err := url.Parse(c.baseURL + "/assets")
	if err != nil {
		return nil, fmt.Errorf("parsing assets URL: %w", err)
	}
	q := u.Query()
	q.Set("window", "24h")
	if c.kubecost {
		q.Set("accumulate", "true")
		if id := c.clusterOf(ctx); id != "" {
			q.Set("filter", fmt.Sprintf("cluster:%q", id))
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating assets request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing assets request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencost returned status %d: %s", resp.StatusCode, body)
	}

	var result AssetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding assets response: %w", err)
	}

	// OpenCost reports one ClusterManagement asset per cluster and provisioner, so the fees
	// add up; keys are sorted so the provider and cluster shown are stable between requests.
	overhead := &models.ClusterOverhead{}
	found := false
	for _, key := range slices.Sorted(maps.Keys(result.Data)) {
		asset := result.Data[key]
		if asset.Type != "ClusterManagement" {
			continue
		}
		if !found {
			overhead.Provider = asset.Properties.Provider
			overhead.Cluster = asset.Properties.Cluster
			found = true
		}
		overhead.ControlPlaneCost += asset.TotalCost * monthlyFactor(asset.Start, asset.End)
	}

	return overhead, nil
}
