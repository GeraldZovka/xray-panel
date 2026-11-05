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
	vmessID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	st.Inbounds = []ManagedInbound{{ID: "vmess-proto-1", Protocol: "vmess", InternalPort: st.Settings.InternalPort + 1}}
	st.Users[0].ProtocolCredentials = map[string]string{"vmess-proto-1": vmessID}
	if err := validateStoredState(st); err != nil {
		t.Fatal(err)
	}
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
	loaded, err := loadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Inbounds) != 1 || loaded.Inbounds[0].Protocol != "vmess" || loaded.Users[0].ProtocolCredentials["vmess-proto-1"] != vmessID {
		t.Fatal("managed inbound and per-user credential did not survive state persistence")
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
