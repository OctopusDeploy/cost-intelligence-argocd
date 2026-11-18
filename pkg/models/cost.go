package models

import (
	"time"
)

type CostActual struct {
	ID                   string `json:"id"`
	ApplicationName      string `json:"applicationName"`
	ApplicationNamespace string `json:"applicationNamespace"`
	DeploymentNamespace  string `json:"deploymentNamespace"`
	// Cluster is the cluster of the cost backend the record was measured in (its id; empty when the
	// backend measures only its own, and for records stored before it was recorded).
	Cluster string `json:"cluster,omitempty"`
	// SyncTimestamp is when the sync that triggered the measurement finished. Zero for a measurement
	// taken at install, which follows no sync.
	SyncTimestamp time.Time `json:"syncTimestamp,omitzero"`
	MeasuredAt    time.Time `json:"measuredAt"`
	// MeasuredOn says what took the measurement: MeasuredOnSync or MeasuredOnInstall.
	MeasuredOn       string                `json:"measuredOn"`
	TotalMonthlyCost float64               `json:"totalMonthlyCost"`
	Resources        []ResourceCostActual  `json:"resources"`
	EstimateAccuracy float64               `json:"estimateAccuracy"`
	EstimateVsActual float64               `json:"estimateVsActual"`
	OpenCostOutput   OpenCostMetrics       `json:"opencostOutput"`
	Efficiency       *EfficiencyMetrics    `json:"efficiency,omitempty"`
	Pods             []PodCost             `json:"pods,omitempty"`
	Anomaly          *CostAnomaly          `json:"anomaly,omitempty"`
	CloudEstimate    *CloudServiceEstimate `json:"cloudEstimate,omitempty"`
	// IdleMonthlyCost is the part of the namespace's CPU and memory cost paid for requests its pods
	// did not use, projected like TotalMonthlyCost. Nil for records measured before it was, or without
	// the namespace's pods. Stored, never sent: Efficiency carries the same cost by resource.
	IdleMonthlyCost *float64 `json:"-"`
	// App is the application's own share of the measurement above, which covers its whole deployment
	// namespace. Nil for records stored before applications were measured on their own, or synced by a
	// notifications template that does not send the application's resources. Stored, never sent: the
	// application routes send ForApplication's view instead.
	App *AppMeasurement `json:"-"`
	// OwnedPods are the pods the application's measurement owns, by the backend's own rules (labels
	// included). Set only on the namespace routes, whose clients split a namespace's pods by application
	// and must not guess the owners again.
	OwnedPods []OwnedPod `json:"ownedPods,omitempty"`
	// Scope says what the cost fields describe: ScopeApplication or ScopeNamespace. Set only by
	// ForApplication.
	Scope string `json:"scope,omitempty"`
}

const (
	// MeasuredOnSync marks a measurement the sync webhook took after a sync.
	MeasuredOnSync = "sync"
	// MeasuredOnInstall marks a measurement the startup backfill took for an application that had no
	// record. It follows no sync, so it is never the baseline of a change since the last sync.
	MeasuredOnInstall = "install"
)

const (
	// ScopeApplication marks figures that cover only the pods the application's controllers own.
	ScopeApplication = "application"
	// ScopeNamespace marks figures that cover the application's whole deployment namespace.
	ScopeNamespace = "namespace"
)

// ScopeOf names what figures measured for resources cover: the pods they own, or the whole
// namespace when resources is nil.
func ScopeOf(resources ManagedResources) string {
	if resources == nil {
		return ScopeNamespace
	}
	return ScopeApplication
}

// MeasuredResources returns the resources the record measured its application by: the controllers
// Argo CD listed at the sync, or nil, meaning the whole deployment namespace, when the record has no
// application measurement. Never nil with one, so an application that owns no pods measures as
// nothing rather than as its namespace.
func (a CostActual) MeasuredResources() ManagedResources {
	if a.App == nil {
		return nil
	}
	return append(ManagedResources{}, a.App.Controllers...)
}

// AppMeasurement is the part of a namespace measurement that belongs to one application: the pods
// its controllers own, with their cost, breakdown and efficiency projected the same way.
type AppMeasurement struct {
	// Controllers are the pod-owning resources Argo CD listed for the application at the sync.
	Controllers      ManagedResources   `json:"controllers"`
	TotalMonthlyCost float64            `json:"totalMonthlyCost"`
	OpenCostOutput   OpenCostMetrics    `json:"opencostOutput"`
	Efficiency       *EfficiencyMetrics `json:"efficiency,omitempty"`
	Pods             []PodCost          `json:"pods"`
}

