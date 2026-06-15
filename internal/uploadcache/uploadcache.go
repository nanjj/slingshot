// Package uploadcache provides an image upload cache to avoid re-uploading
// images that have already been sent to WeChat.
//
// The local cache is stored as images.yaml in the same directory as the markdown
// or HTML file. A global cache is also stored at ~/.dscli/images.yaml so that
// the same file uploaded from different source directories is not re-uploaded.
//
// Each entry maps an md5sum key to an Entry containing the original filename,
// the WeChat URL (from media/uploadimg), and optionally the media_id (from
// material/add_material for thumbnail images).
//
// Lookup order: local by md5 key → global by md5 key → local by filename → global by filename.
//
// Example images.yaml:
//
//	86d66b4cb6fd2a2a49afc0addbdf1b89:
//	  filename: photo.png
//	  url: "https://mmbiz.qpic.cn/abc"
//	  media_id: "xxx"
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

var globalCacheDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dscli")
}

// Entry stores the metadata for a cached image upload.
// Both URL and MediaID are optional — which one is set depends on
// whether the image was uploaded for content use (media/uploadimg → URL)
// or as a thumbnail (material/add_material → media_id).
type Entry struct {
	Filename string `yaml:"filename"`
	URL      string `yaml:"url,omitempty"`
	MediaID  string `yaml:"media_id,omitempty"`
}

// Cache manages image upload state via YAML on disk.
type Cache struct {
	dir      string
	path     string
	data     map[string]Entry
	dirty    bool
	fallback *Cache // read-only global cache fallback
}

func globalCachePath() string {
	d := globalCacheDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, cacheFile)
}

// LoadCache loads both local and global caches. The local cache is at
// <dir>/images.yaml. The global cache is at ~/.dscli/images.yaml and serves
// as a read-only fallback — entries written to the local cache are also
// written to the global cache.
//
// If the local file does not exist or cannot be parsed, an empty local cache
// is returned (no error). The global cache is silently skipped if missing.
func LoadCache(dir string) *Cache {
	c := &Cache{
		dir:  dir,
		path: filepath.Join(dir, cacheFile),
		data: make(map[string]Entry),
	}
	// Load local cache
	raw, err := os.ReadFile(c.path)
	if err == nil {
		if err := yaml.Unmarshal(raw, &c.data); err != nil {
			// corrupt file → start fresh
			c.data = make(map[string]Entry)
		}
	}
	// Load global cache as read-only fallback
	if gp := globalCachePath(); gp != "" && gp != c.path {
		gc := &Cache{
			dir:  filepath.Dir(gp),
			path: gp,
			data: make(map[string]Entry),
		}
		raw, err := os.ReadFile(gp)
		if err == nil {
			if err := yaml.Unmarshal(raw, &gc.data); err != nil {
				gc.data = make(map[string]Entry)
			}
		}
		if len(gc.data) > 0 {
			c.fallback = gc
		}
	}
	return c
}

// Load is a compatibility wrapper — calls LoadCache.
// Deprecated: use LoadCache which also loads the global cache.
func Load(dir string) *Cache {
	return LoadCache(dir)
}

// findEntry looks up a key in the cache, checking local first then fallback.
func (c *Cache) findEntry(key string) (Entry, bool) {
	if entry, ok := c.data[key]; ok {
		return entry, true
	}
	if c.fallback != nil {
		if entry, ok := c.fallback.data[key]; ok {
			return entry, true
		}
	}
	return Entry{}, false
}

// Get returns the cached URL for a key and whether it was found.
// Checks local cache first, then global fallback.
func (c *Cache) Get(key string) (string, bool) {
	entry, ok := c.findEntry(key)
	if !ok {
		return "", false
	}
	return entry.URL, true
}

