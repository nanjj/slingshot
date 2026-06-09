package uploadcache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	c := Load(dir)
	if c == nil {
		t.Fatal("Load() returned nil")
	}
	if c.Path() != filepath.Join(dir, "images.yaml") {
		t.Errorf("unexpected path: %q", c.Path())
	}
}

func TestLoadAndSave(t *testing.T) {
	dir := t.TempDir()
	c := Load(dir)
	c.Set("abc123", "a.png", "http://url1")
	c.Set("def456", "b.png", "http://url2")

	if err := c.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Reload from the same directory
	c2 := Load(dir)
	if url, ok := c2.Get("abc123"); !ok || url != "http://url1" {
		t.Errorf("expected http://url1, got %q, ok=%v", url, ok)
	}
	if url, ok := c2.Get("def456"); !ok || url != "http://url2" {
		t.Errorf("expected http://url2, got %q, ok=%v", url, ok)
	}
}

func TestGetMissing(t *testing.T) {
	c := Load(t.TempDir())
	if _, ok := c.Get("nonexistent"); ok {
		t.Error("expected ok=false for missing key")
	}
}

func TestSetNoop(t *testing.T) {
	c := Load(t.TempDir())
	c.Set("k1", "a.png", "url")
	c.dirty = false // reset
	c.Set("k1", "a.png", "url")
	if c.dirty {
		t.Error("dirty should be false when setting same value")
	}
}

func TestSetMarksDirty(t *testing.T) {
	c := Load(t.TempDir())
	c.Set("k1", "a.png", "url1")
	if !c.dirty {
		t.Error("dirty should be true after first set")
	}
	// Change URL
	c.dirty = false
	c.Set("k1", "a.png", "url2")
	if !c.dirty {
		t.Error("dirty should be true after URL change")
	}
}

func TestSaveNotDirty(t *testing.T) {
	dir := t.TempDir()
	c := Load(dir)
	if err := c.Save(); err != nil {
		t.Fatalf("Save() on clean cache should not error: %v", err)
	}
	// File should not exist
	if _, err := os.Stat(filepath.Join(dir, "images.yaml")); !os.IsNotExist(err) {
		t.Error("images.yaml should not be created when not dirty")
	}
}

func TestKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	key, err := Key(path)
	if err != nil {
		t.Fatalf("Key() error = %v", err)
	}

	// Key should be just md5sum, no "test.png@" prefix
	if strings.Contains(key, "@") {
		t.Errorf("key should not contain '@', got %q", key)
	}
	if strings.HasPrefix(key, "test.png") {
		t.Errorf("key should not have filename prefix, got %q", key)
	}
	// md5 of "hello world" is 5eb63bbbe01eeed093cb22bb8f5acdc3
	expectedMD5 := "5eb63bbbe01eeed093cb22bb8f5acdc3"
	if key != expectedMD5 {
		t.Errorf("expected %q, got %q", expectedMD5, key)
	}
}

func TestKeySameContentDifferentName(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "a.png")
	path2 := filepath.Join(dir, "b.png")
	content := []byte("same content")
	if err := os.WriteFile(path1, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path2, content, 0644); err != nil {
		t.Fatal(err)
	}

	key1, err := Key(path1)
	if err != nil {
		t.Fatal(err)
	}
	key2, err := Key(path2)
	if err != nil {
		t.Fatal(err)
	}

	if key1 != key2 {
		t.Errorf("keys should be equal when content is the same, got %q vs %q", key1, key2)
	}
}

func TestKeyDifferentContentSameName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "img.png")

	if err := os.WriteFile(path, []byte("content v1"), 0644); err != nil {
		t.Fatal(err)
	}
	key1, err := Key(path)
	if err != nil {
		t.Fatal(err)
	}

	// Overwrite with new content
	if err := os.WriteFile(path, []byte("content v2"), 0644); err != nil {
		t.Fatal(err)
	}
	key2, err := Key(path)
	if err != nil {
		t.Fatal(err)
	}

	if key1 == key2 {
		t.Error("keys should differ when content changes")
	}
}

func TestLoadCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "images.yaml")
	if err := os.WriteFile(path, []byte("not: valid: yaml: [[["), 0644); err != nil {
		t.Fatal(err)
	}
	c := Load(dir)
	if c == nil {
		t.Fatal("Load() should return non-nil even for corrupt files")
	}
	// Should have empty data, not crashed
	if len(c.data) != 0 {
		t.Errorf("expected empty data for corrupt file, got %d entries", len(c.data))
	}
}