// OwnedPod identifies a pod an application's measurement owns.
type OwnedPod struct {
	Name           string `json:"name"`
	Controller     string `json:"controller,omitempty"`
	ControllerKind string `json:"controllerKind,omitempty"`
}

// WithOwnedPods returns the record with the pods its application's measurement owns listed. A pod's
// controller is listed once, so the controllers of pods that have come and gone since are still known.
func (a CostActual) WithOwnedPods() CostActual {
	if a.App == nil {
		return a
	}
	view := a
	view.OwnedPods = make([]OwnedPod, 0, len(a.App.Pods))
	for _, p := range a.App.Pods {
		view.OwnedPods = append(view.OwnedPods, OwnedPod{Name: p.Name, Controller: p.Controller, ControllerKind: p.ControllerKind})
	}
	return view
}

// ForApplication returns the record as the application sees it: its own cost, breakdown,
// efficiency and pods when they were measured, or the namespace's otherwise, marked by Scope.
func (a CostActual) ForApplication() CostActual {
	view := a
	view.App = nil
	if a.App == nil {
		view.Scope = ScopeNamespace
		return view
	}
	view.Scope = ScopeApplication
	view.TotalMonthlyCost = a.App.TotalMonthlyCost
	view.OpenCostOutput = a.App.OpenCostOutput
	view.Efficiency = a.App.Efficiency
	view.Pods = a.App.Pods
	return view
}

// CloudServiceResource is a single priced cloud resource from any IaC cost estimation tool.
// The model is intentionally tool-agnostic so that Infracost and future alternatives
// (e.g. Terracost) normalise into the same shape. The Source field on CloudServiceEstimate
// identifies which tool produced the data.
type CloudServiceResource struct {
	Name         string  `json:"name"`         // e.g. "aws_db_instance.payments-db"
	ResourceType string  `json:"resourceType"` // e.g. "aws_db_instance"
	MonthlyCost  float64 `json:"monthlyCost"`
}

// CloudServiceEstimate is a full cloud-services cost estimate pushed by an IaC cost tool.
// It is stored separately from OpenCost data and attached to CostActual at read time.
type CloudServiceEstimate struct {
	Source           string                 `json:"source"`           // "infracost" | "terracost"
	TotalMonthlyCost float64                `json:"totalMonthlyCost"` // sum across all projects
	Currency         string                 `json:"currency"`         // e.g. "USD"
	Resources        []CloudServiceResource `json:"resources"`
	PushedAt         time.Time              `json:"pushedAt"`
}

// AppCloudEstimate is an application's cloud estimate from one source.
type AppCloudEstimate struct {
	App      AppRef
	Estimate CloudServiceEstimate
}

// CostAnomaly is a computed field — never stored in the DB.
// Populated by the /actual handler when historical records show a cost spike.
type CostAnomaly struct {
	Severity    string  `json:"severity"`    // "warning" | "alert"
	Score       float64 `json:"score"`       // (current - avg) / avg, e.g. 0.5 = 50% above avg
	RollingAvg  float64 `json:"rollingAvg"`  // rolling average cost across RecordCount records
	RecordCount int     `json:"recordCount"` // number of historical records the average is based on
	Message     string  `json:"message"`     // human-readable description
}

type ClusterOverhead struct {
	ControlPlaneCost float64 `json:"controlPlaneCost"`
	Provider         string  `json:"provider"`
	Cluster          string  `json:"cluster"`
}

// DataQuality describes the trustworthiness of cost figures returned by OpenCost.
// PricingSource is "real" (cloud-billing-sourced), "estimated" (on-demand defaults), or "unknown".
type DataQuality struct {
	PricingSource string `json:"pricingSource"`
	Message       string `json:"message"`
}

// NamespaceSummary describes a deployment namespace by its most recent measurement.
type NamespaceSummary struct {
	Namespace string `json:"namespace"`
	// Cluster is the cluster of the cost backend the namespace was measured in; empty when the
	// backend measures only its own, and for records stored before it was recorded.
	Cluster   string  `json:"cluster,omitempty"`
	TotalCost float64 `json:"totalCost"`
	// IdleCost is the requested-but-unused CPU and memory cost from the same measurement. Nil when
	// that measurement did not measure it.
	IdleCost   *float64  `json:"idleCost"`
	AppCount   int       `json:"appCount"`
	MeasuredAt time.Time `json:"measuredAt"`
	// Clusters are the clusters the Applications deploying to the namespace run on, in the cost backend's
	// names. Set only when the backend measures several.
	Clusters []string `json:"clusters,omitempty"`
}

