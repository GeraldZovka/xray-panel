package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateIsStoredPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private-state")
	path := filepath.Join(dir, "state.json")
	st := testState(t)
	if err := saveState(path, st); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("state permissions = %04o, want 0600", info.Mode().Perm())
	}
	dinfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dinfo.Mode().Perm() != 0700 {
		t.Fatalf("state directory permissions = %04o, want 0700", dinfo.Mode().Perm())
	}
	if _, err := loadState(path); err != nil {
		t.Fatal(err)
	}
}
