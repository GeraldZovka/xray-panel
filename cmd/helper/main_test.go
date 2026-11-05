package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicKeepsConfigPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := writeAtomic(path, []byte(`{"test":true}`), 0600, os.Geteuid(), os.Getegid()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("config mode = %04o, want 0600", got)
	}
}
