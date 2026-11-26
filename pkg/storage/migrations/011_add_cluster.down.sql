DROP INDEX IF EXISTS idx_cost_actuals_cluster_deployment_namespace;
ALTER TABLE cost_actuals DROP COLUMN IF EXISTS cluster;
