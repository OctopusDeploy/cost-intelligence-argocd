package clustermap

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	listTimeout = 10 * time.Second
	retryAfter  = time.Minute
)

// Discovery knows the cluster names the cost backend reports, so an Argo CD cluster registered under
// the name Kubecost uses needs no entry in the map. The names are read when first asked, and again
// when ttl has passed; a failure keeps the last names and is tried again after a minute.
type Discovery struct {
	list func(ctx context.Context) ([]string, error)
	ttl  time.Duration
	now  func() time.Time

	mu      sync.Mutex
	ids     []string
	fetched time.Time // zero until a read succeeded
	failed  time.Time
}

// NewDiscovery returns a Discovery reading the names with list.
func NewDiscovery(list func(ctx context.Context) ([]string, error), ttl time.Duration) *Discovery {
	return &Discovery{list: list, ttl: ttl, now: time.Now}
}

// IDs are the cluster names last read, sorted.
func (d *Discovery) IDs() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	stale := d.fetched.IsZero() || now.Sub(d.fetched) >= d.ttl
	if stale && (d.failed.IsZero() || now.Sub(d.failed) >= retryAfter) {
		ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
		ids, err := d.list(ctx)
		cancel()
		if err != nil {
			d.failed = now
		} else {
			d.ids, d.fetched, d.failed = ids, now, time.Time{}
		}
	}
	return slices.Clone(d.ids)
}

// Match is the cluster name equal to name, ignoring case, if the backend reports one.
func (d *Discovery) Match(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	for _, id := range d.IDs() {
		if strings.EqualFold(id, name) {
			return id, true
		}
	}
	return "", false
}
