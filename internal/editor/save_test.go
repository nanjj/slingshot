package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func openFileDoc(t *testing.T, ed *Editor, uri, source, lang string) {
	t.Helper()
	err := ed.OpenDocument(uri, []byte(source), lang)
	if err != nil {
		t.Fatalf("OpenDocument(%q) failed: %v", uri, err)
	}
}

// ─── Save ───

func TestSaveToNewFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n\nfunc main() {}\n", "go")

	result, err := ed.Save(uri)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Save returned success=false: %+v", result)
	}
	if result.Path != filePath {
		t.Errorf("path: got %q, want %q", result.Path, filePath)
	}
	if result.Bytes <= 0 {
		t.Errorf("bytes written: got %d, want >0", result.Bytes)
	}

	// Verify file content on disk
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "package main\n\nfunc main() {}\n" {
		t.Errorf("file content: got %q, want %q", string(content), "package main\n\nfunc main() {}\n")
	}

	// After save, doc should not be dirty
	dirty, _ := ed.IsDirty(uri)
	if dirty {
		t.Errorf("after save, doc should not be dirty")
	}
}

func TestSaveAndEditAgain(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")
	ed.Save(uri)

	// Edit and save again
	ed.Insert(uri, 0, "// comment\n")
	result, err := ed.Save(uri)
	if err != nil {
		t.Fatalf("Save after edit failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Save after edit returned success=false: %+v", result)
	}

	content, _ := os.ReadFile(filePath)
	if string(content) != "// comment\npackage main\n" {
		t.Errorf("after re-save: got %q, want %q",
			string(content), "// comment\npackage main\n")
	}
}

// ─── SaveAs ───

func TestSaveAs(t *testing.T) {
	dir := t.TempDir()
	origPath := filepath.Join(dir, "original.go")
	newPath := filepath.Join(dir, "newname.go")
	uri := "file://" + origPath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")

	result, err := ed.SaveAs(uri, newPath)
	if err != nil {
		t.Fatalf("SaveAs failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("SaveAs returned success=false: %+v", result)
	}
	if result.Path != newPath {
		t.Errorf("path: got %q, want %q", result.Path, newPath)
	}

	// New file should exist
	content, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("ReadFile new path failed: %v", err)
	}
	if string(content) != "package main\n" {
		t.Errorf("new file content: got %q", string(content))
	}

	// Original file should NOT exist (was never saved)
	if _, err := os.Stat(origPath); err == nil {
		t.Errorf("original file should not exist before explicit Save")
	}

	// Document URI should still be the original
	doc, _ := ed.GetDocument(uri)
	if doc.URI() != uri {
		t.Errorf("document uri changed: got %q, want %q", doc.URI(), uri)
	}
}

// ─── ForceSave ───

