// Package store keeps Umbrella state. The MVP step 1 store is in memory
// behind one lock; the PostgreSQL implementation replaces it without
// changing callers, because everything goes through Read and Write.
package store

import (
	"fmt"
	"sync"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

// Limits of the in-memory rings.
const (
	MaxEvents      = 20000
	MaxParseErrors = 2000
)

// Data is the whole state. Access it only inside Read or Write.
type Data struct {
	Teams       []model.Team
	CIs         map[string]*model.CI
	Relations   []model.Relation
	Connectors  map[string]*model.Connector
	Events      []*model.Event // oldest first
	Alerts      map[string]*model.Alert
	ParseErrors []*model.ParseError // oldest first
	Maintenance map[string]*model.Maintenance
	Rules       []model.Rule
	Audit       []AuditEntry
	seq         map[string]int
}

// AuditEntry records a change made by a person.
type AuditEntry struct {
	At     string `json:"at"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Object string `json:"object"`
}

// Store guards Data.
type Store struct {
	mu sync.RWMutex
	d  Data
}

// New returns an empty store.
func New() *Store {
	return &Store{d: Data{
		CIs:         map[string]*model.CI{},
		Connectors:  map[string]*model.Connector{},
		Alerts:      map[string]*model.Alert{},
		Maintenance: map[string]*model.Maintenance{},
		seq:         map[string]int{},
	}}
}

// Read runs f under the read lock. f must not keep pointers after return.
func (s *Store) Read(f func(d *Data)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f(&s.d)
}

// Write runs f under the write lock.
func (s *Store) Write(f func(d *Data)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.d)
}

// NextID returns a readable sequential id like "ALR-42". Call inside Write.
func (d *Data) NextID(prefix string) string {
	d.seq[prefix]++
	return fmt.Sprintf("%s-%d", prefix, d.seq[prefix])
}

// AddEvent appends to the ring.
func (d *Data) AddEvent(e *model.Event) {
	d.Events = append(d.Events, e)
	if len(d.Events) > MaxEvents {
		d.Events = append([]*model.Event(nil), d.Events[len(d.Events)-MaxEvents:]...)
	}
}

// AddParseError appends to the ring.
func (d *Data) AddParseError(p *model.ParseError) {
	d.ParseErrors = append(d.ParseErrors, p)
	if len(d.ParseErrors) > MaxParseErrors {
		d.ParseErrors = append([]*model.ParseError(nil), d.ParseErrors[len(d.ParseErrors)-MaxParseErrors:]...)
	}
}

// AddAudit appends an audit line.
func (d *Data) AddAudit(a AuditEntry) {
	d.Audit = append(d.Audit, a)
	if len(d.Audit) > 5000 {
		d.Audit = d.Audit[len(d.Audit)-5000:]
	}
}
