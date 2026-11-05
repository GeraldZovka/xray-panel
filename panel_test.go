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

func TestAdminPasswordLengthPolicy(t *testing.T) {
	if validAdminPasswordLength([]byte("1234567")) {
		t.Fatal("accepted a password shorter than 8 bytes")
	}
	if !validAdminPasswordLength([]byte("12345678")) {
		t.Fatal("rejected the 8-byte minimum password")
	}
	if !validAdminPasswordLength(make([]byte, 1024)) {
		t.Fatal("rejected the maximum 1024-byte password")
	}
	if validAdminPasswordLength(make([]byte, 1025)) {
		t.Fatal("accepted a password longer than 1024 bytes")
	}
}
