package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	cacheDirName       = "plexcli"
	libraryCacheDir    = "library"
	cacheFileMode      = 0600
	cacheDirectoryMode = 0700
)

// LibraryPayloadCache stores raw library section payloads on disk.
type LibraryPayloadCache struct {
	dir string
	now func() time.Time
}

func NewLibraryPayloadCache(dir string) (*LibraryPayloadCache, error) {
	if dir == "" {
		return nil, fmt.Errorf("cache directory is required")
	}

	if err := os.MkdirAll(dir, cacheDirectoryMode); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &LibraryPayloadCache{
		dir: dir,
		now: time.Now,
	}, nil
}

func NewDefaultLibraryPayloadCache() (*LibraryPayloadCache, error) {
	dir, err := DefaultLibraryPayloadCacheDir()
	if err != nil {
		return nil, err
	}

	return NewLibraryPayloadCache(dir)
}

func DefaultLibraryPayloadCacheDir() (string, error) {
	baseDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve user cache directory: %w", err)
	}

	return filepath.Join(baseDir, cacheDirName, libraryCacheDir), nil
}

// Load returns payload bytes if an unexpired cache file exists.
func (c *LibraryPayloadCache) Load(key string, ttl time.Duration) ([]byte, bool, error) {
	if ttl <= 0 {
		return nil, false, nil
	}

	path := filepath.Join(c.dir, key+".json")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to stat cache file: %w", err)
	}

	if c.now().Sub(info.ModTime()) > ttl {
		return nil, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to read cache file: %w", err)
	}

	return data, true, nil
}

// Save writes payload bytes atomically.
func (c *LibraryPayloadCache) Save(key string, payload []byte) error {
	path := filepath.Join(c.dir, key+".json")
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, payload, cacheFileMode); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to persist cache file: %w", err)
	}

	return nil
}
