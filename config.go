package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Settings struct {
	PublicHost   string `json:"publicHost"`
	PublicPort   int    `json:"publicPort"`
	InternalPort int    `json:"internalPort"`
	Mode         string `json:"mode"`
	SNI          string `json:"sni"`
	Dest         string `json:"dest"`
	XHTTPPath    string `json:"xhttpPath"`
}

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UUID      string    `json:"uuid"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
}

type appState struct {
	Schema            int      `json:"schema"`
	Settings          Settings `json:"settings"`
	Users             []User   `json:"users"`
	AdminHash         string   `json:"adminHash"`
	RealityPrivateKey string   `json:"realityPrivateKey"`
	RealityPublicKey  string   `json:"realityPublicKey"`
	RealityShortID    string   `json:"realityShortId"`
}

var hostnameRE = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)
var shortIDRE = regexp.MustCompile(`^[0-9a-f]{2,16}$`)
var nameRE = regexp.MustCompile(`^[^\x00-\x1f\x7f]{1,64}$`)
var xhttpPathRE = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{1,127}$`)

func validPort(p int) bool { return p >= 1 && p <= 65535 }

func validateHostname(s string) error {
	if s != strings.TrimSpace(s) || s == "" || len(s) > 253 || strings.ContainsAny(s, "[]/:@?#\\ \t\r\n") {
		return errors.New("alamat harus berupa hostname atau alamat IP tanpa port")
	}
	if net.ParseIP(s) == nil && !hostnameRE.MatchString(s) {
		return errors.New("hostname tidak valid")
	}
	return nil
}

func validateSettings(s Settings) error {
	if err := validateHostname(s.PublicHost); err != nil {
		return err
	}
	if !validPort(s.PublicPort) || !validPort(s.InternalPort) {
		return errors.New("port harus antara 1 dan 65535")
	}
	if s.Mode != "tcp" && s.Mode != "xhttp" {
		return errors.New("mode harus tcp atau xhttp")
	}
	if !hostnameRE.MatchString(s.SNI) || len(s.SNI) > 253 || strings.Contains(s.SNI, "..") {
		return errors.New("SNI tidak valid")
	}
	if strings.ContainsAny(s.SNI, "/:@?#\\ \t\r\n") {
		return errors.New("SNI tidak valid")
	}
	host, port, err := net.SplitHostPort(s.Dest)
	if err != nil || host == "" {
		return errors.New("dest harus berupa hostname:port atau IP:port")
	}
	if net.ParseIP(host) == nil && (!hostnameRE.MatchString(host) || strings.Contains(host, "..")) {
		return errors.New("hostname dest tidak valid")
	}
	p, err := strconv.Atoi(port)
	if err != nil || !validPort(p) {
		return errors.New("port dest tidak valid")
	}
	if s.Mode == "xhttp" {
		if !xhttpPathRE.MatchString(s.XHTTPPath) {
			return errors.New("path XHTTP harus diawali / dan hanya memakai karakter aman")
		}
	}
	return nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}

func newRealityKeys() (string, string, error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString(key.Bytes()), enc.EncodeToString(key.PublicKey().Bytes()), nil
}

func makeDefaultState(host string, panelPublicPort, panelInternalPort, xrayPublicPort, xrayInternalPort int, passwordHash string) (appState, error) {
	priv, pub, err := newRealityKeys()
	if err != nil {
		return appState{}, err
	}
	shortID, err := randomHex(8)
	if err != nil {
		return appState{}, err
	}
	path, err := randomHex(16)
	if err != nil {
		return appState{}, err
	}
	s := appState{
		Schema: 1,
		Settings: Settings{
			PublicHost: host, PublicPort: xrayPublicPort, InternalPort: xrayInternalPort,
			Mode: "tcp", SNI: "www.cloudflare.com", Dest: "www.cloudflare.com:443", XHTTPPath: "/" + path,
		},
		Users: []User{}, AdminHash: passwordHash,
		RealityPrivateKey: priv, RealityPublicKey: pub, RealityShortID: shortID,
	}
	if !validPort(panelPublicPort) || !validPort(panelInternalPort) {
		return appState{}, errors.New("port panel tidak valid")
	}
	if err := validateSettings(s.Settings); err != nil {
		return appState{}, err
	}
	return s, nil
}

func buildXrayConfig(s appState) ([]byte, error) {
	if err := validateSettings(s.Settings); err != nil {
		return nil, err
	}
	if s.RealityPrivateKey == "" || !shortIDRE.MatchString(s.RealityShortID) {
		return nil, errors.New("key REALITY internal tidak valid")
	}
	clients := make([]map[string]string, 0, len(s.Users))
	for _, u := range s.Users {
		if !u.Enabled {
			continue
		}
		if _, err := parseUUID(u.UUID); err != nil {
			return nil, fmt.Errorf("UUID user %q tidak valid", u.Name)
		}
		c := map[string]string{"id": u.UUID}
		if s.Settings.Mode == "tcp" {
			c["flow"] = "xtls-rprx-vision"
		}
		clients = append(clients, c)
	}
	stream := map[string]any{
		"network":  s.Settings.Mode,
		"security": "reality",
		"realitySettings": map[string]any{
			"show":        false,
			"target":      s.Settings.Dest,
			"serverNames": []string{s.Settings.SNI},
			"privateKey":  s.RealityPrivateKey,
			"shortIds":    []string{s.RealityShortID},
		},
	}
	if s.Settings.Mode == "xhttp" {
		stream["xhttpSettings"] = map[string]any{"path": s.Settings.XHTTPPath}
	}
	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{map[string]any{
			"listen": "0.0.0.0", "port": s.Settings.InternalPort, "protocol": "vless",
			"settings":       map[string]any{"clients": clients, "decryption": "none"},
			"streamSettings": stream,
			"sniffing":       map[string]any{"enabled": true, "destOverride": []string{"http", "tls"}, "routeOnly": true},
		}},
		"outbounds": []any{
			map[string]any{"protocol": "freedom", "tag": "direct"},
			map[string]any{"protocol": "blackhole", "tag": "block"},
		},
		"routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{map[string]any{"type": "field", "network": "udp", "outboundTag": "block"}}},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func makeVLESSLink(s appState, u User) string {
	params := url.Values{}
	params.Set("encryption", "none")
	params.Set("security", "reality")
	params.Set("sni", s.Settings.SNI)
	params.Set("fp", "chrome")
	params.Set("pbk", s.RealityPublicKey)
	params.Set("sid", s.RealityShortID)
	if s.Settings.Mode == "tcp" {
		params.Set("type", "tcp")
		params.Set("flow", "xtls-rprx-vision")
	} else {
		params.Set("type", "xhttp")
		params.Set("path", s.Settings.XHTTPPath)
		params.Set("mode", "auto")
	}
	host := s.Settings.PublicHost
	return "vless://" + u.UUID + "@" + net.JoinHostPort(host, strconv.Itoa(s.Settings.PublicPort)) + "?" + params.Encode() + "#" + url.QueryEscape(u.Name)
}

func parseUUID(s string) ([]byte, error) {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return nil, errors.New("format UUID salah")
	}
	return hex.DecodeString(strings.ReplaceAll(s, "-", ""))
}

func validateUserName(name string) error {
	if strings.TrimSpace(name) != name || !nameRE.MatchString(name) {
		return errors.New("nama harus 1–64 karakter, tanpa karakter kontrol atau spasi di ujung")
	}
	return nil
}
