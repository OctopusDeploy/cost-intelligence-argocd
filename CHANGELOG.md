# Changelog

All notable changes to this project are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Until 1.0, a minor version may include breaking changes. They are listed under **Breaking**.

## [Unreleased]

### Added

- Experimental support for Kubecost 3.1 with its aggregator as the cost backend, next to OpenCost (`COST_PROVIDER=kubecost`, Helm `openCost.provider`). Set `openCost.baseURL` to Kubecost's `/model` endpoint. `KUBECOST_CLUSTER_ID` (Helm `openCost.kubecostClusterID`) names this cluster in Kubecost and is required, because an aggregator answers for every cluster it knows. OpenCost is unchanged and stays the default.
- Applications that deploy to other clusters can be measured with Kubecost. `KUBECOST_CLUSTER_MAP_FILE` (Helm `openCost.kubecostClusters`, rendered into a ConfigMap) maps an Argo CD destination, by server URL or registered name, to its cluster in Kubecost. The Cost Intelligence tab, the per-application views (daily cost, rightsizing, orphaned pods) and the waste ranking follow the map; a destination that is not mapped is still not measured. The same cluster registered in Argo CD under two names, for example `in-cluster` and `prod`, needs one entry for the second name.
- With several clusters in the cluster map, the Namespaces, Idle spend and Chargeback pages have a cluster picker (`?costCluster=`); each reads that cluster's costs. The cluster cost says it is projected from the last 24 hours onto 30 days. With Kubecost, pricing is no longer checked through an endpoint Kubecost does not have, which showed as "Unknown, status 404". It is read from Kubecost's cloud billing integrations (`/cloudCost/status`): "Real" when one such as the AWS CUR is active and valid. A Kubecost aggregator doesn't serve that endpoint, so `KUBECOST_CLOUD_COST_URL` (Helm `openCost.cloudCostURL`) names the service that does, Kubecost's cloud-cost service.
- An Argo CD cluster registered under the name Kubecost uses for it needs no cluster map entry: the service reads Kubecost's cluster names (`KUBECOST_DISCOVER_CLUSTERS`, Helm `openCost.discoverClusters`, on by default). For one that is not matched, the Cost Intelligence tab lists the names Kubecost knows.
- On the Namespaces page a namespace is its cluster and its name: with all clusters shown, one name on two clusters is two rows, and opening one reads that cluster's applications (`?costNs=<cluster>/<namespace>`).
- A protected Kubecost aggregator can be queried with an API key read from a Secret (`KUBECOST_API_KEY`, `KUBECOST_API_KEY_HEADER`; Helm `openCost.apiKey`).
- `KUBECOST_EKS_OPTIMIZED` (Helm `openCost.eksOptimized`) for the EKS-optimized Kubecost, which keeps 15 days: the daily history is read in windows that fit, history already stored is kept, and the UI draws ranges longer than about three months by week instead of by month.
- Kubecost can spread costs no application owns over the applications: `KUBECOST_SHARE_TENANCY_COSTS` (Helm `openCost.shareTenancyCosts`) for its tenancy costs and `KUBECOST_SHARE_NAMESPACES` (Helm `openCost.shareNamespaces`) for the cost of named namespaces such as `kube-system`, spread within each namespace's own cluster (Kubecost itself would pool it across all clusters). Both are off by default and count in the namespace and application costs and the daily history, not in the exporter's per-resource gauges. The cost breakdown lists it as its own "Shared cost" row, named by source (for example "Namespace kube-system"), with a note that it is not the application's own usage.
- `KUBECOST_EXTERNAL_COSTS` (Helm `openCost.externalCosts`), Kubecost only, counts the external cost Kubecost attributes to each allocation (cloud services matched from the bill) in the namespace and application costs. The namespace and application views get an External column, and the cost breakdown an "External cost" row. Off by default, because it can overlap the cloud service estimates; the external cost is still reported in the allocation totals, outside the total. The chargeback view gets Shared (tenancy) and External columns under the same flags.

- Each cost record stores the cluster it was measured in (migration 011), so a namespace is its cluster and its name: two clusters running a namespace of the same name are two rows on the Namespaces page, and the namespace routes read the cluster picked with `?cluster=` (all of them without it). Records stored before this have no cluster and match any, until the application's next sync replaces them.

