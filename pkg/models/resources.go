package models

import (
	"fmt"
	"regexp"
	"strings"
)

// ManagedResource is a resource Argo CD manages for an application, as its status lists it.
type ManagedResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// ManagedResources are the resources of one application.
type ManagedResources []ManagedResource

// nonPodOwnerKinds are the kinds Argo CD lists that never own pods. Any other kind, including a custom
// resource (CloudNativePG Cluster, Argo Workflow, SparkApplication, RayCluster, ...), may own pods
// directly, so it is kept and matched against the controller kind the cost backend reports.
var nonPodOwnerKinds = map[string]bool{
	"service":                  true,
	"serviceaccount":           true,
	"configmap":                true,
	"secret":                   true,
	"namespace":                true,
	"endpoints":                true,
	"endpointslice":            true,
	"ingress":                  true,
	"ingressclass":             true,
	"networkpolicy":            true,
	"persistentvolume":         true,
	"persistentvolumeclaim":    true,
	"storageclass":             true,
	"poddisruptionbudget":      true,
	"horizontalpodautoscaler":  true,
	"verticalpodautoscaler":    true,
	"resourcequota":            true,
	"limitrange":               true,
	"role":                     true,
	"rolebinding":              true,
	"clusterrole":              true,
	"clusterrolebinding":       true,
	"customresourcedefinition": true,
	"priorityclass":            true,
	"servicemonitor":           true,
	"podmonitor":               true,
	"prometheusrule":           true,
	"certificate":              true,
	"issuer":                   true,
	"clusterissuer":            true,
	"externalsecret":           true,
	"sealedsecret":             true,
	"application":              true,
	"applicationset":           true,
	"appproject":               true,
}

