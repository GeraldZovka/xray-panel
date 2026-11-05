#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/install.sh"
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

# The installer asks for exactly three manual port values. Blanks and invalid
# values reprompt; only the two local listener ports must differ. The public
# panel port is allowed to numerically match its internal port.
port_in_use(){ return 1; }
input=$'\nnot-a-port\n8080\n\n8080\n8080\n8443'
ask_install_ports < <(printf '%s\n' "$input") >"$tmpdir/ports.log" 2>&1
[[ $PANEL_PUBLIC == 8080 && $PANEL_INTERNAL == 8080 && $XRAY_INTERNAL == 8443 ]] || {
  echo "unexpected port values: public=$PANEL_PUBLIC panel-internal=$PANEL_INTERNAL xray-internal=$XRAY_INTERNAL" >&2
  exit 1
}
[[ $(grep -c 'Port wajib diisi' "$tmpdir/ports.log") -eq 2 ]] || {
  echo 'blank port input did not reprompt twice' >&2
  cat "$tmpdir/ports.log" >&2
  exit 1
}
grep -q 'Port harus angka antara 1 dan 65535' "$tmpdir/ports.log"
grep -q 'Port harus berbeda dari port terkait' "$tmpdir/ports.log"
if grep -Eiq 'Jumlah mapping NAT Xray|Port publik Xray|mapping NAT Xray #|hostname:port' "$tmpdir/ports.log"; then
  echo 'installer asked for a NAT mapping count, public Xray port, or hostname' >&2
  exit 1
fi
if grep -Eq 'ask_nat_mappings|Jumlah mapping NAT Xray|Port publik Xray #' "$ROOT/install.sh"; then
  echo 'installer still contains a public Xray NAT prompt' >&2
  exit 1
fi

# Public Xray endpoints are configured later in the panel. Their public port
# may match the internal port when the provider supports that mapping.

echo 'All installer prompt tests passed.'
