package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
)

// Cache holds persistent state stored next to the executable binary.
type Cache struct {
	OAuthToken *oauth2.Token `json:"oauth_token,omitempty"`
	Trashed    Trashed       `json:"trashed"`
}

// Trashed records criteria that have been used to trash emails.
type Trashed struct {
	Emails []string `json:"emails,omitempty"`
}

// HasEmail reports whether addr has been trashed (case-insensitive).
func (t *Trashed) HasEmail(addr string) bool {
	for _, e := range t.Emails {
		if strings.EqualFold(e, addr) {
			return true
		}
	}
	return false
}

// AddEmail adds addr to the trashed list if not already present.
func (t *Trashed) AddEmail(addr string) {
	if !t.HasEmail(addr) {
		t.Emails = append(t.Emails, strings.ToLower(addr))
	}
}

// ExcludeSet returns a set of lowercased trashed email addresses
// suitable for passing to the analyzer.
func (t *Trashed) ExcludeSet() map[string]struct{} {
	if len(t.Emails) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(t.Emails))
	for _, e := range t.Emails {
		set[strings.ToLower(e)] = struct{}{}
	}
	return set
}

func cachePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), "cache.json"), nil
}

// Load reads cache.json from next to the executable.
// Returns an empty Cache if the file does not exist.
func Load() (*Cache, error) {
	path, err := cachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Cache{}, nil
		}
		return nil, fmt.Errorf("read cache: %w", err)
	}

	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("decode cache: %w", err)
	}
	return &c, nil
}

// Save writes the cache to cache.json next to the executable.
func (c *Cache) Save() error {
	path, err := cachePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cache: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}
	return nil
}