// resourceName is a Kubernetes object name (DNS-1123 subdomain).
var resourceName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$`)

// PodOwners keeps the resources that may own pods, dropping the kinds that never do (Services,
// ConfigMaps, ...). Returns an empty, non-nil list when there are none.
func (rs ManagedResources) PodOwners() ManagedResources {
	owners := ManagedResources{}
	for _, r := range rs {
		kind := strings.ToLower(r.Kind)
		// A custom resource with a name that is no Kubernetes name (some CRDs allow any) is left out
		// rather than failing the notification; a workload with one is a bad notification (Valid).
		if !nonPodOwnerKinds[kind] && (workloadKinds[kind] || resourceName.MatchString(r.Name)) {
			owners = append(owners, r)
		}
	}
	return owners
}

// Measured keeps the resources an application's cost is measured from: the ones that may own pods and
// its PersistentVolumeClaims, whose volumes cost money whether or not a pod mounts them. Claims never
// own pods: PodOwners drops them.
func (rs ManagedResources) Measured() ManagedResources {
	measured := rs.PodOwners()
	for _, r := range rs {
		if strings.EqualFold(r.Kind, "PersistentVolumeClaim") {
			measured = append(measured, r)
		}
	}
	return measured
}

// Claims returns the names of the PersistentVolumeClaims of the namespace the resources list. A claim
// with no namespace is the namespace's.
func (rs ManagedResources) Claims(namespace string) map[string]bool {
	claims := map[string]bool{}
	for _, r := range rs {
		if strings.EqualFold(r.Kind, "PersistentVolumeClaim") && (r.Namespace == "" || r.Namespace == namespace) {
			claims[r.Name] = true
		}
	}
	return claims
}

// Valid reports whether every resource has a valid Kubernetes object name, so names taken from a
// notification can safely be stored and matched.
func (rs ManagedResources) Valid() bool {
	for _, r := range rs {
		if !resourceName.MatchString(r.Name) {
			return false
		}
	}
	return true
}

// cronJobRun and rolloutReplicaSet match the suffixes of the controllers OpenCost reports for pods
// created on behalf of a CronJob (its Jobs, named <cronjob>-<scheduled time in minutes>) and of an Argo
// Rollout (its ReplicaSets, named <rollout>-<pod template hash>), which Argo CD lists under the parent
// only. The hash uses Kubernetes' safe alphabet, which has no vowels, so a Rollout named web does not
// claim a ReplicaSet named web-api.
var (
	cronJobRun        = regexp.MustCompile(`^-\d+$`)
	rolloutReplicaSet = regexp.MustCompile(`^-[bcdfghjklmnpqrstvwxz2456789]{5,10}$`)
)

// podSuffix matches the rest of a pod's name after its owner's name, for a pod whose controller the
// provider does not report (Kubecost's pod allocations carry none): a Deployment or Rollout pod is
// <name>-<pod template hash>-<5 characters>, a StatefulSet pod <name>-<ordinal>, a DaemonSet or Job pod
// <name>-<5 characters>.
var (
	replicaPodSuffix = regexp.MustCompile(`^-[bcdfghjklmnpqrstvwxz2456789]{5,10}-[a-z0-9]{5}$`)
	ordinalPodSuffix = regexp.MustCompile(`^-\d+$`)
	randomPodSuffix  = regexp.MustCompile(`^-[a-z0-9]{5}$`)
)

// OwnsPod reports whether one of the resources owns a pod. See OwnerOf.
func (rs ManagedResources) OwnsPod(namespace, controllerKind, controller, pod string) bool {
	_, ok := rs.OwnerOf(namespace, controllerKind, controller, pod)
	return ok
}

// kindOwnerLabels are the labels an operator puts on the pods it creates to name the custom resource
// they belong to, by the kinds whose name the label carries. Keys are normalised (see labelKey), as
// OpenCost and Kubecost report labels with dots, slashes and dashes turned into underscores.
var kindOwnerLabels = map[string][]string{
	labelKey("clickhouse.altinity.com/chi"):               {"clickhouseinstallation"},
	labelKey("cnpg.io/cluster"):                           {"cluster"},
	labelKey("strimzi.io/cluster"):                        {"kafka", "kafkaconnect", "kafkamirrormaker2", "kafkabridge"},
	labelKey("elasticsearch.k8s.elastic.co/cluster-name"): {"elasticsearch"},
	labelKey("kibana.k8s.elastic.co/name"):                {"kibana"},
}

// genericOwnerLabels name the owner on the pods of many operators (Pravega's ZookeeperCluster labels
// its pods app and release, RabbitMQ's operator app.kubernetes.io/name, ...). They only match a custom
// resource, never a workload kind, which the controller of the pod already identifies.
var genericOwnerLabels = []string{
	labelKey("app.kubernetes.io/instance"),
	labelKey("app.kubernetes.io/name"),
	labelKey("app"),
	labelKey("release"),
}

// generatedWorkloadSuffixes are the name suffixes of the workload an operator creates for a custom
// resource, by kind: the OpenTelemetry operator runs an OpenTelemetryCollector <name> as a
// Deployment, DaemonSet or StatefulSet <name>-collector, a TargetAllocator <name> as
// <name>-targetallocator. Argo CD lists only the custom resource.
var generatedWorkloadSuffixes = map[string][]string{
	"opentelemetrycollector": {"-collector"},
	"targetallocator":        {"-targetallocator"},
	"opampbridge":            {"-opamp-bridge"},
}

// ownsGenerated reports whether a pod belongs to the workload an operator generated for the resource.
func ownsGenerated(r ManagedResource, kind, controller, pod string) bool {
	for _, suffix := range generatedWorkloadSuffixes[strings.ToLower(r.Kind)] {
		base := r.Name + suffix
		if controller != "" {
			switch kind {
			case "deployment", "statefulset", "daemonset":
				if controller == base {
					return true
				}
			case "replicaset":
				if strings.HasPrefix(controller, base) && rolloutReplicaSet.MatchString(controller[len(base):]) {
					return true
				}
			}
			continue
		}
		if strings.HasPrefix(pod, base) {
			rest := pod[len(base):]
			if replicaPodSuffix.MatchString(rest) || ordinalPodSuffix.MatchString(rest) || randomPodSuffix.MatchString(rest) {
				return true
			}
		}
	}
	return false
}

// workloadKinds are the kinds a pod's controller identifies directly.
var workloadKinds = map[string]bool{
	"deployment": true, "statefulset": true, "daemonset": true, "replicaset": true,
	"job": true, "cronjob": true, "rollout": true, "pod": true,
}

// labelKey normalises a label name the way the cost backends report it.
func labelKey(k string) string {
	return strings.Map(func(r rune) rune {
		if r == '.' || r == '/' || r == '-' {
			return '_'
		}
		return r
	}, strings.ToLower(k))
}

// ConfigureOwnership extends the built-in lists above. It must be called once at startup, before any
// pod is matched.
//
// ownerLabels is a comma-separated list of labels naming the owner of a pod, each written
// "<label>=<Kind>|<Kind>" for one that names a resource of those kinds, or just "<label>" for one that
// names any custom resource, for example "foo.io/cluster=Foo|FooShard,example.com/owner". nonOwnerKinds
// is a comma-separated list of kinds Argo CD lists that never own pods, dropped from the notification.
func ConfigureOwnership(ownerLabels, nonOwnerKinds string) error {
	for _, entry := range strings.Split(ownerLabels, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		label, kinds, hasKinds := strings.Cut(entry, "=")
		label = strings.TrimSpace(label)
		if label == "" {
			return fmt.Errorf("owner label %q has no label name", entry)
		}
		if !hasKinds {
			genericOwnerLabels = append(genericOwnerLabels, labelKey(label))
			continue
		}
		var list []string
		for _, k := range strings.Split(kinds, "|") {
			if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
				list = append(list, k)
			}
		}
		if len(list) == 0 {
			return fmt.Errorf("owner label %q lists no kind after '='", entry)
		}
		key := labelKey(label)
		kindOwnerLabels[key] = append(kindOwnerLabels[key], list...)
	}
	for _, k := range strings.Split(nonOwnerKinds, ",") {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
			nonPodOwnerKinds[k] = true
		}
	}
	return nil
}

// unownedPodLabel is a label that attributes the pods that have no controller to the application of
// their namespace, for tools that create bare pods (Airflow's Kubernetes executor, Argo Workflows-like
// runners). An empty value matches any value.
type unownedPodLabel struct{ key, value string }

var unownedPodLabels []unownedPodLabel

// ConfigureUnownedPodLabels sets the labels that attribute controller-less pods to the only application
// deployed in their namespace: a comma-separated list of "<label>=<value>" or "<label>" (any value),
// for example "managedby=Airflow". It must be called once at startup.
func ConfigureUnownedPodLabels(spec string) error {
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key, value, _ := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("unowned pod label %q has no label name", entry)
		}
		unownedPodLabels = append(unownedPodLabels, unownedPodLabel{labelKey(key), strings.TrimSpace(value)})
	}
	return nil
}

// OwnsPodLabeled is OwnsPod for a pod whose labels the cost backend reports.
func (rs ManagedResources) OwnsPodLabeled(namespace, controllerKind, controller, pod string, labels map[string]string) bool {
	_, ok := rs.OwnerOfLabeled(namespace, controllerKind, controller, pod, labels)
	return ok
}

// OwnerOfLabeled is OwnerOf, then, for a pod no resource owns by its controller or its name, by its
// labels: an operator creates the StatefulSet or Deployment of a custom resource itself, so Argo CD
// lists only the resource while the pod's controller is one it generated (chi-<chi>-<cluster>-0-0).
// The pod belongs to a resource whose name is the value of a label naming its owner, in the
// resource's namespace.
func (rs ManagedResources) OwnerOfLabeled(namespace, controllerKind, controller, pod string, labels map[string]string) (ManagedResource, bool) {
	if r, ok := rs.OwnerOf(namespace, controllerKind, controller, pod); ok {
		return r, true
	}
	if len(labels) == 0 {
		return ManagedResource{}, false
	}
	normalised := make(map[string]string, len(labels))
	for k, v := range labels {
		normalised[labelKey(k)] = v
	}
	for _, r := range rs {
		if r.Namespace != "" && r.Namespace != namespace {
			continue
		}
		rk := strings.ToLower(r.Kind)
		for key, kinds := range kindOwnerLabels {
			if normalised[key] != r.Name {
				continue
			}
			for _, k := range kinds {
				if k == rk {
					return r, true
				}
			}
		}
		if workloadKinds[rk] {
			continue
		}
		for _, key := range genericOwnerLabels {
			if v := normalised[key]; v != "" && (v == r.Name || v == namespace+"."+r.Name) {
				return r, true
			}
		}
	}
	return ManagedResource{}, false
}

// OwnsUnownedPod reports whether a pod with no controller carries one of the unowned pod labels
// (UNOWNED_POD_LABELS) while the resources deploy to its namespace. Each application of a namespace
// would claim such a pod, so a caller applies it only to the namespace's sole application, which then
// owns every unowned pod there once.
func (rs ManagedResources) OwnsUnownedPod(namespace, controllerKind, controller string, labels map[string]string) bool {
	if controller != "" && !strings.EqualFold(controllerKind, "pod") {
		return false
	}
	normalised := make(map[string]string, len(labels))
	for k, v := range labels {
		normalised[labelKey(k)] = v
	}
	for _, u := range unownedPodLabels {
		if v, ok := normalised[u.key]; ok && (u.value == "" || u.value == v) {
			for _, r := range rs {
				if r.Namespace == "" || r.Namespace == namespace {
					return true
				}
			}
		}
	}
	return false
}

// OwnerOf returns the resource that owns a pod, given the namespace, controller kind and controller
// name the cost backend reports for it, and the pod's name. OpenCost reports a Deployment's pods under
// the Deployment, and a pod without a controller under no controller at all. A resource with no
// namespace (one Argo CD lists without it) matches in any namespace. A pod reported with no controller
// is matched by the shape of its name, as a pod without owner references is rare and one named like
// <resource>-<hash>-<suffix> is almost always the resource's.
func (rs ManagedResources) OwnerOf(namespace, controllerKind, controller, pod string) (ManagedResource, bool) {
	kind := strings.ToLower(controllerKind)
	for _, r := range rs {
		if r.Namespace != "" && r.Namespace != namespace {
			continue
		}
		rk := strings.ToLower(r.Kind)
		if ownsGenerated(r, kind, controller, pod) {
			return r, true
		}
		if controller == "" {
			if rk == "pod" && pod == r.Name {
				return r, true
			}
			if strings.HasPrefix(pod, r.Name) {
				rest := pod[len(r.Name):]
				switch rk {
				case "deployment", "rollout":
					if replicaPodSuffix.MatchString(rest) {
						return r, true
					}
				case "statefulset", "cluster":
					if ordinalPodSuffix.MatchString(rest) {
						return r, true
					}
				case "daemonset", "job":
					if randomPodSuffix.MatchString(rest) {
						return r, true
					}
				}
			}
			continue
		}
		switch rk {
		case "pod":
		case "cronjob":
			if (kind == "cronjob" && controller == r.Name) ||
				(kind == "job" && strings.HasPrefix(controller, r.Name) && cronJobRun.MatchString(controller[len(r.Name):])) {
				return r, true
			}
		case "rollout", "deployment":
			// A Deployment's pods are reported under the Deployment by OpenCost, but a provider that
			// does not resolve the owner reports its ReplicaSet instead.
			if (kind == rk && controller == r.Name) ||
				(kind == "replicaset" && strings.HasPrefix(controller, r.Name) && rolloutReplicaSet.MatchString(controller[len(r.Name):])) {
				return r, true
			}
		default:
			if kind == rk && controller == r.Name {
				return r, true
			}
		}
	}
	return ManagedResource{}, false
}