func TestKeyFileNotFound(t *testing.T) {
	_, err := Key("/nonexistent/file.png")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRoundTripViaDisk(t *testing.T) {
	dir := t.TempDir()
	c := Load(dir)

	// Set some values
	c.Set("abc123", "img1.png", "http://url1")
	c.Set("def456", "img2.jpg", "http://url2")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// Read raw file to verify YAML structure
	raw, err := os.ReadFile(filepath.Join(dir, "images.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)

	// Should contain md5sum keys and nested filename/url fields
	if !strings.Contains(s, "abc123") {
		t.Errorf("expected abc123 in YAML, got:\n%s", s)
	}
	if !strings.Contains(s, "def456") {
		t.Errorf("expected def456 in YAML, got:\n%s", s)
	}
	if !strings.Contains(s, "filename: img1.png") {
		t.Errorf("expected 'filename: img1.png' in YAML, got:\n%s", s)
	}
	if !strings.Contains(s, "url: http://url1") {
		t.Errorf("expected 'url: http://url1' in YAML, got:\n%s", s)
	}
	// Verify it's nested format, not flat "key: url"
	if strings.Contains(s, "abc123: http://") {
		t.Errorf("YAML should use nested entry format, not flat string, got:\n%s", s)
	}
}

func TestSetWithFilename(t *testing.T) {
	c := Load(t.TempDir())
	c.Set("md5key", "renamed.png", "http://url")

	// Same md5, different filename — should update filename and mark dirty
	c.dirty = false
	c.Set("md5key", "newname.png", "http://url")
	if !c.dirty {
		t.Error("dirty should be true when filename changes")
	}

	// Verify stored entry has new filename
	url, ok := c.Get("md5key")
	if !ok || url != "http://url" {
		t.Errorf("expected http://url, got %q, ok=%v", url, ok)
	}
}

func TestEntryComparison(t *testing.T) {
	// Verify Entry struct supports == comparison (all fields are comparable)
	e1 := Entry{Filename: "a.png", URL: "http://u1"}
	e2 := Entry{Filename: "a.png", URL: "http://u1"}
	if e1 != e2 {
		t.Error("identical entries should be equal")
	}
	e3 := Entry{Filename: "b.png", URL: "http://u1"}
	if e1 == e3 {
		t.Error("different filename entries should not be equal")
	}
}

// --- MediaID tests ---

func TestGetMediaIDMissing(t *testing.T) {
	c := Load(t.TempDir())
	if _, ok := c.GetMediaID("nonexistent"); ok {
		t.Error("expected ok=false for missing media_id key")
	}
}

func TestSetMediaID(t *testing.T) {
	c := Load(t.TempDir())
	c.SetMediaID("key1", "cover.jpg", "media_abc123")

	mediaID, ok := c.GetMediaID("key1")
	if !ok {
		t.Fatal("expected ok=true after SetMediaID")
	}
	if mediaID != "media_abc123" {
		t.Errorf("expected media_abc123, got %q", mediaID)
	}
}

func TestSetMediaIDMarksDirty(t *testing.T) {
	c := Load(t.TempDir())
	c.SetMediaID("k1", "cover.jpg", "mid1")
	if !c.dirty {
		t.Error("dirty should be true after SetMediaID")
	}
	// Change mediaID
	c.dirty = false
	c.SetMediaID("k1", "cover.jpg", "mid2")
	if !c.dirty {
		t.Error("dirty should be true after mediaID change")
	}
}

func TestSetMediaIDNoop(t *testing.T) {
	c := Load(t.TempDir())
	c.SetMediaID("k1", "cover.jpg", "mid1")
	c.dirty = false
	c.SetMediaID("k1", "cover.jpg", "mid1")
	if c.dirty {
		t.Error("dirty should be false when setting same media_id value")
	}
}

func TestSetMediaIDPreservesURL(t *testing.T) {
	c := Load(t.TempDir())
	// First set URL
	c.Set("key1", "img.png", "http://url1")
	// Then set media_id — should preserve existing URL
	c.SetMediaID("key1", "img.png", "media_xyz")

	// Verify URL is still accessible
	url, ok := c.Get("key1")
	if !ok || url != "http://url1" {
		t.Errorf("expected URL http://url1 to be preserved, got %q, ok=%v", url, ok)
	}
	// Verify media_id is also set
	mediaID, ok := c.GetMediaID("key1")
	if !ok || mediaID != "media_xyz" {
		t.Errorf("expected media_id 'media_xyz', got %q, ok=%v", mediaID, ok)
	}
}

func TestSetPreservesMediaID(t *testing.T) {
	c := Load(t.TempDir())
	// First set media_id
	c.SetMediaID("key1", "img.png", "media_xyz")
	// Then update URL — should preserve existing media_id
	c.Set("key1", "img.png", "http://url1")

	// Verify media_id is still accessible
	mediaID, ok := c.GetMediaID("key1")
	if !ok || mediaID != "media_xyz" {
		t.Errorf("expected media_id 'media_xyz' to be preserved, got %q, ok=%v", mediaID, ok)
	}
	// Verify URL is also set
	url, ok := c.Get("key1")
	if !ok || url != "http://url1" {
		t.Errorf("expected URL http://url1, got %q, ok=%v", url, ok)
	}
}

func TestMediaIDRoundTripViaDisk(t *testing.T) {
	dir := t.TempDir()
	c := Load(dir)

	c.SetMediaID("thumb1", "cover.jpg", "media_abc")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// Read raw file to verify YAML contains media_id field
	raw, err := os.ReadFile(filepath.Join(dir, "images.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)

	if !strings.Contains(s, "media_id: media_abc") {
		t.Errorf("expected 'media_id: media_abc' in YAML, got:\n%s", s)
	}

	// Reload and verify
	c2 := Load(dir)
	mediaID, ok := c2.GetMediaID("thumb1")
	if !ok || mediaID != "media_abc" {
		t.Errorf("expected media_abc after reload, got %q, ok=%v", mediaID, ok)
	}
}
