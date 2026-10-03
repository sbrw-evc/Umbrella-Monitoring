package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const storageFile = "storage.json"

type StorageConfig struct {
	Kind        string   `json:"kind"`
	Postgres    PGConfig `json:"postgres"`
	PasswordRef string   `json:"password_ref,omitempty"`
}

func ReadStorageConfig(dir string) (StorageConfig, error) {
	c := StorageConfig{Kind: "file"}
	if dir == "" {
		return c, nil
	}
	b, err := os.ReadFile(filepath.Join(dir, storageFile))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Kind == "" {
		c.Kind = "file"
	}
	return c, nil
}

func WriteStorageConfig(dir string, c StorageConfig) error {
	if dir == "" {
		return errors.New("каталог данных не задан (UMBRELLA_DATA_DIR): настройки хранилища негде сохранить")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(dir, storageFile), b)
}

func (c StorageConfig) Open(ctx context.Context, dir string, resolve func(string) (string, error)) (Backend, error) {
	if c.Kind != "postgres" {
		return FileBackend{Dir: dir}, nil
	}
	pg := c.Postgres
	if c.PasswordRef != "" {
		v, err := resolve(c.PasswordRef)
		if err != nil {
			return nil, err
		}
		pg.Password = v
	}
	return OpenPostgres(ctx, pg)
}
