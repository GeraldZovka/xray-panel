package main

import (
	"encoding/json"
	"strconv"
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

func TestNATMappingsGenerateOneLinkPerEndpoint(t *testing.T) {
	s := testState(t)
	s.Settings.NATMappings = []string{
		"vectis-nat-1-20151-ybh.vps-eu1.sumopod.my.id:20151",
		"vectis-nat-1-20152-zxf.vps-eu1.sumopod.my.id:20152",
	}
	if err := validateSettings(s.Settings); err != nil {
		t.Fatal(err)
	}
	links := makeVLESSLinks(s, s.Users[0])
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}
	for i, address := range s.Settings.NATMappings {
		if links[i].Address != address || !strings.Contains(links[i].Link, "@"+address+"?") {
			t.Fatalf("link %d does not use endpoint %q: %+v", i, address, links[i])
		}
	}
	if s.Settings.InternalPort != 443 {
		t.Fatalf("shared internal target port = %d, want 443", s.Settings.InternalPort)
	}
}

func TestNATMappingsLimitAndLegacyCompatibility(t *testing.T) {
	s := testState(t)
	s.Settings.NATMappings = nil // Existing schema-1 states only store publicHost/publicPort.
	if err := validateSettings(s.Settings); err != nil {
		t.Fatalf("legacy single-endpoint state rejected: %v", err)
	}
	for i := 0; i < 8; i++ {
		s.Settings.NATMappings = append(s.Settings.NATMappings, "nat-"+strconv.Itoa(i)+".example.net:"+strconv.Itoa(20151+i))
	}
	if err := validateSettings(s.Settings); err != nil {
		t.Fatalf("eight Xray mappings plus one panel mapping rejected: %v", err)
	}
	s.Settings.NATMappings = append(s.Settings.NATMappings, "nat-9.example.net:20159")
	if err := validateSettings(s.Settings); err == nil {
		t.Fatal("accepted nine Xray mappings plus the panel mapping (10 total)")
	}
	s.Settings.NATMappings = []string{"nat.example.net:20151", "nat.example.net:20151"}
	if err := validateSettings(s.Settings); err == nil {
		t.Fatal("accepted duplicate NAT mapping")
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
	bad.NATMappings = []string{"example.net/path:20151"}
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
