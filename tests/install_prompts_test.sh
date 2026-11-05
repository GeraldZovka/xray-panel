#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/install.sh"
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

# The installer asks for exactly two manual internal listener ports. Blanks
# and invalid values reprompt, and the panel and Xray ports must differ.
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
if grep -Eiq 'Port publik|Port public|port eksternal|jumlah mapping|jumlah endpoint|mapping NAT Xray #|hostname:port|endpoint per baris' "$tmpdir/ports.log"; then
  echo 'installer asked for an external port, endpoint count, or mapping list' >&2
  exit 1
fi
if grep -Eq "PANEL_PUBLIC(_PORT)?|panel-public-port|ask_nat_mappings|ask_port 'Port publik|Port publik Xray #" "$ROOT/install.sh"; then
  echo 'installer still contains a public-port or provider-mapping input' >&2
  exit 1
fi
if grep -Eq 'panel-public-port|panelPublic' "$ROOT/panel.go"; then
  echo 'application init still accepts a provider-assigned public panel port' >&2
  exit 1
fi
if ! grep -Fq -- '--panel-internal-port "$PANEL_INTERNAL" --xray-internal-port "$XRAY_INTERNAL"' "$ROOT/install.sh"; then
  echo 'installer does not pass both required internal ports to init' >&2
  exit 1
fi
# Provider-assigned Xray hostname:port endpoints are entered later in panel settings.

echo 'All internal installer port prompt tests passed.'