type PodCost struct {
	Name           string  `json:"name"`
	Namespace      string  `json:"namespace"`
	Controller     string  `json:"controller"`
	ControllerKind string  `json:"controllerKind"`
	CPUCost        float64 `json:"cpuCost"`
	RAMCost        float64 `json:"ramCost"`
	// StorageCost, NetworkCost (load balancers included) and GPUCost are the rest of what the pod
	// costs besides CPU and memory. They are omitted when zero, and records stored before they were
	// kept have none.
	StorageCost float64 `json:"storageCost,omitempty"`
	NetworkCost float64 `json:"networkCost,omitempty"`
	GPUCost     float64 `json:"gpuCost,omitempty"`
	// SharedCost is the part of TotalCost that is shared cost spread over the pod (set only when
	// shared costs are included), not the pod's own usage.
	SharedCost float64 `json:"sharedCost,omitempty"`
	// ExternalCost is the part of TotalCost that is external cost (cloud services Kubecost attributes to
	// the pod), set only when external costs are included.
	ExternalCost float64 `json:"externalCost,omitempty"`
	TotalCost    float64 `json:"totalCost"`
	// Group is the value of the configured grouping labels (UNMANAGED_GROUP_LABELS), for listing the
	// pods no Argo CD application manages by team or project. Presentation only: it never decides
	// which application owns a pod.
	Group string `json:"group,omitempty"`
}

type OrphanedPod struct {
	Name           string  `json:"name"`
	Namespace      string  `json:"namespace"`
	Controller     string  `json:"controller"`
	ControllerKind string  `json:"controllerKind"`
	CPUCost        float64 `json:"cpuCost"`
	RAMCost        float64 `json:"ramCost"`
	StorageCost    float64 `json:"storageCost"`
	TotalCost      float64 `json:"totalCost"`
	// Labels are the pod's, for the ownership rules that read them. Never sent.
	Labels map[string]string `json:"-"`
}

type EfficiencyMetrics struct {
	CPUEfficiency     float64 `json:"cpuEfficiency"`
	RAMEfficiency     float64 `json:"ramEfficiency"`
	TotalEfficiency   float64 `json:"totalEfficiency"`
	CPUCoresRequested float64 `json:"cpuCoresRequested"`
	CPUCoresUsed      float64 `json:"cpuCoresUsed"`
	RAMBytesRequested float64 `json:"ramBytesRequested"`
	RAMBytesUsed      float64 `json:"ramBytesUsed"`
	CPUIdleCost       float64 `json:"cpuIdleCost"`
	RAMIdleCost       float64 `json:"ramIdleCost"`
	// Average limits, from /allocation/summary cpuCoreLimitAverage / ramByteLimitAverage
	CPUCoresLimit float64 `json:"cpuCoresLimit"`
	RAMBytesLimit float64 `json:"ramBytesLimit"`
	// Peak usage, from rawAllocationOnly in /allocation
	CPUCoresUsedMax float64 `json:"cpuCoresUsedMax"`
	RAMBytesUsedMax float64 `json:"ramBytesUsedMax"`
}

type ResourceCostActual struct {
	Name              string  `json:"name"`
	Kind              string  `json:"kind"`
	Namespace         string  `json:"namespace"`
	ActualMonthlyCost float64 `json:"actualMonthlyCost"`
	CPUCost           float64 `json:"cpuCost"`
	MemoryCost        float64 `json:"memoryCost"`
	CPUUsage          string  `json:"cpuUsage"`
	MemoryUsage       string  `json:"memoryUsage"`
}

type OpenCostMetrics struct {
	Window           []string        `json:"window"`
	AllocationTotals AllocationTotal `json:"allocationTotals"`
	ResourceDetails  map[string]any  `json:"resourceDetails"`
	PeakCPUCores     float64         `json:"peakCpuCores,omitempty"`
	PeakRAMBytes     float64         `json:"peakRamBytes,omitempty"`
	// MeasuredHours is how much of the window OpenCost had data for; the monthly
	// costs are projected from it. Zero when OpenCost did not report it.
	MeasuredHours float64 `json:"measuredHours,omitempty"`
}

