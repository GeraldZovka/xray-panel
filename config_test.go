package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func testState(t *testing.T) appState {
	t.Helper()
	s, err := makeDefaultState("proxy.example.net", 8080, 8080, 20151, 443, "bcrypt-hash-placeholder")
	if err != nil {
		t.Fatal(err)
	}
	uuid, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	s.Users = append(s.Users, User{ID: "test-user", Name: "Laptop", UUID: uuid, Enabled: true})
	return s
}

func TestBuildTCPConfigAndVLESSLink(t *testing.T) {
	s := testState(t)
	cfg, err := buildXrayConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(cfg, &got); err != nil {
		t.Fatal(err)
	}
	inbound := got["inbounds"].([]any)[0].(map[string]any)
	if inbound["port"].(float64) != 443 {
		t.Fatalf("internal port = %v", inbound["port"])
	}
	stream := inbound["streamSettings"].(map[string]any)
	if stream["network"] != "tcp" {
		t.Fatalf("network = %v", stream["network"])
	}
	clients := inbound["settings"].(map[string]any)["clients"].([]any)
	if clients[0].(map[string]any)["flow"] != "xtls-rprx-vision" {
		t.Fatal("Vision flow missing")
	}
	link := makeVLESSLink(s, s.Users[0])
	if !strings.Contains(link, "@proxy.example.net:20151?") || !strings.Contains(link, "flow=xtls-rprx-vision") || !strings.Contains(link, "pbk="+s.RealityPublicKey) {
		t.Fatalf("unexpected link: %s", link)
	}
	if strings.Contains(link, s.RealityPrivateKey) {
		t.Fatal("link exposes private key")
	}
	routing := got["routing"].(map[string]any)["rules"].([]any)[0].(map[string]any)
	if routing["network"] != "udp" || routing["outboundTag"] != "block" {
		t.Fatal("UDP block rule missing")
	}
}

func TestBuildXHTTPConfig(t *testing.T) {
	s := testState(t)
	s.Settings.Mode = "xhttp"
	cfg, err := buildXrayConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(cfg, &got); err != nil {
		t.Fatal(err)
	}
	stream := got["inbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)
	if stream["network"] != "xhttp" {
		t.Fatalf("network = %v", stream["network"])
	}
	if stream["xhttpSettings"].(map[string]any)["path"] != s.Settings.XHTTPPath {
		t.Fatal("XHTTP path missing")
	}
	if _, exists := stream["realitySettings"]; !exists {
		t.Fatal("REALITY settings missing")
	}
	link := makeVLESSLink(s, s.Users[0])
	if !strings.Contains(link, "type=xhttp") || !strings.Contains(link, "mode=auto") {
		t.Fatalf("unexpected XHTTP link: %s", link)
	}
}

func TestValidateSettingsRejectsUnsafeValues(t *testing.T) {
	s := testState(t)
	bad := s.Settings
	bad.InternalPort = 0
	if validateSettings(bad) == nil {
		t.Fatal("accepted port zero")
	}
	bad = s.Settings
	bad.PublicHost = "example.net/path"
	if validateSettings(bad) == nil {
		t.Fatal("accepted path in host")
	}
	bad = s.Settings
	bad.Dest = "example.net:abc"
	if validateSettings(bad) == nil {
		t.Fatal("accepted nonnumeric destination port")
	}
	bad = s.Settings
	bad.Mode = "udp"
	if validateSettings(bad) == nil {
		t.Fatal("accepted unsupported mode")
	}
	bad = s.Settings
	bad.Mode = "xhttp"
	bad.XHTTPPath = "/x?param=1"
	if validateSettings(bad) == nil {
		t.Fatal("accepted unsafe XHTTP path")
	}
}
