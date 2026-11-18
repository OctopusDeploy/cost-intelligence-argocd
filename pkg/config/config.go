package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/models"
)

type Config struct {
	Server   ServerConfig   `envconfig:"SERVER"`
	Database DatabaseConfig `envconfig:"DB"`
	OpenCost OpenCostConfig `envconfig:"OPENCOST"`
	// CostProvider is the service OPENCOST_BASE_URL points at: ProviderOpenCost (the default) or
	// ProviderKubecost, whose API this service reads the same way with a few differences.
	CostProvider string `envconfig:"COST_PROVIDER" default:"opencost"`
	// KubecostClusterID is the cluster this service runs in, as Kubecost names it. Required with
	// ProviderKubecost: a Kubecost aggregator answers for every cluster, and a namespace that exists
	// in several would otherwise be summed across them.
	KubecostClusterID string `envconfig:"KUBECOST_CLUSTER_ID"`
	// KubecostClusterMapFile is a JSON file mapping the other clusters Argo CD deploys to onto their
	// names in Kubecost, for example a mounted ConfigMap. Without it only the local cluster is measured.
	KubecostClusterMapFile string `envconfig:"KUBECOST_CLUSTER_MAP_FILE"`
	// KubecostDiscoverClusters has an Argo CD cluster registered under the name Kubecost itself uses
	// for it measured without an entry in the map. A name that differs needs an entry.
	KubecostDiscoverClusters bool `envconfig:"KUBECOST_DISCOVER_CLUSTERS" default:"true"`
	// KubecostAPIKey is sent with every request to a protected Kubecost aggregator, in the header
	// KubecostAPIKeyHeader. It is a secret: the chart reads it from a Secret.
	KubecostAPIKey string `envconfig:"KUBECOST_API_KEY"`
	// KubecostAPIKeyHeader is the header carrying the key. For Authorization the value is sent as
	// "Bearer <key>"; for any other header, as the key itself.
	KubecostAPIKeyHeader string `envconfig:"KUBECOST_API_KEY_HEADER" default:"Authorization"`
	// KubecostEKSOptimized is for the EKS-optimized Kubecost, which keeps only EKSOptimizedRetentionDays
	// days: history is read in windows that fit, and the UI aggregates by week, never by month.
	KubecostEKSOptimized bool `envconfig:"KUBECOST_EKS_OPTIMIZED"`
	// KubecostCloudCostURL is the base URL of the Kubecost service that answers /cloudCost/status, which
	// says whether a cloud billing integration (for example the AWS CUR) prices the cost. The aggregator
	// doesn't serve it: use the cloud-cost service, for example http://kubecost-cloud-cost.kubecost:9005.
	// Empty uses OPENCOST_BASE_URL.
	KubecostCloudCostURL string `envconfig:"KUBECOST_CLOUD_COST_URL"`
	// KubecostShareTenancyCosts spreads Kubecost's tenancy costs (cluster-level costs no namespace
	// owns) over the allocations, and counts that share in each cost. Off by default.
	KubecostShareTenancyCosts bool `envconfig:"KUBECOST_SHARE_TENANCY_COSTS"`
	// KubecostShareNamespaces are namespaces (for example kube-system) whose cost Kubecost spreads over
	// the other allocations, and counts in each cost. Such a namespace is not measured itself.
	KubecostShareNamespaces []string `envconfig:"KUBECOST_SHARE_NAMESPACES"`
	// WebhookSecret is the bearer token Argo CD Notifications must send to POST /on-sync.
	WebhookSecret string `envconfig:"WEBHOOK_SECRET"`

	// ClusterScopeApp ("<namespace>:<name>") is the Argo CD Application whose viewers may see
	// cluster-wide cost data. Empty finds one: the Application that installed the service, else
	// ClusterAccessApp. ClusterScopeOff turns cluster-wide endpoints off.
	ClusterScopeApp string `envconfig:"CLUSTER_SCOPE_APP"`

	// ClusterAccessApp ("<namespace>:<name>") is the access Application the Helm chart creates when
	// Helm installs the service, used when no Argo CD Application installed it.
	ClusterAccessApp string `envconfig:"CLUSTER_ACCESS_APP"`

	// ArgoCDNamespace is where Argo CD runs: an Application named without a namespace in Argo CD's
	// tracking annotation is there.
	ArgoCDNamespace string `envconfig:"ARGOCD_NAMESPACE" default:"argocd"`

	// PodNamespace and DeploymentName locate the service's own Deployment, whose tracking annotation
	// names the Application that installed it.
	PodNamespace   string `envconfig:"POD_NAMESPACE"`
	DeploymentName string `envconfig:"DEPLOYMENT_NAME"`

	// DailyHistoryDays is how many days of each application's daily cost are kept, so the Trends tab
	// can reach further back than OpenCost's Prometheus. 0 turns recording off.
	DailyHistoryDays int `envconfig:"DAILY_HISTORY_DAYS" default:"400"`

	// BackfillConcurrency is how many namespaces the startup backfill measures with OpenCost at once.
	// The backfill measures each Argo CD Application with no cost record yet when the service starts.
	// 0 turns it off.
	BackfillConcurrency int `envconfig:"BACKFILL_CONCURRENCY" default:"4"`

	// PruneRemoteCost deletes the cost records and daily history stored for Argo CD Applications that
	// deploy to another cluster, at startup and on each daily history run. It can't be undone.
	PruneRemoteCost bool `envconfig:"PRUNE_REMOTE_COST" default:"false"`

	// OwnerLabels extends the pod labels that name the custom resource owning a pod, for operators that
	// generate their own StatefulSets or Deployments: comma-separated "<label>=<Kind>|<Kind>", or just
	// "<label>" for any custom resource. See models.ConfigureOwnership.
	OwnerLabels string `envconfig:"OWNER_LABELS"`

	// UnmanagedGroupLabels are the pod labels whose values group the pods no Argo CD application manages
	// in the namespace view, comma-separated, for example the team and GitLab project of CI job pods.
	// Presentation only. Empty turns grouping off.
	UnmanagedGroupLabels string `envconfig:"UNMANAGED_GROUP_LABELS"`

	// NonOwnerKinds extends the kinds Argo CD lists that never own pods: comma-separated.
	NonOwnerKinds string `envconfig:"NON_OWNER_KINDS"`

	// UnownedPodLabels attributes the pods that have no controller (bare pods created by Airflow's
	// Kubernetes executor, for example) to the only application of their namespace when they carry one of
	// these labels: comma-separated "<label>=<value>" or "<label>". See models.ConfigureUnownedPodLabels.
	UnownedPodLabels string `envconfig:"UNOWNED_POD_LABELS"`

	// VPAEnabled reads VerticalPodAutoscaler recommendations (from Goldilocks or any VPA) for the
	// rightsizing advice. It needs the service to run in the cluster with permission to list them.
	VPAEnabled bool `envconfig:"VPA_ENABLED" default:"true"`

	// Mode is what the process runs: ModeAPI, the API and webhook, or ModeExporter, the singleton that
	// exports each Application's cost to Prometheus.
	Mode string `envconfig:"MODE" default:"api"`

	// MetricsPort is the port /metrics is served on, apart from the API port so a NetworkPolicy can
	// admit Prometheus to it alone. 0 turns metrics off in the API.
	MetricsPort int `envconfig:"METRICS_PORT" default:"9102"`

	// ExporterInterval is how often the exporter measures every Application's cost.
	ExporterInterval time.Duration `envconfig:"EXPORTER_INTERVAL" default:"5m"`

	// LogLevel is LogLevelInfo or LogLevelDebug. Debug also logs each OpenCost request URL and
	// response body, which name every namespace and cost in the cluster.
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
}

