package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

func TestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := New()
	if found, err := a.Open(dir); err != nil || found {
		t.Fatalf("empty dir: %v %v", found, err)
	}
	a.Write(func(d *Data) {
		id := d.NextID("USR")
		d.Users[id] = &model.User{ID: id, Username: "admin", PasswordHash: "pbkdf2$x"}
		d.Alerts["INC-1"] = &model.Alert{ID: "INC-1", Title: "disk", FirstSeen: time.Now()}
		d.PagerDuty.Enabled = true
		d.PagerDuty.RoutingKeyRef = "openbao://umbrella/pagerduty#routing_key"
	})
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	a.Write(func(d *Data) { d.Settings.GrafanaURL = "https://grafana/d/x" })
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}

	b := New()
	if found, err := b.Open(dir); err != nil || !found {
		t.Fatalf("reopen: %v %v", found, err)
	}
	b.Read(func(d *Data) {
		u := d.UserByName("admin")
		if u == nil || u.PasswordHash != "pbkdf2$x" {
			t.Fatalf("user = %+v", u)
		}
		if d.Alerts["INC-1"] == nil || !d.PagerDuty.Enabled || d.Settings.GrafanaURL == "" {
			t.Fatalf("state lost: %+v", d.PagerDuty)
		}
	})
	b.Write(func(d *Data) {
		if id := d.NextID("USR"); id != "USR-2" {
			t.Fatalf("sequence restarted: %s", id)
		}
	})
}

func TestCorruptSnapshotFallsBackToBackup(t *testing.T) {
	dir := t.TempDir()
	a := New()
	a.Open(dir)
	a.Write(func(d *Data) { d.Settings.GrafanaURL = "one" })
	a.Flush()
	a.Write(func(d *Data) { d.Settings.GrafanaURL = "two" })
	a.Flush()
	os.WriteFile(filepath.Join(dir, snapshotFile), []byte("garbage"), 0o600)
	b := New()
	if _, err := b.Open(dir); err != nil {
		t.Fatal(err)
	}
	b.Read(func(d *Data) {
		if d.Settings.GrafanaURL != "one" {
			t.Fatalf("backup not used: %q", d.Settings.GrafanaURL)
		}
	})
	os.WriteFile(filepath.Join(dir, snapshotFile+".bak"), []byte("garbage"), 0o600)
	if _, err := New().Open(dir); err == nil {
		t.Fatal("corrupt snapshots accepted")
	}
}
