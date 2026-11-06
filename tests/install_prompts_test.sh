#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/install.sh"
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

# Installer meminta tepat dua port internal. Input kosong/tidak valid diulang,
# dan port panel serta Xray harus berbeda.
port_in_use(){ return 1; }
input=$'\nnot-a-port\n8080\n\n8080\n8080\n8443'
ask_install_ports < <(printf '%s\n' "$input") >"$tmpdir/ports.log" 2>&1
[[ $PANEL_INTERNAL == 8080 && $XRAY_INTERNAL == 8443 ]] || {
  echo "unexpected internal port values: panel=$PANEL_INTERNAL xray=$XRAY_INTERNAL" >&2
  exit 1
}
[[ $(grep -c 'Port wajib diisi' "$tmpdir/ports.log") -eq 2 ]] || {
  echo 'blank port input did not reprompt twice' >&2
  cat "$tmpdir/ports.log" >&2
  exit 1
}
grep -q 'Port harus angka antara 1 dan 65535' "$tmpdir/ports.log"
grep -q 'Port harus berbeda dari port terkait' "$tmpdir/ports.log"
grep -Fq "ask_port 'Port internal panel' PANEL_INTERNAL" "$ROOT/install.sh"
grep -Fq 'ask_port_different '\''Port internal Xray'\'' XRAY_INTERNAL' "$ROOT/install.sh"
if grep -Eiq 'Port publik|Port public|port eksternal|external port|jumlah endpoint|endpoint count|host:port|endpoint per baris' "$tmpdir/ports.log"; then
  echo 'installer unexpectedly asked for an external endpoint or port' >&2
  exit 1
fi
if grep -Eq "PANEL_PUBLIC(_PORT)?|panel-public-port|ask_port 'Port publik|Port publik Xray #" "$ROOT/install.sh"; then
  echo 'installer still contains a public-port input' >&2
  exit 1
fi
if grep -Eq 'panel-public-port|panelPublic' "$ROOT/panel.go"; then
  echo 'application init still accepts an external public panel port' >&2
  exit 1
fi
if ! grep -Fq -- '--panel-internal-port "$PANEL_INTERNAL" --xray-internal-port "$XRAY_INTERNAL"' "$ROOT/install.sh"; then
  echo 'installer does not pass both required internal ports to init' >&2
  exit 1
fi
if grep -Eiq 'NAT|provider|max(imal)?[[:space:]]*[89]|[89][[:space:]]+total|[89][[:space:]]+mapping|[89][[:space:]]+endpoint' "$ROOT/README.md" "$ROOT"/docs/*.md "$ROOT/install.sh"; then
  echo 'public docs or installer reintroduced provider-specific wording or fixed endpoint counts' >&2
  exit 1
fi
if grep -Eiq 'vectis-nat|nat-[0-9]|provider\.example' "$ROOT/config_test.go"; then
  echo 'a provider-specific example hostname remains in test fixtures' >&2
  exit 1
fi
# Optional public host:port endpoints are entered later in the panel.

echo 'All internal installer port prompt tests passed.'