// Log levels.
const (
	LogLevelInfo  = "info"
	LogLevelDebug = "debug"
)

// Debug reports whether LOG_LEVEL asks for debug logging.
func (c *Config) Debug() bool {
	return c.LogLevel == LogLevelDebug
}

// ClusterScopeOff, as CLUSTER_SCOPE_APP, turns cluster-wide endpoints off.
const ClusterScopeOff = "off"

// Modes the service runs in.
const (
	ModeAPI      = "api"
	ModeExporter = "exporter"
)

type ServerConfig struct {
	Port         int           `envconfig:"PORT" default:"8080"`
	ReadTimeout  time.Duration `envconfig:"READ_TIMEOUT" default:"10s"`
	WriteTimeout time.Duration `envconfig:"WRITE_TIMEOUT" default:"10s"`
	IdleTimeout  time.Duration `envconfig:"IDLE_TIMEOUT" default:"120s"`
}

type DatabaseConfig struct {
	Host     string `envconfig:"HOST" default:"localhost"`
	Port     int    `envconfig:"PORT" default:"5432"`
	User     string `envconfig:"USER" default:"cost_user"`
	Password string `envconfig:"PASSWORD" required:"true"`
	Database string `envconfig:"NAME" default:"cost_intelligence"`
	SSLMode  string `envconfig:"SSL_MODE" default:"disable"`
	MaxOpen  int    `envconfig:"MAX_OPEN_CONNS" default:"25"`
	MaxIdle  int    `envconfig:"MAX_IDLE_CONNS" default:"5"`
}

