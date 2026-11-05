#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/install.sh"
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

# Empty, nonnumeric, out-of-range, and valid values: only the manual number wins.
exec 9<<<$'\nabc\n0\n65536\n20151\n'
ask_port 'Port publik panel' TEST_PANEL_PUBLIC <&9 >"$tmpdir/port.log" 2>&1
exec 9<&-
[[ $TEST_PANEL_PUBLIC == 20151 ]] || { echo "port prompt returned '$TEST_PANEL_PUBLIC', expected 20151" >&2; exit 1; }
grep -q 'Port wajib diisi' "$tmpdir/port.log"
grep -q 'Port harus angka' "$tmpdir/port.log"

# Empty/invalid counts and blank/nonnumeric/conflicting/duplicate Xray ports reprompt.
PANEL_PUBLIC=20151
input=$'\n0\n9\n2\n\nbad-port\n20151\n20152\n20152\n20153'
ask_nat_mappings < <(printf '%s\n' "$input") >"$tmpdir/mappings.log" 2>&1
[[ ${#NAT_MAPPING_LIST[@]} -eq 2 ]] || { echo "got ${#NAT_MAPPING_LIST[@]} Xray ports, expected 2" >&2; exit 1; }
[[ ${NAT_MAPPING_LIST[0]} == 20152 && ${NAT_MAPPING_LIST[1]} == 20153 ]] || { echo "unexpected Xray ports: ${NAT_MAPPING_LIST[*]}" >&2; exit 1; }
[[ $XRAY_NAT_MAPPINGS == 20152,20153 && $XRAY_FIRST_PORT == 20152 && -z $XRAY_FIRST_HOST ]] || { echo 'numeric port list or pending-host state is incorrect' >&2; exit 1; }
grep -q 'Jumlah mapping Xray harus angka' "$tmpdir/mappings.log"
grep -q 'Port wajib diisi' "$tmpdir/mappings.log"
grep -q 'Port publik Xray harus berbeda' "$tmpdir/mappings.log"
grep -q 'Port publik Xray duplikat' "$tmpdir/mappings.log"

# Equal panel/Xray internal ports reprompt until distinct.
exec 9<<<$'8080\n8443\n'
ask_port_different 'Port internal Xray' TEST_XRAY_INTERNAL 8080 <&9 >"$tmpdir/internal.log" 2>&1
exec 9<&-
[[ $TEST_XRAY_INTERNAL == 8443 ]] || { echo 'internal port conflict was not reprompted' >&2; exit 1; }

# The accepted maximum is 8 Xray mappings plus the one panel mapping (9 total).
input=$'8\n'
for port in {30001..30008}; do input+=$'\n'"$port"; done
ask_nat_mappings < <(printf '%s\n' "$input") >"$tmpdir/max.log" 2>&1
[[ ${#NAT_MAPPING_LIST[@]} -eq 8 && ${NAT_MAPPING_LIST[7]} == 30008 ]] || { echo 'eight Xray mappings were not accepted' >&2; exit 1; }
if grep -Eiq 'vectis-nat-|host:port|hostname.*:' "$tmpdir/port.log" "$tmpdir/mappings.log" "$tmpdir/max.log"; then
  echo 'installer asked for or supplied a hostname or host:port' >&2
  exit 1
fi
if grep -Eq '\[[0-9]{4,5}\]' "$tmpdir/port.log" "$tmpdir/mappings.log" "$tmpdir/max.log"; then
  echo 'a default port appeared in installer prompts' >&2
  exit 1
fi
echo 'All installer prompt tests passed.'
