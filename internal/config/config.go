package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Config struct {
	RepoPath      string `json:"repo_path"`
	RepoConfirmed bool   `json:"repo_confirmed"`
}

type Store struct {
	Path string
}

func DefaultStore() *Store {
	return &Store{Path: filepath.Join(defaultDir(), "config.json")}
}

func (s *Store) Load() (Config, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", s.Path, err)
	}
	return c, nil
}

func (s *Store) Save(c Config) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func IsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

func defaultDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "chezmoui")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "chezmoui")
	}
	return filepath.Join(home, ".config", "chezmoui")
}