type AllocationTotal struct {
	CPUCost                float64 `json:"cpuCost"`
	MemoryCost             float64 `json:"memoryCost"`
	StorageCost            float64 `json:"storageCost"`
	GPUCost                float64 `json:"gpuCost"`
	NetworkCost            float64 `json:"networkCost"`
	NetworkCrossZoneCost   float64 `json:"networkCrossZoneCost,omitempty"`
	NetworkCrossRegionCost float64 `json:"networkCrossRegionCost,omitempty"`
	NetworkInternetCost    float64 `json:"networkInternetCost,omitempty"`
	NetworkNatGatewayCost  float64 `json:"networkNatGatewayCost,omitempty"`
	LoadBalancerCost       float64 `json:"loadBalancerCost"`
	TotalCost              float64 `json:"totalCost"`
	CarbonCost             float64 `json:"carbonCost"`
	SharedCost             float64 `json:"sharedCost,omitempty"`
	// ExternalCost is what Kubecost attributes to the allocations from outside the cluster (cloud
	// services matched from the bill). Reported always; part of TotalCost only when ExternalCostIncluded
	// (KUBECOST_EXTERNAL_COSTS), as the cloud services estimate may already cover them.
	ExternalCost         float64 `json:"externalCost,omitempty"`
	ExternalCostIncluded bool    `json:"externalCostIncluded,omitempty"`
	// SharedCostIncluded says SharedCost is part of TotalCost (it was asked for: tenancy costs or shared
	// namespaces). Otherwise SharedCost is only reported and TotalCost leaves it out.
	SharedCostIncluded  bool               `json:"sharedCostIncluded,omitempty"`
	SharedCostBreakdown map[string]float64 `json:"sharedCostBreakdown,omitempty"`
}

type DailyCost struct {
	Date        string  `json:"date"`
	TotalCost   float64 `json:"totalCost"`
	CPUCost     float64 `json:"cpuCost"`
	MemoryCost  float64 `json:"memoryCost"`
	NetworkCost float64 `json:"networkCost"`
	StorageCost float64 `json:"storageCost"`
}

// DailyProgress is how far an application's daily cost history has been recorded.
type DailyProgress struct {
	// Through is the last UTC day ("2006-01-02") recorded, whether or not OpenCost had any cost for it.
	Through string
	// Scope is what the recorded days measure: ScopeApplication or ScopeNamespace.
	Scope string
}

type ChargebackEntry struct {
	LabelValue  string  `json:"labelValue"`
	CPUCost     float64 `json:"cpuCost"`
	RAMCost     float64 `json:"ramCost"`
	GPUCost     float64 `json:"gpuCost"`
	StorageCost float64 `json:"storageCost"`
	NetworkCost float64 `json:"networkCost"`
	LBCost      float64 `json:"lbCost"`
	// SharedCost and ExternalCost are the parts of TotalCost that are shared and external cost, set
	// only when those are counted.
	SharedCost   float64 `json:"sharedCost,omitempty"`
	ExternalCost float64 `json:"externalCost,omitempty"`
	TotalCost   float64 `json:"totalCost"`
	// Efficiency is how much of their CPU and memory requests the label value's pods use, weighted
	// by cost; 0 when not measured or when NoRequests.
	Efficiency float64 `json:"efficiency"`
	// NoRequests is set when the label value was measured and its pods request no CPU or memory, so
	// it has no efficiency rather than an unmeasured one.
	NoRequests bool `json:"noRequests,omitempty"`
}

type SyncNotification struct {
	ApplicationName      string    `json:"applicationName"`
	ApplicationNamespace string    `json:"applicationNamespace"`
	Namespace            string    `json:"namespace"`
	SyncTimestamp        time.Time `json:"syncTimestamp"`
	SyncResult           string    `json:"syncResult"` // Succeeded, Failed, Unknown
	// DestinationServer and DestinationName are the cluster the application deploys to
	// ({{.app.spec.destination.server}} and {{.app.spec.destination.name}}; Argo CD sets one or the
	// other). Both are empty when the notifications template does not send them, and the application
	// is then taken to deploy to this cluster.
	DestinationServer string `json:"destinationServer"`
	DestinationName   string `json:"destinationName"`
	// Resources are the resources Argo CD manages for the application ({{.app.status.resources}}).
	// Nil when the notifications template does not send them, and the application is then measured
	// only as its whole namespace; empty when it manages none.
	Resources []ManagedResource `json:"resources"`
}