- With several clusters, the cluster-wide page opens on all of them: the total is all clusters' monthly cost, each namespace names its clusters, and the cluster picker filters the namespaces to one. The Idle spend ranking and its namespace list follow the picked cluster; Chargeback still reads one cluster at a time, and says so.
- The application's monthly cost (Kubernetes plus cloud estimate) shows in Argo CD's status panel, next to Last Sync. The idle part is already inside the Kubernetes cost, so it is shown as detail and not added. It is hidden for an application on a cluster that is not measured.
- Pods of operator-generated workloads are attributed to the custom resource Argo CD lists, by the pod's labels: a list of built-in owner labels (ClickHouse, CloudNativePG, Strimzi, Elastic, OpenTelemetry operator, the `app`/`app.kubernetes.io/*`/`release` labels) and `OWNER_LABELS` (Helm `costIntelligence.ownerLabels`) for others. Pods of any resource kind are matched, not an allowlist; `NON_OWNER_KINDS` (`costIntelligence.nonOwnerKinds`) names kinds that never own pods.
- Pods with no controller (bare pods, such as GitLab Runner's CI job pods or Airflow's executor pods) are charged to the only Application deployed in their namespace when they carry one of `UNOWNED_POD_LABELS` (`costIntelligence.unownedPodLabels`). Off by default; `project.runner.gitlab.com/name` charges a GitLab Runner's job pods to the runner's application. With several Applications in the namespace they stay in the namespace's cost. Each pod is counted once.
- `UNMANAGED_GROUP_LABELS` (`costIntelligence.unmanagedGroupLabels`, off by default) groups pods in the namespace view and in an application's pod list, so CI job pods read per team and project. Presentation only.
- Volumes: claimed volumes no pod mounts are charged to the application that lists their PersistentVolumeClaim, and, when exactly one Application deploys to the namespace, every unmounted volume there is its. With several, the rest stays in the namespace's cost.
- The Namespaces page lists the applications of the picked cluster only, not those of other clusters.
- `GET /{appName}/month` and the status panel give the calendar month (UTC): what the application has cost so far, read from the daily history, so pods that no longer exist count, and the month projected at the average daily cost of the last 7 complete days. A job that ran once is a seventh of a day's run rate, not a run per day. The month is what the cloud bill covers.
- The namespace view's application and controller rows show GPU, Storage and Network columns, like the application's controller list, when some row has that cost.

### Fixed

- Kubecost's cost adjustments (`cpuCostAdjustment`, `networkCostAdjustment`, ...) are added to the costs they adjust. Kubecost reconciles with the cloud bill and reports the correction beside each cost, so a pod's $2.99 of network cost the provider never charged was projected as it was.
- A pod that has ended is projected from the whole measured window, not from its own run time: a job that ran for 76 minutes was multiplied as if it had run all day.
- A daily-history entry that covers more than a day (Kubecost keeps one entry per month for old data) is skipped, and no longer stored as the cost of its first day.

- The pods of a CloudNativePG `Cluster` are counted in the application that declares it. Argo CD lists the Cluster, which owns its pods directly, and it was not one of the kinds whose pods make up an application's cost.
- The cluster picked on the cluster-wide page was ignored by the namespace list, the idle spend ranking and the other cluster-wide reads, because the page and those reads used two separate API clients. They now share one.
- With Kubecost, which reports pods without a controller, an application owned none of its pods: its cost view showed 0 controllers and 0 pods and the namespace page listed them as unmanaged. A pod with no reported controller is now matched to its Deployment, Rollout, StatefulSet, DaemonSet or Job by the shape of its name, and a Deployment also owns its ReplicaSets.
- The `__idle__` and `__unmounted__` entries are no longer listed as pods.
- An Argo CD cluster name is matched to the cluster map ignoring case, and a refused read of the service's cluster list is retried instead of making every application look remote until the page reloads. The "Deploys to another cluster" panel now says which clusters the service's map lists.

## [0.2.0] - 2026-11-05

### Changed

