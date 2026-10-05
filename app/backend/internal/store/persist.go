package store

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	snapshotFile   = "umbrella.gob"
	snapshotFormat = 1
)

var ErrNoState = errors.New("сохранённого состояния нет")

type Backend interface {
	Kind() string
	Where() string
	Load(ctx context.Context) ([][]byte, error)
	Save(ctx context.Context, data []byte) error
	Close()
}

type snapshot struct {
	Format  int
	SavedAt time.Time
	Data    Data
}

func decode(b []byte) (snapshot, error) {
	var snap snapshot
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&snap); err != nil {
		return snap, err
	}
	if snap.Format != snapshotFormat {
		return snap, fmt.Errorf("неизвестный формат снимка %d", snap.Format)
	}
	return snap, nil
}

type FileBackend struct{ Dir string }

func (f FileBackend) Kind() string  { return "file" }
func (f FileBackend) Where() string { return f.Dir }
func (f FileBackend) Close()        {}

func (f FileBackend) Load(context.Context) ([][]byte, error) {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("каталог данных %s: %w", f.Dir, err)
	}
	var out [][]byte
	main := filepath.Join(f.Dir, snapshotFile)
	for _, p := range []string{main, main + ".bak"} {
		b, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil, ErrNoState
	}
	return out, nil
}

func (f FileBackend) Save(_ context.Context, data []byte) error {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(f.Dir, snapshotFile), data)
}

func (s *Store) Open(dir string) (bool, error) {
	return s.Attach(context.Background(), FileBackend{Dir: dir})
}

func (s *Store) Attach(ctx context.Context, b Backend) (bool, error) {
	cands, err := b.Load(ctx)
	if errors.Is(err, ErrNoState) {
		s.persistMu.Lock()
		s.backend = b
		s.persistMu.Unlock()
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var lastErr error
	for i, c := range cands {
		snap, err := decode(c)
		if err != nil {
			lastErr = err
			slog.Error("snapshot is unreadable", "backend", b.Kind(), "where", b.Where(), "candidate", i, "err", err)
			continue
		}
		s.install(snap)
		s.persistMu.Lock()
		s.backend = b
		s.persistMu.Unlock()
		slog.Info("state restored", "backend", b.Kind(), "where", b.Where(), "saved_at", snap.SavedAt,
			"alerts", len(snap.Data.Alerts), "connectors", len(snap.Data.Connectors))
		return true, nil
	}
	return false, fmt.Errorf("снимок состояния повреждён, запуск остановлен, чтобы не перезаписать его: %w", lastErr)
}

func (s *Store) install(snap snapshot) {
	snap.Data.init()
	s.mu.Lock()
	s.d = snap.Data
	s.version++
	v := s.version
	s.mu.Unlock()
	s.persistMu.Lock()
	s.saved = v
	s.persistMu.Unlock()
}

func (s *Store) Switch(ctx context.Context, b Backend, adopt bool) (bool, error) {
	if adopt {
		if cands, err := b.Load(ctx); err == nil {
			for _, c := range cands {
				if snap, err := decode(c); err == nil {
					s.install(snap)
					s.replaceBackend(b)
					return true, nil
				}
			}
			return false, errors.New("в базе есть снимок Umbrella, но его нельзя прочитать")
		} else if !errors.Is(err, ErrNoState) {
			return false, err
		}
	}
	data, err := s.encode()
	if err != nil {
		return false, err
	}
	if err := b.Save(ctx, data); err != nil {
		return false, err
	}
	s.replaceBackend(b)
	return false, nil
}

func (s *Store) replaceBackend(b Backend) {
	s.persistMu.Lock()
	old := s.backend
	s.backend = b
	s.mu.RLock()
	s.saved = s.version
	s.mu.RUnlock()
	s.persistMu.Unlock()
	if old != nil && old != b {
		old.Close()
	}
}

func (s *Store) encode() ([]byte, error) {
	var buf bytes.Buffer
	s.mu.RLock()
	defer s.mu.RUnlock()
	err := gob.NewEncoder(&buf).Encode(snapshot{Format: snapshotFormat, SavedAt: time.Now(), Data: s.d})
	return buf.Bytes(), err
}

func (s *Store) Persistent() bool {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	return s.backend != nil
}

type PersistStatus struct {
	Enabled bool   `json:"enabled"`
	Kind    string `json:"kind,omitempty"`
	Where   string `json:"where,omitempty"`
	Dir     string `json:"dir,omitempty"`
	Pending bool   `json:"pending"`
	Error   string `json:"error,omitempty"`
}

func (s *Store) PersistStatus() PersistStatus {
	s.persistMu.Lock()
	b, saved, err := s.backend, s.saved, s.lastErr
	s.persistMu.Unlock()
	s.mu.RLock()
	v := s.version
	s.mu.RUnlock()
	st := PersistStatus{Enabled: b != nil, Pending: b != nil && v != saved}
	if b != nil {
		st.Kind, st.Where, st.Dir = b.Kind(), b.Where(), b.Where()
	}
	if err != nil {
		st.Error = err.Error()
	}
	return st
}

func (s *Store) Flush() error {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	if s.backend == nil {
		return nil
	}
	var buf bytes.Buffer
	s.mu.RLock()
	v := s.version
	if v == s.saved {
		s.mu.RUnlock()
		return nil
	}
	err := gob.NewEncoder(&buf).Encode(snapshot{Format: snapshotFormat, SavedAt: time.Now(), Data: s.d})
	s.mu.RUnlock()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err = s.backend.Save(ctx, buf.Bytes())
		cancel()
	}
	s.lastErr = err
	if err != nil {
		return err
	}
	s.saved = v
	return nil
}

func WriteFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, path+".bak"); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

func (s *Store) Run(ctx context.Context, every time.Duration, extra ...func() error) {
	tk := time.NewTicker(every)
	defer tk.Stop()
	flush := func() {
		if err := s.Flush(); err != nil {
			slog.Error("state snapshot failed", "err", err)
		}
		for _, f := range extra {
			if err := f(); err != nil {
				slog.Error("snapshot failed", "err", err)
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-tk.C:
			flush()
		}
	}
}
