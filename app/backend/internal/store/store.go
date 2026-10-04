package store

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

const MaxAudit = 5000

type Data struct {
	Users    map[string]*model.User
	Settings model.Settings
	Audit    []AuditEntry
	Seq      map[string]int
}

type AuditEntry struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Object string    `json:"object,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	d       Data
	version uint64

	persistMu sync.Mutex
	backend   Backend
	saved     uint64
	lastErr   error
}

func New() *Store {
	s := &Store{}
	s.d.init()
	return s
}

func (d *Data) init() {
	if d.Users == nil {
		d.Users = map[string]*model.User{}
	}
	if d.Seq == nil {
		d.Seq = map[string]int{}
	}
}

func (s *Store) Read(f func(d *Data)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f(&s.d)
}

func (s *Store) Write(f func(d *Data)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.version++
	f(&s.d)
}

func (d *Data) NextID(prefix string) string {
	d.Seq[prefix]++
	return fmt.Sprintf("%s-%d", prefix, d.Seq[prefix])
}

func (d *Data) AddAudit(a AuditEntry) {
	if a.At.IsZero() {
		a.At = time.Now().UTC()
	}
	d.Audit = append(d.Audit, a)
	if len(d.Audit) > MaxAudit {
		d.Audit = append([]AuditEntry(nil), d.Audit[len(d.Audit)-MaxAudit:]...)
	}
}

func (d *Data) UserByName(username string) *model.User {
	for _, u := range d.Users {
		if strings.EqualFold(u.Username, username) {
			return u
		}
	}
	return nil
}
