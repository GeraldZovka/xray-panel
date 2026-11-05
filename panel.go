package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultStatePath      = "/var/lib/xray-panel/state.json"
	sessionCookie         = "xray_panel_session"
	sessionLifetime       = 12 * time.Hour
	maxBodyBytes          = 2 << 20
	minAdminPasswordBytes = 8
	maxAdminPasswordBytes = 1024
)

func validAdminPasswordLength(password []byte) bool {
	return len(password) >= minAdminPasswordBytes && len(password) <= maxAdminPasswordBytes
}

type session struct {
	CSRF    string
	Expires time.Time
}
type loginWindow struct {
	Count int
	Until time.Time
}
type server struct {
	mu        sync.Mutex
	state     appState
	statePath string
	sessions  map[string]session
	login     map[string]loginWindow
	sessionMu sync.Mutex
	tpl       *template.Template
}

type publicUser struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Enabled   bool         `json:"enabled"`
	CreatedAt time.Time    `json:"createdAt"`
	Link      string       `json:"link"`
	Links     []publicLink `json:"links"`
}
type publicState struct {
	CSRF     string           `json:"csrf"`
	Settings Settings         `json:"settings"`
	Inbounds []ManagedInbound `json:"inbounds"`
	Users    []publicUser     `json:"users"`
	Running  bool             `json:"running"`
	Version  string           `json:"version"`
	Uptime   string           `json:"uptime"`
	CPU      string           `json:"cpu"`
	Load     string           `json:"load"`
	RAM      string           `json:"ram"`
	Logs     string           `json:"logs"`
	HTTPS    bool             `json:"https"`
}
type helperRequest struct {
	Action    string `json:"action"`
	Config    string `json:"config,omitempty"`
	Operation string `json:"operation,omitempty"`
}
type helperResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Status  string `json:"status,omitempty"`
	Version string `json:"version,omitempty"`
	Uptime  string `json:"uptime,omitempty"`
	CPU     string `json:"cpu,omitempty"`
	Load    string `json:"load,omitempty"`
	RAM     string `json:"ram,omitempty"`
	Logs    string `json:"logs,omitempty"`
}

