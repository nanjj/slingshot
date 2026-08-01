package base

import (
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// seedProjects creates a store with two indexed projects.
func seedProjects(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "code.db"))
	assert.NilError(t, err)
	t.Cleanup(func() { store.Close() })

	_, err = store.UpsertProject("dscli", "/home/me/src/github.com/dscli/dscli")
	assert.NilError(t, err)
	err = store.SetProjectStatus("dscli", "ready")
	assert.NilError(t, err)

	_, err = store.UpsertProject("slingshot", "/home/me/src/github.com/nanjj/slingshot")
	assert.NilError(t, err)
	err = store.SetProjectStatus("slingshot", "ready")
	assert.NilError(t, err)

	return store
}

func TestResolveProject_ExactName(t *testing.T) {
	store := seedProjects(t)
	info, err := store.ResolveProject("dscli")
	assert.NilError(t, err)
	assert.Equal(t, info.Name, "dscli")
}

func TestResolveProject_FullPath(t *testing.T) {
	store := seedProjects(t)
	info, err := store.ResolveProject("/home/me/src/github.com/dscli/dscli")
	assert.NilError(t, err)
	assert.Equal(t, info.Name, "dscli")
}

func TestResolveProject_TrailingSlashPath(t *testing.T) {
	store := seedProjects(t)
	info, err := store.ResolveProject("/home/me/src/github.com/dscli/dscli/")
	assert.NilError(t, err)
	assert.Equal(t, info.Name, "dscli")
}

func TestResolveProject_Basename(t *testing.T) {
	store := seedProjects(t)
	// "slingshot" is both a name and the basename of the second root.
	info, err := store.ResolveProject("slingshot")
	assert.NilError(t, err)
	assert.Equal(t, info.Name, "slingshot")
}

func TestResolveProject_PathSuffix(t *testing.T) {
	store := seedProjects(t)
	info, err := store.ResolveProject("github.com/nanjj/slingshot")
	assert.NilError(t, err)
	assert.Equal(t, info.Name, "slingshot")
}

func TestResolveProject_CaseInsensitiveName(t *testing.T) {
	store := seedProjects(t)
	info, err := store.ResolveProject("DSCLI")
	assert.NilError(t, err)
	assert.Equal(t, info.Name, "dscli")
}

func TestResolveProject_UniqueSubstring(t *testing.T) {
	store := seedProjects(t)
	// "dscl" matches only dscli (slingshot does not contain it).
	info, err := store.ResolveProject("dscl")
	assert.NilError(t, err)
	assert.Equal(t, info.Name, "dscli")
}

func TestResolveProject_NotFound(t *testing.T) {
	store := seedProjects(t)
	_, err := store.ResolveProject("nonexistent")
	assert.ErrorContains(t, err, "not found")
}

func TestResolveProject_Ambiguous(t *testing.T) {
	store := seedProjects(t)
	// "git" is a substring of both roots.
	_, err := store.ResolveProject("git")
	assert.ErrorContains(t, err, "ambiguous")
}

func TestResolveProject_EmptyStore(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "code.db"))
	assert.NilError(t, err)
	t.Cleanup(func() { store.Close() })

	_, err = store.ResolveProject("anything")
	assert.ErrorContains(t, err, "no projects indexed")
}

func TestResolveProject_EmptyIdentifier(t *testing.T) {
	store := seedProjects(t)
	_, err := store.ResolveProject("  ")
	assert.ErrorContains(t, err, "project is required")
}
