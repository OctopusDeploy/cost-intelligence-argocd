package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  DatabaseConfig
		want string
	}{
		{
			name: "standard values",
			cfg: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "postgres",
				Password: "secret",
				Database: "mydb",
				SSLMode:  "disable",
			},
			want: "host=localhost port=5432 user=postgres password=secret dbname=mydb sslmode=disable connect_timeout=5",
		},
		{
			name: "custom host and port",
			cfg: DatabaseConfig{
				Host:     "db.example.com",
				Port:     5433,
				User:     "admin",
				Password: "p@ssw0rd",
				Database: "production",
				SSLMode:  "require",
			},
			want: "host=db.example.com port=5433 user=admin password=p@ssw0rd dbname=production sslmode=require connect_timeout=5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.cfg.DSN())
		})
	}
}

const testWebhookSecret = "0123456789abcdef0123456789abcdef"

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "http://opencost:9003"},
			},
			wantErr: false,
		},
		{
			name: "negative daily history days",
			cfg: Config{
				Database:         DatabaseConfig{Password: "secret"},
				WebhookSecret:    testWebhookSecret,
				OpenCost:         OpenCostConfig{BaseURL: "http://opencost:9003"},
				DailyHistoryDays: -1,
			},
			wantErr: true,
		},
		{
			name: "negative backfill concurrency",
			cfg: Config{
				Database:            DatabaseConfig{Password: "secret"},
				WebhookSecret:       testWebhookSecret,
				OpenCost:            OpenCostConfig{BaseURL: "http://opencost:9003"},
				BackfillConcurrency: -1,
			},
			wantErr: true,
		},
		{
			name: "debug log level",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "http://opencost:9003"},
				LogLevel:      LogLevelDebug,
			},
			wantErr: false,
		},
		{
			name: "unknown log level",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "http://opencost:9003"},
				LogLevel:      "trace",
			},
			wantErr: true,
		},
		{
			name: "missing database password",
			cfg: Config{
				Database:      DatabaseConfig{Password: ""},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "http://opencost:9003"},
			},
			wantErr: true,
		},
		{
			name: "missing opencost base URL",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: ""},
			},
			wantErr: true,
		},
		{
			name: "opencost URL with non-http scheme rejected",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "file:///etc/passwd"},
			},
			wantErr: true,
		},
		{
			name: "opencost URL with ftp scheme rejected",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "ftp://opencost:9003"},
			},
			wantErr: true,
		},
		{
			name: "opencost URL with empty host rejected",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "http://"},
			},
			wantErr: true,
		},
		{
			name: "missing webhook secret",
			cfg: Config{
				Database: DatabaseConfig{Password: "secret"},
				OpenCost: OpenCostConfig{BaseURL: "http://opencost:9003"},
			},
			wantErr: true,
		},
		{
			name: "webhook secret shorter than 16 characters",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: "fifteen-chars-x",
				OpenCost:      OpenCostConfig{BaseURL: "http://opencost:9003"},
			},
			wantErr: true,
		},
		{
			name: "webhook secret of exactly 16 characters accepted",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: "sixteen-chars-xx",
				OpenCost:      OpenCostConfig{BaseURL: "http://opencost:9003"},
			},
			wantErr: false,
		},
		{
			name: "exporter needs no webhook secret",
			cfg: Config{
				Database:         DatabaseConfig{Password: "secret"},
				OpenCost:         OpenCostConfig{BaseURL: "http://opencost:9003"},
				Mode:             ModeExporter,
				MetricsPort:      9102,
				ExporterInterval: time.Minute,
			},
			wantErr: false,
		},
		{
			name: "exporter without a metrics port",
			cfg: Config{
				Database:         DatabaseConfig{Password: "secret"},
				OpenCost:         OpenCostConfig{BaseURL: "http://opencost:9003"},
				Mode:             ModeExporter,
				ExporterInterval: 5 * time.Minute,
			},
			wantErr: true,
		},
		{
			name: "exporter refreshing more than once a minute",
			cfg: Config{
				Database:         DatabaseConfig{Password: "secret"},
				OpenCost:         OpenCostConfig{BaseURL: "http://opencost:9003"},
				Mode:             ModeExporter,
				MetricsPort:      9102,
				ExporterInterval: 59 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "exporter still needs a usable OpenCost URL",
			cfg: Config{
				Database:         DatabaseConfig{Password: "secret"},
				OpenCost:         OpenCostConfig{BaseURL: "file:///etc/passwd"},
				Mode:             ModeExporter,
				MetricsPort:      9102,
				ExporterInterval: 5 * time.Minute,
			},
			wantErr: true,
		},
		{
			name: "unknown mode",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "http://opencost:9003"},
				Mode:          "worker",
			},
			wantErr: true,
		},
		{
			name: "metrics on the API port",
			cfg: Config{
				Server:        ServerConfig{Port: 8080},
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "http://opencost:9003"},
				MetricsPort:   8080,
			},
			wantErr: true,
		},
		{
			name: "metrics port out of range",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "http://opencost:9003"},
				MetricsPort:   65536,
			},
			wantErr: true,
		},
		{
			name: "cluster scope app accepted",
			cfg: Config{
				Database:        DatabaseConfig{Password: "secret"},
				WebhookSecret:   testWebhookSecret,
				OpenCost:        OpenCostConfig{BaseURL: "http://opencost:9003"},
				ClusterScopeApp: "argocd:cost-intelligence",
			},
			wantErr: false,
		},
		{
			name: "cluster scope off accepted",
			cfg: Config{
				Database:        DatabaseConfig{Password: "secret"},
				WebhookSecret:   testWebhookSecret,
				OpenCost:        OpenCostConfig{BaseURL: "http://opencost:9003"},
				ClusterScopeApp: "off",
			},
			wantErr: false,
		},
		{
			name: "cluster scope app without namespace",
			cfg: Config{
				Database:        DatabaseConfig{Password: "secret"},
				WebhookSecret:   testWebhookSecret,
				OpenCost:        OpenCostConfig{BaseURL: "http://opencost:9003"},
				ClusterScopeApp: "cost-intelligence",
			},
			wantErr: true,
		},
		{
			name: "cluster access app accepted",
			cfg: Config{
				Database:         DatabaseConfig{Password: "secret"},
				WebhookSecret:    testWebhookSecret,
				OpenCost:         OpenCostConfig{BaseURL: "http://opencost:9003"},
				ClusterAccessApp: "argocd:cost-intelligence",
			},
			wantErr: false,
		},
		{
			name: "cluster access app without namespace",
			cfg: Config{
				Database:         DatabaseConfig{Password: "secret"},
				WebhookSecret:    testWebhookSecret,
				OpenCost:         OpenCostConfig{BaseURL: "http://opencost:9003"},
				ClusterAccessApp: "cost-intelligence",
			},
			wantErr: true,
		},
		{
			name: "opencost HTTPS URL accepted",
			cfg: Config{
				Database:      DatabaseConfig{Password: "secret"},
				WebhookSecret: testWebhookSecret,
				OpenCost:      OpenCostConfig{BaseURL: "https://opencost.monitoring.svc:9003"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("DB_PASSWORD", "test-password")
	// Prevent host environment variables from leaking into the test.
	t.Setenv("SERVER_PORT", "8080")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "cost_user")
	t.Setenv("DB_NAME", "cost_intelligence")
	t.Setenv("DB_SSL_MODE", "disable")
	t.Setenv("OPENCOST_BASE_URL", "http://opencost:9003")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "cost_user", cfg.Database.User)
	assert.Equal(t, "test-password", cfg.Database.Password)
	assert.Equal(t, "cost_intelligence", cfg.Database.Database)
	assert.Equal(t, "disable", cfg.Database.SSLMode)
	assert.Equal(t, "http://opencost:9003", cfg.OpenCost.BaseURL)
	assert.Equal(t, ModeAPI, cfg.Mode)
	assert.Equal(t, 9102, cfg.MetricsPort)
	assert.Equal(t, 5*time.Minute, cfg.ExporterInterval)
	assert.Equal(t, 4, cfg.BackfillConcurrency)
	assert.Equal(t, LogLevelInfo, cfg.LogLevel)
	assert.False(t, cfg.Debug())
}

