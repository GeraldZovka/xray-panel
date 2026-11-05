#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$ROOT/install.sh"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

# Empty, out-of-range, and valid input: only the manually entered value wins.
exec 9<<<$'\n0\n65536\n20151\n'
ask_port 'Port publik panel' TEST_PANEL_PUBLIC <&9 >"$tmpdir/port.log" 2>&1
exec 9<&-
[[ $TEST_PANEL_PUBLIC == 20151 ]] || { echo "port prompt returned '$TEST_PANEL_PUBLIC', expected 20151" >&2; exit 1; }
grep -q 'Port wajib diisi' "$tmpdir/port.log"
grep -q 'Port harus angka' "$tmpdir/port.log"

# Blank and out-of-range counts reprompt; a blank endpoint and duplicate endpoint
# also reprompt. Eight unique Xray endpoints plus the panel stay within 9 total.
input=$'\n9\n8\n\nbad-endpoint\nnat-1.example.net:20151\nnat-1.example.net:20151\nnat-2.example.net:20152'
for i in {3..8}; do
  input+=$'\n'nat-${i}.example.net:$((20150 + i))
done
ask_nat_mappings < <(printf '%s\n' "$input") >"$tmpdir/mappings.log" 2>&1

[[ ${#NAT_MAPPING_LIST[@]} -eq 8 ]] || { echo "got ${#NAT_MAPPING_LIST[@]} Xray mappings, expected 8" >&2; exit 1; }
[[ ${NAT_MAPPING_LIST[0]} == nat-1.example.net:20151 ]] || { echo 'first mapping was not the manually entered endpoint' >&2; exit 1; }
[[ ${NAT_MAPPING_LIST[7]} == nat-8.example.net:20158 ]] || { echo 'eighth mapping was not accepted' >&2; exit 1; }
[[ $XRAY_NAT_MAPPINGS == nat-1.example.net:20151,nat-2.example.net:20152,nat-3.example.net:20153,nat-4.example.net:20154,nat-5.example.net:20155,nat-6.example.net:20156,nat-7.example.net:20157,nat-8.example.net:20158 ]] || { echo "unexpected mapping list: $XRAY_NAT_MAPPINGS" >&2; exit 1; }
grep -q 'Jumlah mapping wajib diisi' "$tmpdir/mappings.log"
grep -q 'Jumlah mapping Xray harus angka' "$tmpdir/mappings.log"
grep -q 'Mapping host:port wajib diisi' "$tmpdir/mappings.log"
grep -q 'Format mapping NAT tidak valid' "$tmpdir/mappings.log"
grep -q 'Mapping NAT duplikat' "$tmpdir/mappings.log"
if grep -Eq 'vectis-nat-|\[[0-9]{4,5}\]' "$tmpdir/port.log" "$tmpdir/mappings.log"; then
  echo 'a sample mapping or default port appeared in installer prompts' >&2
  exit 1
fi

echo 'All installer prompt tests passed.'