func TestForceSave(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")

	result, err := ed.ForceSave(uri)
	if err != nil {
		t.Fatalf("ForceSave failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("ForceSave returned success=false: %+v", result)
	}
}

// ─── Reload ───

func TestReloadFromDisk(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")
	ed.Save(uri)

	// Externally modify the file
	os.WriteFile(filePath, []byte("package main\n\nfunc modified() {}\n"), 0644)

	// Reload
	err := ed.Reload(uri)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// Should have the external content
	doc, _ := ed.GetDocument(uri)
	got := string(doc.Source())
	want := "package main\n\nfunc modified() {}\n"
	if got != want {
		t.Errorf("after reload: got %q, want %q", got, want)
	}

	// Should not be dirty
	if doc.Dirty() {
		t.Errorf("after reload, doc should not be dirty")
	}
}

func TestReloadScratchFails(t *testing.T) {
	ed := NewEditor("")
	openFileDoc(t, ed, "scratch:///test.go", "package main\n", "go")
	err := ed.Reload("scratch:///test.go")
	if err != ErrNonFileURI {
		t.Errorf("expected ErrNonFileURI for scratch reload, got %v", err)
	}
}

func TestReloadNotFound(t *testing.T) {
	ed := NewEditor("")
	err := ed.Reload("nonexistent")
	if err != ErrDocumentNotFound {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// ─── Conflict detection ───

func TestSaveConflictDetection(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")
	ed.Save(uri) // lastSavedMTime = mtime@t1

	// External modification
	os.WriteFile(filePath, []byte("package main\n\n// external change\n"), 0644)

	// Save should detect conflict
	result, err := ed.Save(uri)
	if err != nil {
		t.Fatalf("Save after external change failed: %v", err)
	}
	if result.Success {
		t.Errorf("expected conflict, but Save returned success=true")
	}
	if !result.Conflict {
		t.Errorf("expected Conflict=true, got Conflict=%v", result.Conflict)
	}
}

func TestSaveConflictCanBeOverridden(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")
	ed.Save(uri)

	// External modification
	os.WriteFile(filePath, []byte("// external\n"), 0644)

	// ForceSave should override
	result, err := ed.ForceSave(uri)
	if err != nil {
		t.Fatalf("ForceSave failed: %v", err)
	}
	if !result.Success {
		t.Errorf("ForceSave should succeed, got success=%v", result.Success)
	}

	// File should now contain our content
	content, _ := os.ReadFile(filePath)
	if string(content) != "package main\n" {
		t.Errorf("after ForceSave: got %q, want %q", string(content), "package main\n")
	}
}

func TestSaveNoConflictIfUnchanged(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")
	ed.Save(uri)

	// Save again without any changes (or external modification)
	result, err := ed.Save(uri)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if !result.Success {
		t.Errorf("re-saving unchanged doc should succeed, got success=%v", result.Success)
	}
	if result.Conflict {
		t.Errorf("re-saving unchanged doc should not be a conflict")
	}
}

func TestSaveNonFileURI(t *testing.T) {
	ed := NewEditor("")
	openFileDoc(t, ed, "scratch:///test.go", "package main\n", "go")
	_, err := ed.Save("scratch:///test.go")
	if err != ErrNonFileURI {
		t.Errorf("expected ErrNonFileURI, got %v", err)
	}
}

func TestSaveDocumentNotFound(t *testing.T) {
	ed := NewEditor("")
	_, err := ed.Save("nonexistent")
	if err != ErrDocumentNotFound {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// ─── AutoSave ───

func TestAutoSaveAfterEdit(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")

	// Edit — after this, auto-save should have written to disk
	_, err := ed.Insert(uri, 0, "// comment\n")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	want := "// comment\npackage main\n"
	if string(content) != want {
		t.Errorf("after auto-save: got %q, want %q", string(content), want)
	}

	// Doc should not be dirty
	dirty, _ := ed.IsDirty(uri)
	if dirty {
		t.Errorf("after auto-save, doc should not be dirty")
	}
}

func TestAutoSaveScratchDoc(t *testing.T) {
	ed := NewEditor("")
	openFileDoc(t, ed, "scratch:///test.go", "package main\n", "go")

	ed.Insert("scratch:///test.go", 0, "// comment\n")

	// Should still be dirty (auto-save skips scratch docs)
	dirty, _ := ed.IsDirty("scratch:///test.go")
	if !dirty {
		t.Errorf("scratch doc should be dirty after edit (auto-save skipped)")
	}
}

func TestAutoSaveMultipleEdits(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")

	ed.Insert(uri, 0, "// first\n")
	ed.Insert(uri, 0, "// second\n")

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	want := "// second\n// first\npackage main\n"
	if string(content) != want {
		t.Errorf("after multiple auto-saves: got %q, want %q", string(content), want)
	}

	dirty, _ := ed.IsDirty(uri)
	if dirty {
		t.Errorf("after auto-save, doc should not be dirty")
	}
}

// ─── AutoReload ───

func TestAutoReloadOnRead(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")
	ed.Save(uri)

	// Externally modify the file
	os.WriteFile(filePath, []byte("package main\n\nfunc external() {}\n"), 0644)

	// Read should trigger auto-reload
	text, err := ed.GetText(uri, 0, 33)
	if err != nil {
		t.Fatalf("GetText failed: %v", err)
	}
	want := "package main\n\nfunc external() {}\n"
	if string(text) != want {
		t.Errorf("after auto-reload: got %q, want %q", string(text), want)
	}

	// Doc should not be dirty after reload
	dirty, _ := ed.IsDirty(uri)
	if dirty {
		t.Errorf("after auto-reload, doc should not be dirty")
	}
}

func TestAutoReloadNoChangeIfSame(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")
	ed.Save(uri)

	// Read with no external modification — should work fine
	text, err := ed.GetText(uri, 0, 13)
	if err != nil {
		t.Fatalf("GetText failed: %v", err)
	}
	want := "package main\n"
	if string(text) != want {
		t.Errorf("expected %q, got %q", want, text)
	}
}

func TestAutoReloadScratchDoc(t *testing.T) {
	ed := NewEditor("")
	openFileDoc(t, ed, "scratch:///test.go", "package main\n", "go")

	// Read on scratch doc — should not panic or error
	text, err := ed.GetText("scratch:///test.go", 0, 13)
	if err != nil {
		t.Fatalf("GetText failed: %v", err)
	}
	want := "package main\n"
	if string(text) != want {
		t.Errorf("expected %q, got %q", want, text)
	}
}

func TestAutoReloadAfterInsert(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	openFileDoc(t, ed, uri, "package main\n", "go")

	// Auto-save after insert writes to disk
	ed.Insert(uri, 0, "// added\n")

	// Externally modify
	os.WriteFile(filePath, []byte("package main\n\nfunc external() {}\n"), 0644)

	// Auto-reload on the next read
	text, err := ed.GetText(uri, 0, 33)
	if err != nil {
		t.Fatalf("GetText failed: %v", err)
	}
	want := "package main\n\nfunc external() {}\n"
	if string(text) != want {
		t.Errorf("after auto-reload: got %q, want %q", string(text), want)
	}
}