// The cost providers COST_PROVIDER accepts.
const (
	ProviderOpenCost = "opencost"
	ProviderKubecost = "kubecost"
)

type OpenCostConfig struct {
	BaseURL string        `envconfig:"BASE_URL" default:"http://opencost:9003"`
	Timeout time.Duration `envconfig:"TIMEOUT" default:"10s"`
}

func LoadConfig() (*Config, error) {
	var cfg Config

	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to process environment config: %w", err)
	}
	// "DEBUG" or "Debug " mean debug too, rather than stopping the service.
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))

	return &cfg, nil
}

func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=5",
		c.Host,
		c.Port,
		c.User,
		c.Password,
		c.Database,
		c.SSLMode,
	)
}

// minWebhookSecretLength rejects placeholder secrets. The Helm chart generates 32 characters.
const minWebhookSecretLength = 16

func (c *Config) Validate() error {
	if c.Database.Password == "" {
		return errors.New("database password is required")
	}
	if c.MetricsPort < 0 || c.MetricsPort > 65535 {
		return fmt.Errorf("METRICS_PORT %d is not a port; 0 turns metrics off", c.MetricsPort)
	}
	switch c.LogLevel {
	case LogLevelInfo, LogLevelDebug, "": // the zero value is info, the default
	default:
		return fmt.Errorf("LOG_LEVEL %q is not %q or %q", c.LogLevel, LogLevelInfo, LogLevelDebug)
	}
	switch c.Mode {
	case ModeExporter:
		if c.MetricsPort == 0 {
			return errors.New("METRICS_PORT is required in exporter mode: the exporter serves nothing else")
		}
		if c.ExporterInterval < time.Minute {
			return fmt.Errorf("EXPORTER_INTERVAL %s is too short: each refresh queries OpenCost for every namespace, so it must be at least 1m", c.ExporterInterval)
		}
		return c.validateCostBackend()
	case ModeAPI, "": // the zero value is the API, the default
	default:
		return fmt.Errorf("MODE %q is not %q or %q", c.Mode, ModeAPI, ModeExporter)
	}
	if c.MetricsPort != 0 && c.MetricsPort == c.Server.Port {
		return fmt.Errorf("METRICS_PORT and SERVER_PORT are both %d: metrics need a port of their own", c.MetricsPort)
	}
	if c.DailyHistoryDays < 0 {
		return errors.New("DAILY_HISTORY_DAYS must not be negative; 0 turns daily cost history off")
	}
	if c.BackfillConcurrency < 0 {
		return errors.New("BACKFILL_CONCURRENCY must not be negative; 0 turns the startup backfill off")
	}
	if c.ClusterScopeApp != "" && c.ClusterScopeApp != ClusterScopeOff {
		if _, err := models.ParseAppRef(c.ClusterScopeApp); err != nil {
			return fmt.Errorf("CLUSTER_SCOPE_APP %q: %w, or %q to turn cluster-wide cost data off", c.ClusterScopeApp, err, ClusterScopeOff)
		}
	}
	if c.ClusterAccessApp != "" {
		if _, err := models.ParseAppRef(c.ClusterAccessApp); err != nil {
			return fmt.Errorf("CLUSTER_ACCESS_APP %q: %w", c.ClusterAccessApp, err)
		}
	}
	if len(c.WebhookSecret) < minWebhookSecretLength {
		return fmt.Errorf("WEBHOOK_SECRET is required and must be at least %d characters: it authenticates Argo CD Notifications to POST /on-sync", minWebhookSecretLength)
	}

	return c.validateCostBackend()
}

