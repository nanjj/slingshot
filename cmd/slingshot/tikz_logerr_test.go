package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTexLogErr pins the diagnostics that a failed TeX run produces. When the
// engine fails, the only useful output is the set of "!" lines in input.log;
// if this extraction regresses, every compile error degrades to "exit status 1".
func TestTexLogErr(t *testing.T) {
	baseErr := errors.New("latexmk: exit status 12")

	t.Run("nil error stays nil", func(t *testing.T) {
		if err := texLogErr(nil, t.TempDir()); err != nil {
			t.Fatalf("texLogErr(nil) = %v, want nil", err)
		}
	})

	t.Run("no log file returns the original error", func(t *testing.T) {
		if err := texLogErr(baseErr, t.TempDir()); !errors.Is(err, baseErr) {
			t.Fatalf("texLogErr() = %v, want the original error", err)
		}
	})

	t.Run("empty log returns the original error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "input.log"), nil, 0644); err != nil {
			t.Fatal(err)
		}
		if err := texLogErr(baseErr, dir); !errors.Is(err, baseErr) {
			t.Fatalf("texLogErr() = %v, want the original error", err)
		}
	})

	t.Run("log without bang lines returns the original error", func(t *testing.T) {
		dir := t.TempDir()
		log := "This is XeTeX\n(./input.tex\n)\nNo pages of output.\n"
		if err := os.WriteFile(filepath.Join(dir, "input.log"), []byte(log), 0644); err != nil {
			t.Fatal(err)
		}
		if err := texLogErr(baseErr, dir); !errors.Is(err, baseErr) {
			t.Fatalf("texLogErr() = %v, want the original error", err)
		}
	})

	t.Run("bang lines are appended and wrapped", func(t *testing.T) {
		dir := t.TempDir()
		log := "noise\n! Undefined control sequence.\nmore noise\n! Emergency stop.\n"
		if err := os.WriteFile(filepath.Join(dir, "input.log"), []byte(log), 0644); err != nil {
			t.Fatal(err)
		}

		err := texLogErr(baseErr, dir)
		if !errors.Is(err, baseErr) {
			t.Fatalf("texLogErr() lost the wrapped error: %v", err)
		}
		for _, want := range []string{
			"! Undefined control sequence.",
			"! Emergency stop.",
			"log errors:",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
		}
		if strings.Contains(err.Error(), "noise") {
			t.Errorf("error %q contains log noise it should have filtered", err)
		}
	})

	t.Run("at most ten bang lines", func(t *testing.T) {
		dir := t.TempDir()
		var b strings.Builder
		for i := 0; i < 25; i++ {
			b.WriteString("! error number ")
			b.WriteByte(byte('a' + i))
			b.WriteString("\n")
		}
		if err := os.WriteFile(filepath.Join(dir, "input.log"), []byte(b.String()), 0644); err != nil {
			t.Fatal(err)
		}

		err := texLogErr(baseErr, dir)
		// Every line, including the first, is preceded by a newline: the
		// first follows "log errors:\n". So ten bang lines means ten
		// occurrences of "\n!".
		if n := strings.Count(err.Error(), "\n!"); n != 10 {
			t.Fatalf("got %d bang lines, want 10 (the cap): %v", n, err)
		}
	})
}
