// Package uploadcache provides an image upload cache to avoid re-uploading
// images that have already been sent to WeChat.
//
// The cache is stored as images.yaml in the same directory as the markdown
// or HTML file. Each entry maps an md5sum key to an Entry containing the
// original filename and the WeChat URL returned by the upload API.
//
// Example images.yaml:
//
//	86d66b4cb6fd2a2a49afc0addbdf1b89:
//	  filename: photo.png
//	  url: "https://mmbiz.qpic.cn/abc"
//	a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d:
//	  filename: logo.jpg
//	  url: "https://mmbiz.qpic.cn/def"
package uploadcache

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const cacheFile = "images.yaml"

// Entry stores the metadata for a cached image upload.
type Entry struct {
	Filename string `yaml:"filename"`
	URL      string `yaml:"url"`
}

// Cache manages image upload state via YAML on disk.
type Cache struct {
	dir   string
	path  string
	data  map[string]Entry
	dirty bool
}

// Load reads the cache from <dir>/images.yaml.
//
// If the file does not exist or cannot be parsed, an empty cache is returned
// (no error). This lets the caller simply check-and-use without extra error
// handling for first-run or corrupt files.
func Load(dir string) *Cache {
	c := &Cache{
		dir:  dir,
		path: filepath.Join(dir, cacheFile),
		data: make(map[string]Entry),
	}
	raw, err := os.ReadFile(c.path)
	if err != nil {
		return c // not found or unreadable → empty cache
	}
	if err := yaml.Unmarshal(raw, &c.data); err != nil {
		return c // corrupt file → start fresh
	}
	return c
}

// Save persists the cache to images.yaml if it has been modified since load.
func (c *Cache) Save() error {
	if !c.dirty {
		return nil
	}
	raw, err := yaml.Marshal(&c.data)
	if err != nil {
		return fmt.Errorf("marshalling upload cache: %w", err)
	}
	if err := os.WriteFile(c.path, raw, 0644); err != nil {
		return fmt.Errorf("writing %q: %w", c.path, err)
	}
	c.dirty = false
	return nil
}

// Key returns the cache key (md5 hex checksum) for a file on disk.
//
// Unlike the old filename@md5sum scheme, the key is now just the md5sum so
// that renaming a file still hits the cache as long as the content is the same.
func Key(absPath string) (string, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("reading %q for checksum: %w", absPath, err)
	}
	sum := fmt.Sprintf("%x", md5.Sum(data))
	return sum, nil
}

// Get returns the cached URL for a key and whether it was found.
func (c *Cache) Get(key string) (string, bool) {
	entry, ok := c.data[key]
	if !ok {
		return "", false
	}
	return entry.URL, true
}

// Set records a cache entry and marks the cache as dirty (needs saving).
func (c *Cache) Set(key, filename, url string) {
	entry := Entry{Filename: filename, URL: url}
	if existing, ok := c.data[key]; ok && existing == entry {
		return // no change
	}
	c.data[key] = entry
	c.dirty = true
}

// Path returns the full path to the cache file.
func (c *Cache) Path() string {
	return c.path
}
