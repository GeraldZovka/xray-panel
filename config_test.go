package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func testState(t *testing.T) appState {
	t.Helper()
	s, err := makeDefaultState(8080, 443, "bcrypt-hash-placeholder")
	if err != nil {
		t.Fatal(err)
	}
	s.Settings.PublicHost = "proxy.example.net"
	s.Settings.PublicPort = 20151
	s.Settings.NATMappings = []string{"proxy.example.net:20151"}
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
	rules := got["routing"].(map[string]any)["rules"].([]any)
	dnsRule := rules[0].(map[string]any)
	udpBlockRule := rules[1].(map[string]any)
	if dnsRule["network"] != "udp" || dnsRule["port"] != "53" || dnsRule["outboundTag"] != "direct" {
		t.Fatal("UDP DNS must route direct before the general UDP block")
	}
	if udpBlockRule["network"] != "udp" || udpBlockRule["outboundTag"] != "block" {
		t.Fatal("general UDP block rule missing after DNS exception")
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

func TestPendingPublicPortsBecomeLinksAfterProviderHostnameIsEntered(t *testing.T) {
	s := testState(t)
	s.Settings.PublicHost = ""
	s.Settings.PublicPort = 20151
	s.Settings.NATMappings = []string{"20151", "20152"}
	if err := validateSettings(s.Settings); err != nil {
		t.Fatalf("numeric pending ports rejected: %v", err)
	}
	if links := makeVLESSLinks(s, s.Users[0]); len(links) != 0 {
		t.Fatalf("pending ports unexpectedly produced links: %+v", links)
	}
	if _, err := buildXrayConfig(s); err != nil {
		t.Fatalf("Xray config rejected before provider hostnames are available: %v", err)
	}

	s.Settings.NATMappings = []string{"20151", "nat-two.example.net:20152"}
	if err := validateSettings(s.Settings); err != nil {
		t.Fatalf("provider hostname entry rejected: %v", err)
	}
	links := makeVLESSLinks(s, s.Users[0])
	if len(links) != 1 || links[0].Address != "nat-two.example.net:20152" || !strings.Contains(links[0].Link, "@nat-two.example.net:20152?") {
		t.Fatalf("completed endpoint did not generate exactly its link: %+v", links)
	}

	s.Settings.NATMappings = []string{"20151", "20151"}
	if err := validateSettings(s.Settings); err == nil {
		t.Fatal("accepted duplicate pending public ports")
	}
	s.Settings.NATMappings = []string{"nat-one.example.net:20151", "nat-two.example.net:20151"}
	if err := validateSettings(s.Settings); err != nil {
		t.Fatalf("rejected legacy-compatible distinct hostnames on the same port: %v", err)
	}
	s.Settings.NATMappings = []string{"65536"}
	if err := validateSettings(s.Settings); err == nil {
		t.Fatal("accepted an out-of-range pending public port")
	}
	s.Settings.InternalPort = 20151
	s.Settings.NATMappings = []string{"nat-one.example.net:20151"}
	if err := validateSettings(s.Settings); err != nil {
		t.Fatalf("provider public port matching the internal port was rejected: %v", err)
	}
}
func TestDefaultStateCanWaitForProviderEndpoint(t *testing.T) {
	s, err := makeDefaultState(20151, 443, "bcrypt-hash-placeholder")
	if err != nil {
		t.Fatal(err)
	}
	if s.Settings.PublicHost != "" || s.Settings.PublicPort != 0 || len(s.Settings.NATMappings) != 0 {
		t.Fatalf("default state invented a provider endpoint: %+v", s.Settings)
	}
	if err := validateSettings(s.Settings); err != nil {
		t.Fatalf("empty-endpoint default state is invalid: %v", err)
	}
	if _, err := buildXrayConfig(s); err != nil {
		t.Fatalf("Xray config rejected before provider endpoint is available: %v", err)
	}
	s.Settings.NATMappings = []string{"provider.example.net:20152"}
	if err := validateSettings(s.Settings); err != nil {
		t.Fatalf("provider endpoint entered later in panel settings was rejected: %v", err)
	}
}
func TestDefaultStateRejectsCollidingInternalListeners(t *testing.T) {
	if _, err := makeDefaultState(443, 443, "bcrypt-hash-placeholder"); err == nil {
		t.Fatal("accepted identical panel and Xray internal listener ports")
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

func TestOfficialInboundProtocolChoicesAndConfigGeneration(t *testing.T) {
	want := []string{"tunnel", "http", "shadowsocks", "socks", "trojan", "vless", "vmess", "wireguard", "hysteria", "masque", "tun"}
	if len(supportedInboundProtocols) != len(want) {
		t.Fatalf("protocol choices = %d, want %d", len(supportedInboundProtocols), len(want))
	}
	for i, protocol := range want {
		if supportedInboundProtocols[i] != protocol {
			t.Fatalf("protocol choice %d = %q, want %q", i, supportedInboundProtocols[i], protocol)
		}
		s := testState(t)
		s.Settings.Protocol = protocol
		s.Settings.ProtocolSettings = `{"customMarker":"kept"}`
		s.Settings.ProtocolStreamSettings = `{"network":"tcp"}`
		if err := validateSettings(s.Settings); err != nil {
			t.Fatalf("%s settings rejected: %v", protocol, err)
		}
		cfg, err := buildXrayConfig(s)
		if err != nil {
			t.Fatalf("%s config failed: %v", protocol, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(cfg, &decoded); err != nil {
			t.Fatalf("%s config is not JSON: %v", protocol, err)
		}
		inbound := decoded["inbounds"].([]any)[0].(map[string]any)
		if inbound["protocol"] != protocol {
			t.Fatalf("generated protocol = %v, want %s", inbound["protocol"], protocol)
		}
		if protocol == "tun" {
			if _, ok := inbound["port"]; ok {
				t.Fatal("TUN inbound unexpectedly has a listening port")
			}
			if _, ok := inbound["listen"]; ok {
				t.Fatal("TUN inbound unexpectedly has a listen address")
			}
		} else if inbound["port"].(float64) != float64(s.Settings.InternalPort) || inbound["listen"] != "0.0.0.0" {
			t.Fatalf("%s listener fields = %#v", protocol, inbound)
		}
		if protocol != "vless" {
			if inbound["settings"].(map[string]any)["customMarker"] != "kept" {
				t.Fatalf("%s settings JSON was not preserved", protocol)
			}
			if inbound["streamSettings"].(map[string]any)["network"] != "tcp" {
				t.Fatalf("%s streamSettings JSON was not preserved", protocol)
			}
			if links := makeVLESSLinks(s, s.Users[0]); len(links) != 0 {
				t.Fatalf("%s unexpectedly generated VLESS links: %+v", protocol, links)
			}
		}
	}
}

func TestLegacyProtocolDefaultsToVLESS(t *testing.T) {
	s := testState(t)
	s.Settings.Protocol = ""
	if effectiveInboundProtocol(s.Settings) != "vless" {
		t.Fatal("legacy settings without protocol did not default to VLESS")
	}
	if err := validateSettings(s.Settings); err != nil {
		t.Fatalf("legacy settings rejected: %v", err)
	}
	if links := makeVLESSLinks(s, s.Users[0]); len(links) != 1 {
		t.Fatalf("legacy VLESS links = %d, want 1", len(links))
	}
}

func TestCustomInboundSettingsMustBeJSONObjects(t *testing.T) {
	s := testState(t)
	s.Settings.Protocol = "socks"
	s.Settings.ProtocolSettings = `[]`
	if err := validateSettings(s.Settings); err == nil {
		t.Fatal("accepted an array instead of protocol settings object")
	}
	s.Settings.ProtocolSettings = `{invalid`
	if err := validateSettings(s.Settings); err == nil {
		t.Fatal("accepted malformed protocol settings JSON")
	}
	s.Settings.ProtocolSettings = `{}`
	s.Settings.ProtocolStreamSettings = `null`
	if err := validateSettings(s.Settings); err == nil {
		t.Fatal("accepted null instead of stream settings object")
	}
	s.Settings.ProtocolStreamSettings = `{}`
	s.Settings.Protocol = "not-an-xray-protocol"
	if err := validateSettings(s.Settings); err == nil {
		t.Fatal("accepted an unsupported inbound protocol")
	}
}

func TestBuildMultipleDistinctManagedInboundsAndClientLinks(t *testing.T) {
	s := testState(t)
	s.Users[0].Name = "Client A #1"
	vmessID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	shadowsocksPassword, err := randomHex(16)
	if err != nil {
		t.Fatal(err)
	}
	s.Inbounds = []ManagedInbound{
		{ID: "vmess-proto-1", Protocol: "vmess", InternalPort: 20152, NATMappings: []string{"vmess.example.net:22152"}},
		{ID: "ss-proto-01", Protocol: "shadowsocks", InternalPort: 20153, NATMappings: []string{"ss.example.net:22153"}},
	}
	s.Users[0].ProtocolCredentials = map[string]string{
		"vmess-proto-1": vmessID,
		"ss-proto-01":   shadowsocksPassword,
	}

	cfg, err := buildXrayConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(cfg, &decoded); err != nil {
		t.Fatal(err)
	}
	inbounds := decoded["inbounds"].([]any)
	if len(inbounds) != 3 {
		t.Fatalf("combined inbound count = %d, want 3", len(inbounds))
	}
	wantProtocols := []string{"vless", "vmess", "shadowsocks"}
	for i, want := range wantProtocols {
		got := inbounds[i].(map[string]any)["protocol"]
		if got != want {
			t.Fatalf("inbound %d protocol = %v, want %s", i, got, want)
		}
	}
	vless := inbounds[0].(map[string]any)
	if vless["port"].(float64) != float64(s.Settings.InternalPort) {
		t.Fatalf("existing VLESS listener port changed: %v", vless["port"])
	}
	vmess := inbounds[1].(map[string]any)
	vmessUsers := vmess["settings"].(map[string]any)["users"].([]any)
	if vmess["port"].(float64) != 20152 || vmessUsers[0].(map[string]any)["id"] != vmessID {
		t.Fatal("VMess listener or per-user UUID missing")
	}
	shadowsocks := inbounds[2].(map[string]any)
	ssSettings := shadowsocks["settings"].(map[string]any)
	ssUsers := ssSettings["users"].([]any)
	if shadowsocks["port"].(float64) != 20153 || ssSettings["network"] != "tcp" || ssUsers[0].(map[string]any)["password"] != shadowsocksPassword {
		t.Fatal("Shadowsocks listener or TCP-only per-user credential missing")
	}

	links := makeUserLinks(s, s.Users[0])
	if len(links) != 3 {
		t.Fatalf("generated client link count = %d, want 3", len(links))
	}
	if !strings.HasPrefix(links[0].Link, "vless://") || !strings.HasPrefix(links[1].Link, "vmess://"+vmessID+"@vmess.example.net:22152?") || !strings.HasPrefix(links[2].Link, "ss://") {
		t.Fatal("client links do not match their protocol or mapped endpoint")
	}
	if !strings.HasSuffix(links[1].Link, "#Client%20A%20%231") || !strings.HasSuffix(links[2].Link, "#Client%20A%20%231") {
		t.Fatal("VMess/Shadowsocks tags are not URI percent-encoded")
	}
	if strings.Contains(links[2].Link, s.RealityPrivateKey) {
		t.Fatal("generated link exposed the REALITY private key")
	}
}

func TestManagedInboundsRequireDistinctProtocolsPortsAndCombinedMappingLimit(t *testing.T) {
	s := testState(t)
	vmessID, _ := newUUID()
	ssPassword, _ := randomHex(16)
	s.Inbounds = []ManagedInbound{{ID: "vmess-proto-1", Protocol: "vmess", InternalPort: 20152}}
	s.Users[0].ProtocolCredentials = map[string]string{"vmess-proto-1": vmessID}
	if err := validateManagedInbounds(s); err != nil {
		t.Fatalf("valid additional inbound rejected: %v", err)
	}

	bad := s
	bad.Inbounds = append(append([]ManagedInbound(nil), s.Inbounds...), ManagedInbound{ID: "vmess-proto-2", Protocol: "vmess", InternalPort: 20153})
	if validateManagedInbounds(bad) == nil {
		t.Fatal("accepted two active inbounds using the same protocol")
	}
	bad = s
	bad.Inbounds = append([]ManagedInbound(nil), s.Inbounds...)
	bad.Inbounds[0].InternalPort = s.Settings.InternalPort
	if validateManagedInbounds(bad) == nil {
		t.Fatal("accepted duplicate internal listener ports")
	}
	bad = s
	bad.Inbounds = append([]ManagedInbound(nil), s.Inbounds...)
	for i := 0; i < 8; i++ {
		bad.Inbounds[0].NATMappings = append(bad.Inbounds[0].NATMappings, "extra-"+strconv.Itoa(i)+".example.net:"+strconv.Itoa(23000+i))
	}
	if validateManagedInbounds(bad) == nil {
		t.Fatal("accepted more than eight Xray mappings including the primary endpoint")
	}
	bad = s
	bad.Inbounds = append([]ManagedInbound(nil), s.Inbounds...)
	bad.Users = append([]User(nil), s.Users...)
	bad.Users[0].ProtocolCredentials = map[string]string{"vmess-proto-1": ssPassword}
	if validateManagedInbounds(bad) == nil {
		t.Fatal("accepted non-UUID VMess credential")
	}
}

func TestVLESSVisionAndXHTTPREALITYRunAsSeparateInbounds(t *testing.T) {
	s := testState(t)
	s.Settings.InternalPort = 20151
	inbound, err := newVLESSXHTTPInbound("xhttp-inbound-1", 20152, s.Settings)
	if err != nil {
		t.Fatal(err)
	}
	inbound.NATMappings = []string{"xhttp.example.net:20252"}
	s.Inbounds = []ManagedInbound{inbound}
	if err := validateManagedInbounds(s); err != nil {
		t.Fatal(err)
	}
	cfg, err := buildXrayConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(cfg, &decoded); err != nil {
		t.Fatal(err)
	}
	inbounds := decoded["inbounds"].([]any)
	if len(inbounds) != 2 {
		t.Fatalf("generated %d inbounds, want 2", len(inbounds))
	}
	primary := inbounds[0].(map[string]any)
	primaryStream := primary["streamSettings"].(map[string]any)
	if primary["protocol"] != "vless" || primary["port"].(float64) != 20151 || primaryStream["network"] != "tcp" || primaryStream["security"] != "reality" {
		t.Fatalf("primary Vision inbound changed: %#v", primary)
	}
	primaryUsers := primary["settings"].(map[string]any)["clients"].([]any)
	if primaryUsers[0].(map[string]any)["flow"] != "xtls-rprx-vision" {
		t.Fatal("primary Vision flow was not preserved")
	}
	second := inbounds[1].(map[string]any)
	secondStream := second["streamSettings"].(map[string]any)
	if second["protocol"] != "vless" || second["port"].(float64) != 20152 || secondStream["network"] != "xhttp" || secondStream["security"] != "reality" {
		t.Fatalf("XHTTP inbound not generated independently: %#v", second)
	}
	secondUsers := second["settings"].(map[string]any)["clients"].([]any)
	if secondUsers[0].(map[string]any)["id"] != s.Users[0].UUID {
		t.Fatal("XHTTP inbound did not preserve the user's existing UUID")
	}
	if inbound.RealityPrivateKey == s.RealityPrivateKey || inbound.RealityPublicKey == s.RealityPublicKey || inbound.RealityShortID == s.RealityShortID {
		t.Fatal("XHTTP inbound reused the primary REALITY material")
	}
	links := makeUserLinks(s, s.Users[0])
	if len(links) != 2 || links[1].Address != "xhttp.example.net:20252" || links[1].InboundID != inbound.ID {
		t.Fatalf("completed XHTTP link missing for the provider endpoint")
	}
	if !strings.HasPrefix(links[1].Link, "vless://"+s.Users[0].UUID+"@xhttp.example.net:20252?") || !strings.Contains(links[1].Link, "type=xhttp") || !strings.Contains(links[1].Link, "security=reality") || strings.Contains(links[1].Link, inbound.RealityPrivateKey) {
		t.Fatal("generated XHTTP client link is malformed or exposes a private key")
	}
}

func TestVLESSXHTTPInboundRequiresDistinctPortAndSafePath(t *testing.T) {
	s := testState(t)
	inbound, err := newVLESSXHTTPInbound("xhttp-inbound-1", s.Settings.InternalPort+1, s.Settings)
	if err != nil {
		t.Fatal(err)
	}
	s.Inbounds = []ManagedInbound{inbound}
	if err := validateManagedInbounds(s); err != nil {
		t.Fatal(err)
	}
	s.Inbounds[0].InternalPort = s.Settings.InternalPort
	if validateManagedInbounds(s) == nil {
		t.Fatal("accepted duplicate primary and XHTTP listener ports")
	}
	s.Inbounds[0] = inbound
	s.Inbounds[0].XHTTPPath = "/x?unsafe=1"
	if validateManagedInbounds(s) == nil {
		t.Fatal("accepted an unsafe XHTTP path")
	}
	s.Inbounds[0] = inbound
	s.Inbounds[0].Mode = "tcp"
	if validateManagedInbounds(s) == nil {
		t.Fatal("accepted a non-XHTTP additional VLESS inbound")
	}
}
