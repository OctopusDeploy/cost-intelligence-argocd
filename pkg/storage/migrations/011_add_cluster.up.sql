-- cluster is the cluster of the cost backend a record was measured in (Kubecost's cluster id; empty for
-- OpenCost, which measures only its own). Two clusters can run a namespace of the same name, and without
-- this column their records were taken for one namespace. Records measured before it have an empty
-- cluster, which reads as the backend's own: the next sync of an application replaces its record.
ALTER TABLE cost_actuals ADD COLUMN IF NOT EXISTS cluster VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_cost_actuals_cluster_deployment_namespace
    ON cost_actuals (cluster, deployment_namespace, measured_at DESC);