func TestLoadConfig_Overrides(t *testing.T) {
	t.Setenv("DB_PASSWORD", "override-password")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("DB_HOST", "remote-db")
	t.Setenv("OPENCOST_BASE_URL", "http://my-opencost:9003")
	t.Setenv("BACKFILL_CONCURRENCY", "0")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.True(t, cfg.Debug())

	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, 0, cfg.BackfillConcurrency)
	assert.Equal(t, "remote-db", cfg.Database.Host)
	assert.Equal(t, "http://my-opencost:9003", cfg.OpenCost.BaseURL)
}

func TestLoadConfig_LogLevelIgnoresCaseAndSpaces(t *testing.T) {
	t.Setenv("DB_PASSWORD", "test-password")
	t.Setenv("LOG_LEVEL", " DEBUG ")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.True(t, cfg.Debug())
	assert.Equal(t, LogLevelDebug, cfg.LogLevel, "read as debug")
}

func TestValidate_CostProvider(t *testing.T) {
	t.Parallel()

	base := func(provider, clusterID string) Config {
		return Config{
			Database:          DatabaseConfig{Password: "secret"},
			WebhookSecret:     testWebhookSecret,
			OpenCost:          OpenCostConfig{BaseURL: "http://kubecost-cost-analyzer:9090/model"},
			CostProvider:      provider,
			KubecostClusterID: clusterID,
		}
	}
	tests := []struct {
		name     string
		cfg      Config
		wantText string
	}{
		{name: "opencost by default", cfg: base("", "")},
		{name: "opencost", cfg: base(ProviderOpenCost, "")},
		{name: "kubecost with its cluster", cfg: base(ProviderKubecost, "eks-prod-1")},
		{name: "kubecost without a cluster", cfg: base(ProviderKubecost, ""), wantText: "KUBECOST_CLUSTER_ID is required"},
		{name: "unknown provider", cfg: base("cloudhealth", ""), wantText: `COST_PROVIDER "cloudhealth"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()

			if tt.wantText == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantText)
		})
	}
}

func TestValidate_ExporterModeChecksTheCostProviderToo(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Database:         DatabaseConfig{Password: "secret"},
		Mode:             ModeExporter,
		MetricsPort:      9102,
		ExporterInterval: 5 * time.Minute,
		OpenCost:         OpenCostConfig{BaseURL: "http://kubecost-cost-analyzer:9090/model"},
		CostProvider:     ProviderKubecost,
	}

	require.ErrorContains(t, cfg.Validate(), "KUBECOST_CLUSTER_ID is required")
}

func TestLoadConfig_CostProviderDefaultsToOpenCost(t *testing.T) {
	t.Setenv("DB_PASSWORD", "test-password")
	t.Setenv("OPENCOST_BASE_URL", "http://opencost:9003")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, ProviderOpenCost, cfg.CostProvider)
	assert.Empty(t, cfg.KubecostClusterID)
}

func TestLoadConfig_ReadsTheKubecostClusterMapFile(t *testing.T) {
	t.Setenv("DB_PASSWORD", "test-password")
	t.Setenv("OPENCOST_BASE_URL", "http://kubecost:9090/model")
	t.Setenv("COST_PROVIDER", "kubecost")
	t.Setenv("KUBECOST_CLUSTER_ID", "eks-prod-1")
	t.Setenv("KUBECOST_CLUSTER_MAP_FILE", "/etc/cost-intelligence/clusters.json")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, "/etc/cost-intelligence/clusters.json", cfg.KubecostClusterMapFile)
}

func TestAPIKeyHeader(t *testing.T) {
	tests := map[string]struct {
		cfg         Config
		name, value string
	}{
		"bearer by default":    {Config{CostProvider: ProviderKubecost, KubecostAPIKey: "k", KubecostAPIKeyHeader: "Authorization"}, "Authorization", "Bearer k"},
		"empty header name":    {Config{CostProvider: ProviderKubecost, KubecostAPIKey: "k"}, "Authorization", "Bearer k"},
		"custom header is raw": {Config{CostProvider: ProviderKubecost, KubecostAPIKey: "k", KubecostAPIKeyHeader: "X-Api-Key"}, "X-Api-Key", "k"},
		"no key":               {Config{CostProvider: ProviderKubecost}, "", ""},
		"opencost ignores it":  {Config{CostProvider: ProviderOpenCost, KubecostAPIKey: "k"}, "", ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			gotName, gotValue := tt.cfg.APIKeyHeader()
			assert.Equal(t, tt.name, gotName)
			assert.Equal(t, tt.value, gotValue)
		})
	}
}

func TestRetentionDays(t *testing.T) {
	assert.Equal(t, EKSOptimizedRetentionDays, (&Config{CostProvider: ProviderKubecost, KubecostEKSOptimized: true}).RetentionDays())
	assert.Zero(t, (&Config{CostProvider: ProviderKubecost}).RetentionDays())
	assert.Zero(t, (&Config{CostProvider: ProviderOpenCost, KubecostEKSOptimized: true}).RetentionDays())
}

func TestValidate_KubecostOnlySettingsNeedKubecost(t *testing.T) {
	cfg := Config{CostProvider: ProviderOpenCost, KubecostEKSOptimized: true, OpenCost: OpenCostConfig{BaseURL: "http://opencost:9003"}}

	assert.ErrorContains(t, cfg.validateCostBackend(), "only apply with COST_PROVIDER=kubecost")
}

func TestValidate_APIKeyHeaderName(t *testing.T) {
	cfg := Config{CostProvider: ProviderKubecost, KubecostClusterID: "c", KubecostAPIKey: "k", KubecostAPIKeyHeader: "Bad Header",
		OpenCost: OpenCostConfig{BaseURL: "http://kubecost:9090/model"}}

	assert.ErrorContains(t, cfg.validateCostBackend(), "not a valid header name")
}