- **Breaking:** the project is now called Cost Intelligence for Argo CD, and its technical names follow.
  - The repository, Go module, chart, images and docs site are now `cost-intelligence-argocd`.
  - The default namespace is now `cost-intelligence`.
  - Metrics now start with `cost_intelligence_` and alerts with `CostIntelligence`, so dashboards, custom rules, Alertmanager routes and silences need updating.
  - To upgrade an existing install without losing its data, see [Upgrade from 0.1.0](docs/installation.md#upgrade-from-010).

## [0.1.0] - 2026-11-01

The first public release.

### Added

- A sync webhook (`POST /api/cost-analysis/on-sync`). Argo CD Notifications calls it after each successful sync. The service then asks OpenCost what the application's namespace and its own pods cost, projects that onto a 30-day month from the hours OpenCost has data for, and stores it in PostgreSQL. The webhook needs a bearer token (`WEBHOOK_SECRET`). When OpenCost has less than 5 minutes of data for the namespace, nothing is stored and the namespace is measured again, waiting 1, 2, 4 and then 8 minutes between tries.
- Costs right after install. When the service starts, it measures each Application that has no cost yet, instead of waiting for its next sync. These records are marked "Measured at install" and are never compared with a later sync. `BACKFILL_CONCURRENCY` (Helm `costIntelligence.backfillConcurrency`, 4 by default, `0` to turn it off) bounds how many namespaces are measured at once.
- A REST API behind Argo CD's extension proxy. Application routes only answer for the application named in the `Argocd-Application-Name` header. Cluster-wide routes answer for a gate Application: the one in `CLUSTER_SCOPE_APP`, otherwise the Argo CD Application that installed Cost Intelligence, otherwise the chart's `cost-intelligence-access` Application. Anyone who can `get` the gate sees every namespace's cost. `CLUSTER_SCOPE_APP=off` turns cluster-wide routes off.
- A UI plugin for Argo CD 3.4, 3.5 and 3.6, in Octopus Deploy's look:
  - On each application, a Cost Intelligence tab with actual costs (split by CPU, memory, network and storage), idle spend, efficiency, what changed since the last sync, a resources view and a trends view.
  - A system-level page with cost per namespace, idle spend per application with the saving from lowering its CPU request, chargeback by Kubernetes label and the control-plane fee.
- Trends with a date range, a month-end forecast and anomaly notices. The service records each application's daily cost itself (`DAILY_HISTORY_DAYS`, 400 by default), so the history reaches back further than OpenCost's Prometheus.
- Rightsizing from Vertical Pod Autoscaler recommendations in `Off` mode (including those Goldilocks creates), priced with OpenCost, once a recommendation has a day of history. It only suggests requests, never limits.
- Cloud service costs: Infracost estimates pushed from CI, shown next to the Kubernetes cost.
- Prometheus metrics on a port of their own (`METRICS_PORT`, 9102). A single exporter publishes each Application's cost per hour, and every API replica reports its own health. The chart can add ServiceMonitors and a PrometheusRule with a recording rule and starter alerts. See [docs/metrics.md](docs/metrics.md).
- A Helm chart, published to `oci://ghcr.io/octopusdeploy/charts/argocd-cost-intelligence`. It deploys the backend, the exporter, the UI plugin and a bundled PostgreSQL, with NetworkPolicies that only let Argo CD's server, notifications controller and repo server reach the backend's API. It checks the type of each value, so a wrongly typed value stops `helm install` with an error naming the key. A post-install hook registers the extension and the webhook in Argo CD, installed with `install.yaml` or its Helm chart, and lets `role:admin` open the extension (`argoCd.rbac.grantAdmin`). It only writes keys of its own, plus its entry in `subscriptions`. `postInstallHook.extensionInitImage` sets the image that downloads the plugin, for clusters that mirror images. Multi-arch images (amd64 and arm64) are on GHCR.
- A PostgreSQL you already run, with `postgres.enabled=false`: `postgres.host` and `postgres.port` point the chart at it, or an ExternalName Service called `postgres` when `postgres.host` is empty.
- A guide for installing when Git manages Argo CD's own configuration, with the hook turned off. See [docs/gitops-install.md](docs/gitops-install.md). Each release attaches the UI plugin as `extension.tar.gz`, with its checksum in `extension_checksums.txt`, for the Argo CD extension installer.
- Applications that deploy to another cluster are never measured, since OpenCost only sees the cluster it runs in, and their tab shows which cluster they deploy to. `PRUNE_REMOTE_COST` (Helm `costIntelligence.pruneRemoteCost`, off by default) deletes cost already stored for them, and can't be undone.
- `LOG_LEVEL` (Helm `costIntelligence.logLevel`, `info` by default). OpenCost request URLs and responses, which name each namespace and its cost, are only logged at `debug`.

### Security

- Application data is scoped to the application in `Argocd-Application-Name`. Argo CD's proxy checks that header against the viewer's RBAC but not the request path, so the service checks the path itself.
- Records are keyed by the Application's namespace and name, so `team-a/web` and `team-b/web` stay separate with apps-in-any-namespace.
- `/on-sync` needs `Authorization: Bearer <WEBHOOK_SECRET>`. The backend won't start with a secret shorter than 16 characters.
- The NetworkPolicies are what make the headers trustworthy, so they need a CNI that enforces NetworkPolicy. k3s and k3d do, unless k3s was started with `--disable-network-policy`.

[Unreleased]: https://github.com/OctopusDeploy/cost-intelligence-argocd/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/OctopusDeploy/cost-intelligence-argocd/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/OctopusDeploy/cost-intelligence-argocd/releases/tag/v0.1.0
