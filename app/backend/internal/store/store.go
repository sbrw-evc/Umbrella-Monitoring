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
	Users       map[string]*model.User
	Roles       map[string]*model.Role
	Teams       map[string]*model.Team
	Services    map[string]*model.Service
	Connectors  map[string]*model.Connector
	Credentials map[string]*model.Credential
	ConfigItems map[string]*model.ConfigItem
	// NetBoxContacts maps NetBox contact IDs to the user accounts made or found for them.
	NetBoxContacts map[int]string
	NetBoxSync     model.SyncState
	Settings       model.Settings
	Audit          []AuditEntry
	Seq            map[string]int
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
	if d.Roles == nil {
		d.Roles = map[string]*model.Role{}
	}
	if d.Teams == nil {
		d.Teams = map[string]*model.Team{}
	}
	if d.Services == nil {
		d.Services = map[string]*model.Service{}
	}
	if d.Connectors == nil {
		d.Connectors = map[string]*model.Connector{}
	}
	if d.Credentials == nil {
		d.Credentials = map[string]*model.Credential{}
	}
	if d.ConfigItems == nil {
		d.ConfigItems = map[string]*model.ConfigItem{}
	}
	if d.NetBoxContacts == nil {
		d.NetBoxContacts = map[int]string{}
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

func (d *Data) EnsureSystemRoles(now time.Time) bool {
	added := false
	for _, r := range model.SystemRoles(now) {
		if d.Roles[r.ID] == nil {
			d.Roles[r.ID] = r
			added = true
		}
	}
	return added
}

func (d *Data) RoleOf(u *model.User) *model.Role {
	if r := d.Roles[u.Role]; r != nil {
		return r
	}
	id := model.RoleUser
	if u.Role == model.RoleAdmin {
		id = model.RoleAdmin
	}
	if r := d.Roles[id]; r != nil {
		return r
	}
	for _, r := range model.SystemRoles(time.Time{}) {
		if r.ID == id {
			return r
		}
	}
	return nil
}

func (d *Data) UserByName(username string) *model.User {
	for _, u := range d.Users {
		if strings.EqualFold(u.Username, username) {
			return u
		}
	}
	return nil
}