// EKSOptimizedRetentionDays is how far back the EKS-optimized Kubecost can be queried.
const EKSOptimizedRetentionDays = 15

// RetentionDays is how many days back the cost backend can be asked for, 0 when it has no such limit.
func (c *Config) RetentionDays() int {
	if c.CostProvider == ProviderKubecost && c.KubecostEKSOptimized {
		return EKSOptimizedRetentionDays
	}
	return 0
}

// APIKeyHeader is the header name and value that carry the Kubecost API key, empty without a key.
func (c *Config) APIKeyHeader() (name, value string) {
	if c.CostProvider != ProviderKubecost || c.KubecostAPIKey == "" {
		return "", ""
	}
	name = c.KubecostAPIKeyHeader
	if name == "" {
		name = "Authorization"
	}
	if strings.EqualFold(name, "Authorization") {
		return name, "Bearer " + c.KubecostAPIKey
	}
	return name, c.KubecostAPIKey
}

// validateCostBackend checks the cost provider and the URL it is reached at.
func (c *Config) validateCostBackend() error {
	switch c.CostProvider {
	case ProviderOpenCost, "":
	case ProviderKubecost:
		if c.KubecostClusterID == "" {
			return errors.New("KUBECOST_CLUSTER_ID is required with COST_PROVIDER=kubecost: Kubecost answers for every cluster it knows, and the namespaces of the others would be counted as this cluster's")
		}
	default:
		return fmt.Errorf("COST_PROVIDER %q is not %q or %q", c.CostProvider, ProviderOpenCost, ProviderKubecost)
	}
	if c.CostProvider != ProviderKubecost && (c.KubecostEKSOptimized || c.KubecostAPIKey != "") {
		return errors.New("KUBECOST_EKS_OPTIMIZED and KUBECOST_API_KEY only apply with COST_PROVIDER=kubecost")
	}
	if c.KubecostAPIKey != "" && !validHeaderName(c.KubecostAPIKeyHeader) {
		return fmt.Errorf("KUBECOST_API_KEY_HEADER %q is not a valid header name", c.KubecostAPIKeyHeader)
	}
	return validateOpenCostURL(c.OpenCost.BaseURL)
}

// validateOpenCostURL ensures the configured OpenCost URL cannot be used for
// SSRF by restricting it to http/https schemes with a non-empty host.
func validateOpenCostURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("opencost base URL is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("opencost base URL is invalid: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("opencost base URL scheme must be http or https, got %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return errors.New("opencost base URL must include a hostname")
	}
	return nil
}

// validHeaderName reports whether name can be an HTTP header field name; empty means Authorization.
func validHeaderName(name string) bool {
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