type backupFile struct {
	Schema            int              `json:"schema"`
	Settings          Settings         `json:"settings"`
	Users             []User           `json:"users"`
	Inbounds          []ManagedInbound `json:"inbounds,omitempty"`
	RealityPrivateKey string           `json:"realityPrivateKey"`
	RealityPublicKey  string           `json:"realityPublicKey"`
	RealityShortID    string           `json:"realityShortId"`
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		return initialize(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "apply-state" {
		if len(os.Args) != 2 {
			return errors.New("apply-state tidak menerima argumen tambahan")
		}
		return applyStoredState()
	}
	statePath := os.Getenv("XRAY_PANEL_STATE")
	if statePath == "" {
		statePath = defaultStatePath
	}
	st, err := loadState(statePath)
	if err != nil {
		return fmt.Errorf("membaca state panel: %w (jalankan install.sh terlebih dahulu)", err)
	}
	if err := validateStoredState(st); err != nil {
		return fmt.Errorf("state panel tidak valid: %w", err)
	}
	tpl, err := template.New("index").Parse(indexHTML)
	if err != nil {
		return err
	}
	s := &server{state: st, statePath: statePath, sessions: map[string]session{}, login: map[string]loginWindow{}, tpl: tpl}
	addr := os.Getenv("PANEL_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/action", s.handleAction)
	mux.HandleFunc("/api/qr", s.handleQR)
	mux.HandleFunc("/api/backup", s.handleBackup)
	wrapped := securityHeaders(mux)
	log.Printf("xray-panel listening on %s", addr)
	cert, key := os.Getenv("PANEL_TLS_CERT"), os.Getenv("PANEL_TLS_KEY")
	if (cert == "") != (key == "") {
		return errors.New("PANEL_TLS_CERT dan PANEL_TLS_KEY harus dipasang berpasangan")
	}
	httpServer := &http.Server{Addr: addr, Handler: wrapped, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	if cert != "" {
		return httpServer.ListenAndServeTLS(cert, key)
	}
	return httpServer.ListenAndServe()
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func applyStoredState() error {
	statePath := os.Getenv("XRAY_PANEL_STATE")
	if statePath == "" {
		statePath = defaultStatePath
	}
	st, err := loadState(statePath)
	if err != nil {
		return fmt.Errorf("membaca state panel: %w", err)
	}
	if err := validateStoredState(st); err != nil {
		return fmt.Errorf("state panel tidak valid: %w", err)
	}
	cfg, err := buildXrayConfig(st)
	if err != nil {
		return err
	}
	resp, err := callHelper(helperRequest{Action: "apply", Config: string(cfg)})
	if err != nil {
		return errors.New(redact(err.Error(), st.RealityPrivateKey))
	}
	if !resp.OK {
		return errors.New(redact(resp.Error, st.RealityPrivateKey))
	}
	return nil
}

func initialize(args []string) error {
	fs := newFlagSet("init")
	statePath := fs.String("state", defaultStatePath, "state file path")
	panelInternal := fs.Int("panel-internal-port", 0, "internal panel port")
	xrayInternal := fs.Int("xray-internal-port", 0, "internal Xray port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pw, err := io.ReadAll(io.LimitReader(os.Stdin, 4097))
	if err != nil {
		return err
	}
	pw = bytes.TrimSuffix(pw, []byte("\n"))
	pw = bytes.TrimSuffix(pw, []byte("\r"))
	if !validAdminPasswordLength(pw) {
		return errors.New("password admin harus 8–1024 byte")
	}
	hash, err := bcrypt.GenerateFromPassword(pw, 12)
	for i := range pw {
		pw[i] = 0
	}
	if err != nil {
		return err
	}
	if _, err := os.Stat(*statePath); err == nil {
		return fmt.Errorf("state %s sudah ada; init tidak akan menimpanya", *statePath)
	} else if !os.IsNotExist(err) {
		return err
	}
	st, err := makeDefaultState(*panelInternal, *xrayInternal, string(hash))
	if err != nil {
		return err
	}
	if err := validateSettings(st.Settings); err != nil {
		return err
	}
	if err := saveState(*statePath, st); err != nil {
		return err
	}
	cfg, err := buildXrayConfig(st)
	if err != nil {
		return err
	}
	resp, err := callHelper(helperRequest{Action: "apply", Config: string(cfg)})
	if err != nil {
		_ = os.Remove(*statePath)
		return fmt.Errorf("konfigurasi awal Xray gagal: %s", redact(err.Error(), st.RealityPrivateKey))
	}
	if !resp.OK {
		_ = os.Remove(*statePath)
		return fmt.Errorf("konfigurasi awal Xray gagal: %s", redact(resp.Error, st.RealityPrivateKey))
	}
	return nil
}

// flagSet is a tiny adapter so the one-shot init mode has no extra CLI dependency.
type flagSet struct{ fs *flag.FlagSet }

func newFlagSet(name string) *flagSet {
	return &flagSet{fs: flag.NewFlagSet(name, flag.ContinueOnError)}
}
func (f *flagSet) String(name, value, usage string) *string      { return f.fs.String(name, value, usage) }
func (f *flagSet) Int(name string, value int, usage string) *int { return f.fs.Int(name, value, usage) }
func (f *flagSet) Parse(args []string) error                     { return f.fs.Parse(args) }

func loadState(path string) (appState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return appState{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return appState{}, errors.New("state harus berupa file reguler, bukan symlink")
	}
	if info.Mode().Perm() != 0600 {
		if err := os.Chmod(path, 0600); err != nil {
			return appState{}, fmt.Errorf("mengatur izin state ke 0600: %w", err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return appState{}, err
	}
	var st appState
	if err := json.Unmarshal(b, &st); err != nil {
		return appState{}, err
	}
	return st, nil
}
func saveState(path string, st appState) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
func validateStoredState(st appState) error {
	if st.Schema != 1 || st.AdminHash == "" {
		return errors.New("versi state atau hash admin tidak ditemukan")
	}
	if err := validateSettings(st.Settings); err != nil {
		return err
	}
	if err := validateManagedInbounds(st); err != nil {
		return err
	}
	if st.RealityPrivateKey == "" || st.RealityPublicKey == "" || !shortIDRE.MatchString(st.RealityShortID) {
		return errors.New("material REALITY tidak lengkap")
	}
	seen := map[string]bool{}
	for _, u := range st.Users {
		if u.ID == "" || seen[u.ID] {
			return errors.New("ID user kosong atau duplikat")
		}
		seen[u.ID] = true
		if err := validateUserName(u.Name); err != nil {
			return err
		}
		if _, err := parseUUID(u.UUID); err != nil {
			return err
		}
	}
	return nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.Execute(w, nil); err != nil {
		log.Printf("template error: %v", err)
	}
}
func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	csrf, ok := s.authenticated(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	resp, err := callHelper(helperRequest{Action: "status"})
	if err != nil {
		resp = helperResponse{OK: false, Error: "helper layanan tidak tersedia"}
	}
	logs := redact(resp.Logs, st.RealityPrivateKey)
	if !resp.OK && logs == "" {
		logs = redact(resp.Error, st.RealityPrivateKey)
	}
	users := make([]publicUser, 0, len(st.Users))
	for _, u := range st.Users {
		links := makeUserLinks(st, u)
		primary := ""
		if len(links) > 0 {
			primary = links[0].Link
		}
		users = append(users, publicUser{ID: u.ID, Name: u.Name, Enabled: u.Enabled, CreatedAt: u.CreatedAt, Link: primary, Links: links})
	}
	out := publicState{CSRF: csrf, Settings: st.Settings, Inbounds: st.Inbounds, Users: users, Running: resp.Status == "active", Version: resp.Version, Uptime: resp.Uptime, CPU: resp.CPU, Load: resp.Load, RAM: resp.RAM, Logs: logs, HTTPS: isHTTPS(r)}
	writeJSON(w, http.StatusOK, out)
}
func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "origin tidak diizinkan", http.StatusForbidden)
		return
	}
	ip := clientIP(r)
	if !s.allowLogin(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Terlalu banyak percobaan. Coba lagi 15 menit kemudian."})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	hash := s.state.AdminHash
	s.mu.Unlock()
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) != nil {
		s.failedLogin(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Password salah."})
		return
	}
	s.resetLogin(ip)
	token, err := randomToken(32)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	s.sessionMu.Lock()
	now := time.Now()
	for oldToken, oldSession := range s.sessions {
		if now.After(oldSession.Expires) {
			delete(s.sessions, oldToken)
		}
	}
	if len(s.sessions) >= 1024 {
		var oldestToken string
		var oldestExpiry time.Time
		for oldToken, oldSession := range s.sessions {
			if oldestToken == "" || oldSession.Expires.Before(oldestExpiry) {
				oldestToken, oldestExpiry = oldToken, oldSession.Expires
			}
		}
		delete(s.sessions, oldestToken)
	}
	s.sessions[token] = session{CSRF: csrf, Expires: time.Now().Add(sessionLifetime)}
	s.sessionMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode, Expires: time.Now().Add(sessionLifetime), MaxAge: int(sessionLifetime.Seconds())})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "csrf": csrf})
}
func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	_, ok := s.authenticated(w, r)
	if !ok {
		return
	}
	c, _ := r.Cookie(sessionCookie)
	s.sessionMu.Lock()
	delete(s.sessions, c.Value)
	s.sessionMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if _, ok := s.authenticated(w, r); !ok {
		return
	}
	var raw map[string]json.RawMessage
	if err := decodeBody(w, r, &raw); err != nil {
		writeError(w, 400, err)
		return
	}
	var action string
	_ = json.Unmarshal(raw["action"], &action)
	var result any = map[string]bool{"ok": true}
	var err error
	switch action {
	case "addUser":
		result, err = s.addUser(raw)
	case "deleteUser":
		err = s.deleteUser(raw)
	case "toggleUser":
		err = s.toggleUser(raw)
	case "settings":
		err = s.updateSettings(raw)
	case "addInbound":
		result, err = s.addInbound(raw)
	case "deleteInbound":
		err = s.deleteInbound(raw)
	case "inboundMappings":
		err = s.updateInboundMappings(raw)
	case "changePassword":
		err = s.changePassword(raw)
	case "service":
		result, err = s.serviceAction(raw)
	case "updateXray":
		result, err = s.updateXray()
	case "restore":
		err = s.restore(raw)
	default:
		err = errors.New("aksi tidak dikenal")
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, result)
}
func (s *server) handleQR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if _, ok := s.authenticated(w, r); !ok {
		return
	}
	id := r.URL.Query().Get("id")
	inboundID := r.URL.Query().Get("inbound")
	if inboundID == "" {
		inboundID = "primary"
	}
	mappingIndex := 0
	if value := r.URL.Query().Get("mapping"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			http.Error(w, "mapping NAT tidak valid", http.StatusBadRequest)
			return
		}
		mappingIndex = parsed
	}
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	for _, u := range st.Users {
		if u.ID == id {
			for _, link := range makeUserLinks(st, u) {
				if link.InboundID != inboundID || link.MappingIndex != mappingIndex {
					continue
				}
				png, err := qrcode.Encode(link.Link, qrcode.Medium, 256)
				if err != nil {
					http.Error(w, "gagal membuat QR", 500)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				w.Header().Set("Cache-Control", "no-store")
				_, _ = w.Write(png)
				return
			}
			http.Error(w, "mapping NAT tidak ditemukan", http.StatusBadRequest)
			return
		}
	}
	http.NotFound(w, r)
}
func (s *server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if _, ok := s.authenticated(w, r); !ok {
		return
	}
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	b := backupFile{Schema: st.Schema, Settings: st.Settings, Users: st.Users, Inbounds: st.Inbounds, RealityPrivateKey: st.RealityPrivateKey, RealityPublicKey: st.RealityPublicKey, RealityShortID: st.RealityShortID}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		writeError(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=xray-panel-backup-%s.json", time.Now().UTC().Format("20060102T150405Z")))
	_, _ = w.Write(data)
}

func (s *server) addUser(raw map[string]json.RawMessage) (any, error) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw["name"], &in.Name); err != nil {
		return nil, errors.New("nama user wajib diisi")
	}
	if err := validateUserName(in.Name); err != nil {
		return nil, err
	}
	id, err := randomToken(12)
	if err != nil {
		return nil, err
	}
	uuid, err := newUUID()
	if err != nil {
		return nil, err
	}
	credentials := map[string]string{}
	err = s.transact(func(st *appState) error {
		for _, u := range st.Users {
			if strings.EqualFold(u.Name, in.Name) {
				return errors.New("nama user sudah digunakan")
			}
		}
		for _, inbound := range st.Inbounds {
			credential, err := newProtocolCredential(inbound.Protocol)
			if err != nil {
				return err
			}
			credentials[inbound.ID] = credential
		}
		st.Users = append(st.Users, User{ID: id, Name: in.Name, UUID: uuid, ProtocolCredentials: credentials, Enabled: true, CreatedAt: time.Now().UTC()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"id": id}, nil
}
func (s *server) deleteUser(raw map[string]json.RawMessage) error {
	id, err := requiredString(raw, "id")
	if err != nil {
		return err
	}
	return s.transact(func(st *appState) error {
		for i, u := range st.Users {
			if u.ID == id {
				st.Users = append(st.Users[:i], st.Users[i+1:]...)
				return nil
			}
		}
		return errors.New("user tidak ditemukan")
	})
}
func (s *server) toggleUser(raw map[string]json.RawMessage) error {
	id, err := requiredString(raw, "id")
	if err != nil {
		return err
	}
	return s.transact(func(st *appState) error {
		for i := range st.Users {
			if st.Users[i].ID == id {
				st.Users[i].Enabled = !st.Users[i].Enabled
				return nil
			}
		}
		return errors.New("user tidak ditemukan")
	})
}
func (s *server) updateSettings(raw map[string]json.RawMessage) error {
	var in Settings
	if err := json.Unmarshal(raw["settings"], &in); err != nil {
		return errors.New("pengaturan tidak valid")
	}
	in.PublicHost = strings.TrimSpace(in.PublicHost)
	in.SNI = strings.TrimSpace(in.SNI)
	in.Dest = strings.TrimSpace(in.Dest)
	for i := range in.NATMappings {
		in.NATMappings[i] = strings.TrimSpace(in.NATMappings[i])
	}
	if len(in.NATMappings) > 0 {
		host, port, err := parseNATMappingOrPort(in.NATMappings[0])
		if err != nil {
			return err
		}
		in.PublicHost, in.PublicPort = host, port
	}
	return s.transact(func(st *appState) error { st.Settings = in; return validateSettings(st.Settings) })
}

func (s *server) addInbound(raw map[string]json.RawMessage) (any, error) {
	var in struct {
		Protocol     string `json:"protocol"`
		InternalPort int    `json:"internalPort"`
	}
	if err := json.Unmarshal(raw["protocol"], &in.Protocol); err != nil || !contains(managedInboundProtocols, in.Protocol) {
		return nil, errors.New("pilih protokol tambahan VMess atau Shadowsocks")
	}
	if !validPort(in.InternalPort) {
		return nil, errors.New("port internal harus antara 1 dan 65535")
	}
	id, err := randomToken(12)
	if err != nil {
		return nil, err
	}
	err = s.transact(func(st *appState) error {
		if len(st.Inbounds) >= 8 {
			return errors.New("maksimal 8 inbound tambahan dikelola panel")
		}
		for _, current := range st.Inbounds {
			if current.Protocol == in.Protocol {
				return errors.New("inbound aktif sudah memakai protokol itu")
			}
		}
		managed := ManagedInbound{ID: id, Protocol: in.Protocol, InternalPort: in.InternalPort}
		for i := range st.Users {
			credential, err := newProtocolCredential(in.Protocol)
			if err != nil {
				return err
			}
			if st.Users[i].ProtocolCredentials == nil {
				st.Users[i].ProtocolCredentials = map[string]string{}
			}
			st.Users[i].ProtocolCredentials[id] = credential
		}
		st.Inbounds = append(st.Inbounds, managed)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"id": id}, nil
}

func (s *server) deleteInbound(raw map[string]json.RawMessage) error {
	id, err := requiredString(raw, "id")
	if err != nil {
		return err
	}
	return s.transact(func(st *appState) error {
		for i, inbound := range st.Inbounds {
			if inbound.ID != id {
				continue
			}
			st.Inbounds = append(st.Inbounds[:i], st.Inbounds[i+1:]...)
			for userIndex := range st.Users {
				delete(st.Users[userIndex].ProtocolCredentials, id)
			}
			return nil
		}
		return errors.New("inbound tidak ditemukan")
	})
}

func (s *server) updateInboundMappings(raw map[string]json.RawMessage) error {
	var in struct {
		ID          string   `json:"id"`
		NATMappings []string `json:"natMappings"`
	}
	if err := json.Unmarshal(raw["inbound"], &in); err != nil || in.ID == "" {
		return errors.New("pengaturan mapping inbound tidak valid")
	}
	for i := range in.NATMappings {
		in.NATMappings[i] = strings.TrimSpace(in.NATMappings[i])
	}
	return s.transact(func(st *appState) error {
		for i := range st.Inbounds {
			if st.Inbounds[i].ID == in.ID {
				st.Inbounds[i].NATMappings = in.NATMappings
				return nil
			}
		}
		return errors.New("inbound tidak ditemukan")
	})
}
func (s *server) changePassword(raw map[string]json.RawMessage) error {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.Unmarshal(raw["passwords"], &in); err != nil {
		return errors.New("isian password tidak valid")
	}
	if !validAdminPasswordLength([]byte(in.New)) {
		return errors.New("password baru harus 8–1024 byte")
	}
	s.mu.Lock()
	oldHash := s.state.AdminHash
	s.mu.Unlock()
	if bcrypt.CompareHashAndPassword([]byte(oldHash), []byte(in.Current)) != nil {
		return errors.New("password lama salah")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.New), 12)
	if err != nil {
		return err
	}
	err = s.transact(func(st *appState) error { st.AdminHash = string(hash); return nil })
	if err == nil {
		s.sessionMu.Lock()
		s.sessions = map[string]session{}
		s.sessionMu.Unlock()
	}
	return err
}
func (s *server) serviceAction(raw map[string]json.RawMessage) (any, error) {
	op, err := requiredString(raw, "operation")
	if err != nil {
		return nil, err
	}
	if op != "start" && op != "stop" && op != "restart" {
		return nil, errors.New("operasi layanan tidak diizinkan")
	}
	resp, err := callHelper(helperRequest{Action: "service", Operation: op})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	return map[string]bool{"ok": true}, nil
}
func (s *server) updateXray() (any, error) {
	resp, err := callHelper(helperRequest{Action: "update"})
	if err != nil {
		s.mu.Lock()
		private := s.state.RealityPrivateKey
		s.mu.Unlock()
		return nil, errors.New(redact(err.Error(), private))
	}
	if !resp.OK {
		s.mu.Lock()
		private := s.state.RealityPrivateKey
		s.mu.Unlock()
		return nil, errors.New(redact(resp.Error, private))
	}
	return map[string]bool{"ok": true}, nil
}
func (s *server) restore(raw map[string]json.RawMessage) error {
	var b backupFile
	if err := json.Unmarshal(raw["backup"], &b); err != nil {
		return errors.New("file backup tidak valid")
	}
	if b.Schema != 1 {
		return errors.New("versi backup tidak didukung")
	}
	return s.transact(func(st *appState) error {
		hash := st.AdminHash
		*st = appState{Schema: b.Schema, Settings: b.Settings, Users: b.Users, Inbounds: b.Inbounds, AdminHash: hash, RealityPrivateKey: b.RealityPrivateKey, RealityPublicKey: b.RealityPublicKey, RealityShortID: b.RealityShortID}
		return validateStoredState(*st)
	})
}
func (s *server) transact(change func(*appState) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.state
	b, _ := json.Marshal(old)
	var next appState
	_ = json.Unmarshal(b, &next)
	if err := change(&next); err != nil {
		return err
	}
	if err := validateStoredState(next); err != nil {
		return err
	}
	cfg, err := buildXrayConfig(next)
	if err != nil {
		return err
	}
	resp, err := callHelper(helperRequest{Action: "apply", Config: string(cfg)})
	if err != nil {
		return errors.New(redact(err.Error(), next.RealityPrivateKey))
	}
	if !resp.OK {
		return fmt.Errorf("Xray menolak perubahan; konfigurasi lama dipulihkan: %s", redact(resp.Error, next.RealityPrivateKey))
	}
	if err := saveState(s.statePath, next); err != nil {
		oldCfg, _ := buildXrayConfig(old)
		_, rollbackErr := callHelper(helperRequest{Action: "apply", Config: string(oldCfg)})
		if rollbackErr != nil {
			return fmt.Errorf("gagal menyimpan state (%v) dan rollback helper bermasalah: %w", err, rollbackErr)
		}
		return fmt.Errorf("gagal menyimpan state; perubahan Xray dibatalkan: %w", err)
	}
	s.state = next
	return nil
}

func (s *server) authenticated(w http.ResponseWriter, r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login diperlukan"})
		return "", false
	}
	s.sessionMu.Lock()
	sess, ok := s.sessions[c.Value]
	if ok && time.Now().After(sess.Expires) {
		delete(s.sessions, c.Value)
		ok = false
	}
	s.sessionMu.Unlock()
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sesi berakhir"})
		return "", false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		csrf := r.Header.Get("X-CSRF-Token")
		if csrf == "" || csrf != sess.CSRF || !sameOrigin(r) {
			http.Error(w, "CSRF/origin check gagal", http.StatusForbidden)
			return "", false
		}
	}
	return sess.CSRF, true
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return strings.EqualFold(origin, scheme+"://"+r.Host)
}
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// Trust a TLS-terminating reverse proxy only when it connects from this machine.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && (host == "127.0.0.1" || host == "::1") && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
func (s *server) allowLogin(ip string) bool {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	now := time.Now()
	for oldIP, window := range s.login {
		if now.After(window.Until) {
			delete(s.login, oldIP)
		}
	}
	w, ok := s.login[ip]
	if !ok || now.After(w.Until) {
		delete(s.login, ip)
		return ok || len(s.login) < 4096
	}
	return w.Count < 5
}
func (s *server) failedLogin(ip string) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	w, ok := s.login[ip]
	if !ok || time.Now().After(w.Until) {
		w = loginWindow{Until: time.Now().Add(15 * time.Minute)}
	}
	w.Count++
	s.login[ip] = w
}
func (s *server) resetLogin(ip string) { s.sessionMu.Lock(); delete(s.login, ip); s.sessionMu.Unlock() }
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func callHelper(req helperRequest) (helperResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return helperResponse{}, err
	}
	path := os.Getenv("XRAY_PANEL_HELPER")
	if path == "" {
		path = "/usr/local/sbin/xray-panel-helper"
	}
	var cmdArgs []string
	if os.Geteuid() == 0 {
		cmdArgs = []string{path}
	} else {
		cmdArgs = []string{"/usr/bin/sudo", "-n", path}
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	var resp helperResponse
	decodeErr := json.Unmarshal(out, &resp)
	if decodeErr == nil && (resp.OK || resp.Error != "") {
		resp.Error = redact(resp.Error, "")
		resp.Logs = redact(resp.Logs, "")
		return resp, nil
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return helperResponse{}, fmt.Errorf("helper gagal: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return helperResponse{}, fmt.Errorf("helper tidak tersedia: %w", err)
	}
	if decodeErr != nil {
		return helperResponse{}, fmt.Errorf("jawaban helper tidak valid")
	}
	return resp, nil
}
func redact(text, private string) string {
	if private != "" {
		text = strings.ReplaceAll(text, private, "[disembunyikan]")
	}
	return text
}
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
func requiredString(m map[string]json.RawMessage, key string) (string, error) {
	var s string
	if err := json.Unmarshal(m[key], &s); err != nil || s == "" {
		return "", fmt.Errorf("%s wajib diisi", key)
	}
	return s, nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
