package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// findGoModRoot walks up from dir looking for go.mod.
// Returns the absolute path of the directory containing go.mod.
func findGoModRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", dir)
		}
		dir = parent
	}
}
