// Package clustermap maps the clusters Argo CD deploys to onto the names Kubecost knows them by.
package clustermap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/OctopusDeploy/cost-intelligence-argocd/pkg/exporter"
)

// Cluster is one Argo CD cluster and its name in Kubecost. Argo CD names a destination by its API
// server URL or by the name the cluster is registered under, so an Application may use either; give
// the one or both an Application of this cluster might use.
type Cluster struct {
	// ID is the cluster's name in Kubecost, the value of its "cluster" property.
	ID string `json:"id"`
	// Server is the cluster's API server URL as Argo CD has it (spec.destination.server).
	Server string `json:"server,omitempty"`
	// Name is the name the cluster is registered under in Argo CD (spec.destination.name).
	Name string `json:"name,omitempty"`
}

// file is the content of the mounted ConfigMap key.
type file struct {
	Clusters []Cluster `json:"clusters"`
}

// Map is an exporter.Scope reading the cluster of every Application that Kubecost can tell apart.
type Map struct {
	local     string
	clusters  []Cluster
	discovery *Discovery
	byServer  map[string]string
	byName    map[string]string
}

// New returns a Map of the clusters, with localID the name in Kubecost of the cluster Argo CD runs in,
// which needs no entry. A server or a name may belong to one cluster only.
func New(localID string, clusters []Cluster) (*Map, error) {
	if localID == "" {
		return nil, errors.New("the Kubecost name of the local cluster is required")
	}
	m := &Map{local: localID, clusters: clusters, byServer: map[string]string{}, byName: map[string]string{}}
	for i, c := range clusters {
		switch {
		case c.ID == "":
			return nil, fmt.Errorf("cluster %d: id is required", i+1)
		case c.Server == "" && c.Name == "":
			return nil, fmt.Errorf("cluster %q: server or name is required", c.ID)
		}
		if err := add(m.byServer, "server", c.Server, c.ID); err != nil {
			return nil, err
		}
		if err := add(m.byName, "name", strings.ToLower(c.Name), c.ID); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func add(index map[string]string, kind, key, id string) error {
	if key == "" {
		return nil
	}
	if other, ok := index[key]; ok && other != id {
		return fmt.Errorf("%s %q is mapped to both %q and %q", kind, key, other, id)
	}
	index[key] = id
	return nil
}

// Parse reads the JSON of the mounted ConfigMap key: {"clusters":[{"id":"...","server":"...","name":"..."}]}.
func Parse(localID string, data []byte) (*Map, error) {
	var f file
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("invalid cluster map: %w", err)
	}
	return New(localID, f.Clusters)
}

// Load reads the cluster map at path. An empty path is a map of the local cluster only.
func Load(localID, path string) (*Map, error) {
	if path == "" {
		return New(localID, nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cluster map: %w", err)
	}
	return Parse(localID, data)
}

// WithDiscovery has the map take a cluster named as the backend names it, when no entry has it.
func (m *Map) WithDiscovery(d *Discovery) *Map {
	m.discovery = d
	return m
}

// Known are the cluster names the backend reports, none without discovery.
func (m *Map) Known() []string {
	if m.discovery == nil {
		return nil
	}
	return m.discovery.IDs()
}

// Clusters are the entries of the map, the other clusters whose Applications are measured.
func (m *Map) Clusters() []Cluster { return m.clusters }

// ClusterFor is the Kubecost name of the cluster dest deploys to. The local cluster is the one Argo CD
// runs in; any other is in the map, by its server or its name, or is registered under a name Kubecost
// uses itself. One that is neither is not measured.
func (m *Map) ClusterFor(dest exporter.Destination) (string, bool) {
	if dest.Local() {
		return m.local, true
	}
	if id, ok := m.byServer[dest.Server]; ok && dest.Server != "" {
		return id, true
	}
	if id, ok := m.byName[strings.ToLower(dest.Name)]; ok && dest.Name != "" {
		return id, true
	}
	if m.discovery != nil {
		return m.discovery.Match(dest.Name)
	}
	return "", false
}
