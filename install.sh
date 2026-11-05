#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

APP_DIR=/opt/xray-panel
STATE_DIR=/var/lib/xray-panel
STATE_FILE=$STATE_DIR/state.json
PANEL_USER=xray-panel
PANEL_UNIT=xray-panel.service
XRAY_CONFIG=/usr/local/etc/xray/config.json
XRAY_INSTALL_URL=https://github.com/XTLS/Xray-install/raw/main/install-release.sh
REPO_URL=${XRAY_PANEL_REPO:-https://github.com/GeraldZovka/xray-panel.git}
GENERATED_PASSWORD=

say(){ printf '\n==> %s\n' "$*"; }
die(){ printf 'ERROR: %s\n' "$*" >&2; exit 1; }
require_root(){ [[ ${EUID:-$(id -u)} -eq 0 ]] || die 'Jalankan sebagai root.'; }
check_platform(){
  [[ -r /etc/os-release ]] || die 'OS tidak dikenal; script ini hanya mendukung Ubuntu dengan systemd.'
  . /etc/os-release
  [[ ${ID:-} == ubuntu ]] || die "OS didukung: Ubuntu. Terdeteksi: ${ID:-unknown}."
  dpkg --compare-versions "${VERSION_ID:-0}" ge 22.04 || die 'Ubuntu 22.04 atau lebih baru diperlukan agar Go compiler dari repositori dapat membangun panel.'
  command -v systemctl >/dev/null || die 'systemd tidak ditemukan.'
  local systemd_version
  systemd_version=$(systemctl --version | awk 'NR==1 {print $2}')
  [[ $systemd_version =~ ^[0-9]+$ ]] && (( systemd_version >= 249 )) || die 'systemd 249 atau lebih baru diperlukan untuk menyerahkan config mode 0600 dengan LoadCredential.'
  systemctl is-system-running >/dev/null 2>&1 || [[ $(systemctl is-system-running 2>/dev/null || true) == degraded ]] || die 'systemd tidak siap.'
  [[ $(uname -m) == aarch64 || $(uname -m) == arm64 || $(uname -m) == x86_64 ]] || die "Arsitektur tidak didukung: $(uname -m)."
}
install_deps(){
  say 'Memasang dependensi ringan dan tool build.'
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y ca-certificates curl git golang-go iproute2 openssl sudo ufw
}
check_repo_url(){
  if [[ $REPO_URL == *'USER/REPO'* ]]; then
    local entered=
    read -r -p 'URL clone repo GitHub Anda (HTTPS, mis. https://github.com/owner/repo.git): ' entered || true
    [[ -n $entered ]] || die 'URL repo GitHub diperlukan untuk clone/update panel.'
    REPO_URL=$entered
  fi
  [[ $REPO_URL != *'USER/REPO'* ]] || die 'Atur XRAY_PANEL_REPO ke URL repo GitHub Anda, atau ganti USER/REPO di nilai default install.sh.'
  [[ $REPO_URL =~ ^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(\.git)?/?$ ]] || die 'XRAY_PANEL_REPO harus URL clone HTTPS GitHub berbentuk https://github.com/owner/repo.git.'
}
fetch_repo(){
  check_repo_url
  if [[ ! -e $APP_DIR ]]; then
    say "Clone repo ke $APP_DIR"
    git clone --depth 1 "$REPO_URL" "$APP_DIR"
  elif [[ -d $APP_DIR/.git ]]; then
    git -C "$APP_DIR" remote set-url origin "$REPO_URL"
    git -C "$APP_DIR" fetch --depth 1 origin
    git -C "$APP_DIR" pull --ff-only origin main || die 'Git pull gagal (mungkin branch bukan main atau ada perubahan lokal); periksa /opt/xray-panel.'
  else
    die "$APP_DIR sudah ada tetapi bukan clone Git. Pindahkan folder itu sendiri sebelum melanjutkan."
  fi
}
build_app(){
  say 'Build panel dan helper dengan konkurensi rendah.'
  command -v go >/dev/null || die 'Go compiler tidak tersedia setelah instalasi dependensi.'
  (cd "$APP_DIR" && GOMAXPROCS=1 GOFLAGS='-mod=vendor -p=1' go build -trimpath -ldflags='-s -w' -o /usr/local/bin/xray-panel .)
  (cd "$APP_DIR" && GOMAXPROCS=1 GOFLAGS='-mod=vendor -p=1' go build -trimpath -ldflags='-s -w' -o /usr/local/sbin/xray-panel-helper ./cmd/helper)
  chown root:root /usr/local/bin/xray-panel /usr/local/sbin/xray-panel-helper
  chmod 0755 /usr/local/bin/xray-panel /usr/local/sbin/xray-panel-helper
}
ensure_xray(){
  mkdir -p /usr/local/etc/xray
  if [[ -f $XRAY_CONFIG && ! -e $STATE_FILE ]]; then
    local backup="$XRAY_CONFIG.pre-xray-panel.$(date -u +%Y%m%dT%H%M%SZ)"
    cp -a -- "$XRAY_CONFIG" "$backup"
    chmod 0600 "$backup"
    say "Config lama disimpan: $backup"
  fi
  local installer
  installer=$(mktemp /tmp/xray-install-XXXXXX.sh)
  trap 'rm -f "${installer:-}"' RETURN
  curl -fsSL "$XRAY_INSTALL_URL" -o "$installer"
  chmod 0700 "$installer"
  say 'Memasang/memperbarui Xray melalui installer resmi XTLS.'
  bash "$installer" install --without-geodata --manually-restart-later
  rm -f "$installer"
  trap - RETURN
  write_xray_credential_override
  systemctl daemon-reload
}
write_xray_credential_override(){
  local dir=/etc/systemd/system/xray.service.d
  install -d -o root -g root -m 0755 "$dir"
  cat > "$dir/90-xray-panel-credentials.conf" <<'EOF'
[Service]
LoadCredential=config.json:/usr/local/etc/xray/config.json
ExecStart=
ExecStart=/usr/local/bin/xray run -config ${CREDENTIALS_DIRECTORY}/config.json
EOF
  chown root:root "$dir/90-xray-panel-credentials.conf"
  chmod 0644 "$dir/90-xray-panel-credentials.conf"
}
ask_port(){
  local label=$1 variable=$2 answer
  while :; do
    answer=
    if ! read -r -p "$label (1-65535): " answer; then die "Input untuk $label diperlukan."; fi
    if [[ -z $answer ]]; then printf 'Port wajib diisi; masukkan angka antara 1 dan 65535.\n'; continue; fi
    if [[ $answer =~ ^[0-9]{1,5}$ ]] && (( 10#$answer >= 1 && 10#$answer <= 65535 )); then printf -v "$variable" '%s' "$((10#$answer))";return;fi
    printf 'Port harus angka antara 1 dan 65535.\n'
  done
}
ask_port_different(){
  local label=$1 variable=$2 other_port=$3 selected
  while :; do
    ask_port "$label" "$variable"
    selected=${!variable}
    if [[ $selected != "$other_port" ]]; then return; fi
    printf 'Port harus berbeda dari port terkait; masukkan ulang.\n'
  done
}
port_in_use(){
  local port=$1 output
  output=$(ss -H -ltn "sport = :$port" 2>/dev/null || true)
  [[ -n $output ]]
}
check_new_ports(){
  local panel_internal=$1 xray_internal=$2
  [[ $panel_internal != "$xray_internal" ]] || die 'Port internal panel dan Xray harus berbeda.'
  for p in "$panel_internal" "$xray_internal"; do
    if port_in_use "$p"; then
      printf 'Port internal TCP %s sedang dipakai oleh service lain:\n' "$p" >&2
      ss -ltnp "sport = :$p" >&2 || true
      die 'Pilih port internal yang tidak sedang listen; instalasi dihentikan tanpa menimpa service itu.'
    fi
  done
}
ask_install_ports(){
  ask_port 'Port internal panel' PANEL_INTERNAL
  ask_port_different 'Port internal Xray' XRAY_INTERNAL "$PANEL_INTERNAL"
  check_new_ports "$PANEL_INTERNAL" "$XRAY_INTERNAL"
}
ask_password(){
  local p1 p2
  read -r -s -p 'Password admin (kosong = buat acak): ' p1 || true; printf '\n'
  if [[ -z $p1 ]]; then GENERATED_PASSWORD=$(openssl rand -hex 24); p1=$GENERATED_PASSWORD
  else
    (( ${#p1} >= 12 && ${#p1} <= 1024 )) || die 'Password admin harus 12–1024 karakter.'
    read -r -s -p 'Ulangi password admin: ' p2 || true; printf '\n'
    [[ $p1 == "$p2" ]] || die 'Password tidak sama.'
  fi
  ADMIN_PASSWORD=$p1
}
ask_tls(){
  local choice cert key
  read -r -p 'Aktifkan HTTPS langsung di panel? [y/N]: ' choice || true
  if [[ ${choice,,} != y && ${choice,,} != yes ]]; then TLS_CERT= TLS_KEY=; return; fi
  read -r -p 'Path sertifikat (atau ketik self-signed untuk membuat sertifikat lokal): ' cert || true
  if [[ $cert == self-signed ]]; then
    command -v openssl >/dev/null || die 'openssl diperlukan untuk membuat sertifikat.'
    mkdir -p /etc/xray-panel/tls
    chmod 0750 /etc/xray-panel /etc/xray-panel/tls
    openssl req -x509 -nodes -newkey rsa:2048 -days 825 -keyout /etc/xray-panel/tls/tls.key -out /etc/xray-panel/tls/tls.crt -subj "/CN=xray-panel.local" -addext "subjectAltName=DNS:xray-panel.local" >/dev/null 2>&1
    TLS_CERT=/etc/xray-panel/tls/tls.crt; TLS_KEY=/etc/xray-panel/tls/tls.key
  else
    [[ -f $cert ]] || die 'File sertifikat tidak ditemukan.'
    read -r -p 'Path private key: ' key || true
    [[ -f $key ]] || die 'File private key tidak ditemukan.'
    mkdir -p /etc/xray-panel/tls
    cp -- "$cert" /etc/xray-panel/tls/tls.crt
    cp -- "$key" /etc/xray-panel/tls/tls.key
    TLS_CERT=/etc/xray-panel/tls/tls.crt; TLS_KEY=/etc/xray-panel/tls/tls.key
  fi
  chown root:"$PANEL_USER" /etc/xray-panel /etc/xray-panel/tls "$TLS_CERT" "$TLS_KEY"
  chmod 0750 /etc/xray-panel /etc/xray-panel/tls
  chmod 0640 "$TLS_CERT" "$TLS_KEY"
  openssl x509 -in "$TLS_CERT" -noout >/dev/null 2>&1 || die 'Sertifikat TLS tidak dapat dibaca.'
  openssl pkey -in "$TLS_KEY" -passin pass: -noout >/dev/null 2>&1 || die 'Private key TLS tidak valid atau masih dienkripsi; panel memerlukan key tanpa passphrase.'
  local cert_pub key_pub
  cert_pub=$(openssl x509 -in "$TLS_CERT" -pubkey -noout | openssl pkey -pubin -outform DER 2>/dev/null | sha256sum | awk '{print $1}')
  key_pub=$(openssl pkey -in "$TLS_KEY" -passin pass: -pubout -outform DER 2>/dev/null | sha256sum | awk '{print $1}')
  [[ -n $cert_pub && $cert_pub == "$key_pub" ]] || die 'Private key TLS tidak cocok dengan sertifikat.'
}
write_service_config(){
  install -d -o root -g "$PANEL_USER" -m 0750 /etc/xray-panel
  {
    printf 'XRAY_PANEL_STATE=%s\n' "$STATE_FILE"
    printf 'PANEL_ADDR=0.0.0.0:%s\n' "$PANEL_INTERNAL"
    if [[ -n ${TLS_CERT:-} ]]; then printf 'PANEL_TLS_CERT=%s\nPANEL_TLS_KEY=%s\n' "$TLS_CERT" "$TLS_KEY"; fi
  } > /etc/xray-panel.env
  chown root:"$PANEL_USER" /etc/xray-panel.env
  chmod 0640 /etc/xray-panel.env
  install -m 0644 -o root -g root "$APP_DIR/systemd/xray-panel.service" /etc/systemd/system/xray-panel.service
  install -m 0440 -o root -g root "$APP_DIR/sudoers/xray-panel" /etc/sudoers.d/xray-panel
  visudo -cf /etc/sudoers.d/xray-panel >/dev/null || die 'File sudoers tidak valid.'
}
write_policy_files(){
  install -m 0644 -o root -g root "$APP_DIR/systemd/xray-panel.service" /etc/systemd/system/xray-panel.service
  install -m 0440 -o root -g root "$APP_DIR/sudoers/xray-panel" /etc/sudoers.d/xray-panel
  visudo -cf /etc/sudoers.d/xray-panel >/dev/null || die 'File sudoers tidak valid.'
  systemctl daemon-reload
}
start_services(){
  mkdir -p "$STATE_DIR"
  chown "$PANEL_USER":"$PANEL_USER" "$STATE_DIR"
  chmod 0700 "$STATE_DIR"
  systemctl daemon-reload
  systemctl enable xray-panel.service >/dev/null
  systemctl restart xray-panel.service
}
open_firewall(){
  if ufw status 2>/dev/null | grep -q '^Status: active'; then
    say 'UFW aktif: mengizinkan TCP pada port internal panel dan Xray.'
    ufw allow "$PANEL_INTERNAL/tcp" comment 'xray-panel internal panel' >/dev/null || true
    ufw allow "$XRAY_INTERNAL/tcp" comment 'xray-panel internal xray' >/dev/null || true
  fi
}
install_fresh(){
  require_root;check_platform;check_repo_url;install_deps;fetch_repo
  if [[ -f $STATE_FILE || -L $STATE_FILE ]]; then
    [[ -f $STATE_FILE && ! -L $STATE_FILE ]] || die 'State panel bukan file reguler yang aman; periksa /var/lib/xray-panel/state.json.'
    say 'State sudah ada; memakai jalur update untuk mempertahankannya.'
    [[ -f /etc/xray-panel.env ]] || die 'State panel ada tetapi /etc/xray-panel.env hilang; pulihkan konfigurasi env sebelum install ulang.'
    if ! id "$PANEL_USER" >/dev/null 2>&1; then useradd --system --home-dir "$STATE_DIR" --create-home --shell /usr/sbin/nologin "$PANEL_USER"; fi
    install -d -o "$PANEL_USER" -g "$PANEL_USER" -m 0700 "$STATE_DIR"
    chown "$PANEL_USER":"$PANEL_USER" "$STATE_FILE";chmod 0600 "$STATE_FILE"
    build_app;ensure_xray;write_policy_files
    /usr/local/bin/xray run -test -config "$XRAY_CONFIG"
    systemctl restart xray.service;systemctl enable xray-panel.service >/dev/null;systemctl restart xray-panel.service
    say 'Update selesai; konfigurasi, pengguna, key dan password dipertahankan.'
    return
  fi
  say 'Pengaturan awal. Masukkan dua port internal manual; hostname dan port publik panel serta endpoint Xray ditetapkan provider setelah pemetaan NAT tersedia.'
  ask_install_ports
  ask_password
  build_app
  ensure_xray
  if ! id "$PANEL_USER" >/dev/null 2>&1; then useradd --system --home-dir "$STATE_DIR" --create-home --shell /usr/sbin/nologin "$PANEL_USER"; fi
  install -d -o "$PANEL_USER" -g "$PANEL_USER" -m 0700 "$STATE_DIR"
  write_service_config
  ask_tls
  if [[ -n $TLS_CERT ]]; then write_service_config; fi
  printf '%s' "$ADMIN_PASSWORD" | /usr/local/bin/xray-panel init --state "$STATE_FILE" --panel-internal-port "$PANEL_INTERNAL" --xray-internal-port "$XRAY_INTERNAL"
  chown "$PANEL_USER":"$PANEL_USER" "$STATE_FILE"
  chmod 0600 "$STATE_FILE"
  if [[ -n $GENERATED_PASSWORD ]]; then printf '\nPASSWORD ADMIN (simpan sekarang; tidak akan ditampilkan lagi):\n%s\n\n' "$GENERATED_PASSWORD"; fi
  open_firewall
  start_services
  local scheme=http
  [[ -z $TLS_CERT ]] || scheme=https
  say 'Instalasi selesai.'
  printf 'Panel: %s://<hostname-panel>:<port-publik-acak-dari-provider>\n' "$scheme"
  printf 'Mapping panel provider: gunakan hostname:port publik yang provider tetapkan -> %s/TCP.\n' "$PANEL_INTERNAL"
  printf 'Port internal Xray: %s/TCP; masukkan hostname:port publik yang provider tetapkan untuk Xray di Pengaturan panel setelah detail mapping tersedia.\n' "$XRAY_INTERNAL"
  printf 'Batas provider: 1 mapping panel + maksimal 8 endpoint Xray (9 total).\n'
  printf 'Mode awal: VLESS + TCP + REALITY + Vision. UDP diblokir.\n'
  printf 'Port UFW internal yang diizinkan: %s/TCP dan %s/TCP (jika UFW aktif).\n' "$PANEL_INTERNAL" "$XRAY_INTERNAL"
  printf 'Periksa provider NAT: mapping panel ke %s/TCP dan setiap endpoint Xray ke %s/TCP; provider menetapkan hostname serta port publik acak, gunakan TCP.\n' "$PANEL_INTERNAL" "$XRAY_INTERNAL"
  if [[ -z $TLS_CERT ]]; then printf 'PERINGATAN: panel masih HTTP. Gunakan reverse proxy HTTPS atau sertifikat yang cocok dengan hostname panel dari provider.\n'; fi
}
update_existing(){
  require_root;check_platform;check_repo_url
  [[ -f $STATE_FILE && ! -L $STATE_FILE ]] || die 'Instalasi tidak ditemukan atau state bukan file reguler; periksa /var/lib/xray-panel/state.json.'
  install_deps;fetch_repo;build_app;ensure_xray;write_policy_files
  say 'Memvalidasi config lama dan me-restart Xray.'
  /usr/local/bin/xray run -test -config "$XRAY_CONFIG"
  systemctl restart xray.service
  systemctl restart xray-panel.service
  say 'Update selesai; state, pengguna, key, password, port dan pengaturan panel dipertahankan.'
}
remove_panel_ufw_rules(){
  command -v ufw >/dev/null 2>&1 || return 0
  local number
  while :; do
    number=$(ufw status numbered 2>/dev/null | grep -E '# xray-panel internal (panel|xray)( |$)' | sed -nE 's/^[[:space:]]*\[[[:space:]]*([0-9]+)\].*/\1/p' | head -n 1)
    [[ -n $number ]] || break
    ufw --force delete "$number" >/dev/null || break
  done
}
uninstall_panel(){
  require_root;check_platform
  [[ -d /etc/systemd/system ]] || die 'systemd tidak tersedia.'
  printf 'PURGE TOTAL: panel, Xray, konfigurasi, key/user, sertifikat TLS, state/backup lokal, akun service, clone source, dan aturan UFW bertanda xray-panel akan DIHAPUS PERMANEN. Paket OS umum dan riwayat jurnal systemd bersama tetap ada.\n'
  read -r -p 'Lanjutkan? ketik hapus-total: ' confirm || true
  [[ $confirm == hapus-total ]] || die 'Dibatalkan; tidak ada perubahan.'

  local has_xray_unit=0 remover=
  if systemctl list-unit-files --type=service --no-legend 2>/dev/null | awk '$1 == "xray.service" || $1 == "xray@.service" { found=1 } END { exit !found }'; then has_xray_unit=1; fi
  if (( has_xray_unit )); then
    remover=$(mktemp /tmp/xray-remove-XXXXXX.sh)
    if ! curl -fsSL "$XRAY_INSTALL_URL" -o "$remover"; then
      rm -f "$remover"
      die 'Tidak dapat mengambil uninstaller resmi XTLS; tidak ada komponen yang dihapus.'
    fi
    if ! bash "$remover" remove --purge; then
      rm -f "$remover"
      die 'Uninstaller resmi XTLS gagal. Komponen panel belum dibersihkan; periksa status Xray sebelum mencoba lagi.'
    fi
    rm -f "$remover"
  else
    systemctl disable --now xray.service >/dev/null 2>&1 || true
    rm -f /usr/local/bin/xray /etc/systemd/system/xray.service /etc/systemd/system/xray@.service /etc/logrotate.d/xray
    rm -rf /usr/local/share/xray /var/log/xray /etc/systemd/system/xray.service.d /etc/systemd/system/xray@.service.d
  fi

  systemctl disable --now "$PANEL_UNIT" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/$PANEL_UNIT" /etc/sudoers.d/xray-panel /usr/local/bin/xray-panel /usr/local/sbin/xray-panel-helper
  rm -f /etc/xray-panel.env /etc/systemd/system/xray.service.d/90-xray-panel-credentials.conf "$XRAY_CONFIG"
  rmdir /etc/systemd/system/xray.service.d /etc/systemd/system/xray@.service.d 2>/dev/null || true
  rm -rf /etc/xray-panel "$STATE_DIR" "$APP_DIR"
  find /var/backups -maxdepth 1 -mindepth 1 -type d -name 'xray-panel-uninstalled-*' -exec rm -rf -- {} + 2>/dev/null || true
  shopt -s nullglob
  local config_backups=(/usr/local/etc/xray/config.json.pre-xray-panel.*)
  ((${#config_backups[@]} == 0)) || rm -f -- "${config_backups[@]}"
  shopt -u nullglob
  rmdir /usr/local/etc/xray 2>/dev/null || true
  remove_panel_ufw_rules
  if id "$PANEL_USER" >/dev/null 2>&1; then userdel --remove "$PANEL_USER" >/dev/null 2>&1 || true; fi
  if getent group "$PANEL_USER" >/dev/null 2>&1; then groupdel "$PANEL_USER" >/dev/null 2>&1 || true; fi
  systemctl daemon-reload
  systemctl reset-failed "$PANEL_UNIT" xray.service >/dev/null 2>&1 || true
  say 'Uninstall total selesai: panel, Xray, data/config, dan aturan UFW milik panel telah dihapus.'
  printf 'Paket OS umum (mis. curl, git, Go, sudo, dan UFW) tetap terpasang agar software lain di VPS tidak terganggu.\n'
}

main(){
  require_root
  case ${1:-install} in
    install) install_fresh ;;
    update) update_existing ;;
    uninstall) uninstall_panel ;;
    *) printf 'Pemakaian: install.sh [install|update|uninstall]\n'; exit 2 ;;
  esac
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
  main "$@"
fi