// GetMediaID returns the cached media_id for a key and whether it was found.
// Checks local cache first, then global fallback.
func (c *Cache) GetMediaID(key string) (string, bool) {
	entry, ok := c.findEntry(key)
	if !ok {
		return "", false
	}
	return entry.MediaID, true
}

// GetByFilename looks up a cache entry by filename across local and global caches.
// Returns the first matching entry. This enables dedup by filename: if the same
// filename was already uploaded (even from a different directory), we can skip
// the upload and reuse the existing URL/media_id.
func (c *Cache) GetByFilename(filename string) (Entry, bool) {
	for _, entry := range c.data {
		if entry.Filename == filename {
			return entry, true
		}
	}
	if c.fallback != nil {
		for _, entry := range c.fallback.data {
			if entry.Filename == filename {
				return entry, true
			}
		}
	}
	return Entry{}, false
}

// HasFilename reports whether any cache entry (local or global) has the given filename.
func (c *Cache) HasFilename(filename string) bool {
	_, ok := c.GetByFilename(filename)
	return ok
}

// Save persists the local cache to images.yaml if it has been modified since load.
// Also writes to the global cache so future runs from other directories can reuse.
func (c *Cache) Save() error {
	if !c.dirty {
		return nil
	}
	// Save local cache
	raw, err := yaml.Marshal(&c.data)
	if err != nil {
		return fmt.Errorf("marshalling upload cache: %w", err)
	}
	if err := os.WriteFile(c.path, raw, 0644); err != nil {
		return fmt.Errorf("writing %q: %w", c.path, err)
	}
	// Also save to global cache for cross-directory sharing
	if gp := globalCachePath(); gp != "" && gp != c.path {
		// Read existing global cache
		gc := make(map[string]Entry)
		if existing, err := os.ReadFile(gp); err == nil {
			_ = yaml.Unmarshal(existing, &gc)
		}
		// Merge local entries into global
		changed := false
		for k, v := range c.data {
			if existing, ok := gc[k]; !ok || existing != v {
				gc[k] = v
				changed = true
			}
		}
		if changed {
			dir := filepath.Dir(gp)
			if err := os.MkdirAll(dir, 0755); err == nil {
				raw, err := yaml.Marshal(&gc)
				if err == nil {
					_ = os.WriteFile(gp, raw, 0644)
				}
			}
		}
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

// Set records a cache entry (with URL) and marks the cache as dirty.
func (c *Cache) Set(key, filename, url string) {
	entry := Entry{Filename: filename, URL: url}
	if existing, ok := c.data[key]; ok && existing == entry {
		return // no change
	}
	// Preserve existing MediaID if present
	if existing, ok := c.data[key]; ok && existing.MediaID != "" {
		entry.MediaID = existing.MediaID
	}
	c.data[key] = entry
	c.dirty = true
}

// SetMediaID records a cache entry with a media_id (for thumbnails) and marks
// the cache as dirty. If a URL was previously cached for the same key, it is
// preserved.
func (c *Cache) SetMediaID(key, filename, mediaID string) {
	entry := Entry{Filename: filename, MediaID: mediaID}
	if existing, ok := c.data[key]; ok && existing == entry {
		return // no change
	}
	// Preserve existing URL if present
	if existing, ok := c.data[key]; ok && existing.URL != "" {
		entry.URL = existing.URL
	}
	c.data[key] = entry
	c.dirty = true
}

// SetEntry records a full cache entry (with both URL and MediaID) and marks
// the cache as dirty. This is used when a thumbnail upload returns both
// a media_id and a URL — we save both so that content images matching the
// same filename can reuse the URL without a separate upload.
func (c *Cache) SetEntry(key, filename, url, mediaID string) {
	entry := Entry{Filename: filename, URL: url, MediaID: mediaID}
	if existing, ok := c.data[key]; ok && existing == entry {
		return // no change
	}
	c.data[key] = entry
	c.dirty = true
}

// Path returns the full path to the local cache file.
func (c *Cache) Path() string {
	return c.path
}

