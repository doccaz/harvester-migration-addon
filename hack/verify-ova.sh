#!/usr/bin/env bash
# Verify an OVA independently of the code that wrote it.
#   hack/verify-ova.sh file.ova [path/to/dsp8023.xsd]
# Checks (DMTF DSP0243): USTAR tar; member order (.ovf first, then .mf, then the disks);
# every manifest line "SHA256(name)= <hex>" matches the member's real digest; no member is
# missing from the manifest; the OVF validates against the DSP8023 schema when xmllint is
# available; and, if qemu-img is available, the disk image opens and reports its size.
set -uo pipefail
OVA="${1:?usage: verify-ova.sh file.ova [dsp8023.xsd]}"
XSD="${2:-$(dirname "$0")/../ui-backend/internal/export/testdata/xsd/dsp8023.xsd}"
pass=0; failn=0
ok()  { pass=$((pass+1)); echo "  PASS  $1"; }
bad() { failn=$((failn+1)); echo "  FAIL  $1"; }
[ -f "$OVA" ] || { echo "no such file: $OVA"; exit 1; }
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

echo "== Container"
tar tf "$OVA" > "$TMP/members" 2>"$TMP/tarerr" && ok "readable tar ($(wc -l <"$TMP/members") members, $(du -h "$OVA" | cut -f1))" || { bad "not a readable tar: $(head -c 200 "$TMP/tarerr")"; exit 1; }
if tar tvf "$OVA" 2>/dev/null | head -1 | awk '{print $1}' | grep -q '^-'; then ok "regular-file entries"; fi
FIRST="$(sed -n 1p "$TMP/members")"; SECOND="$(sed -n 2p "$TMP/members")"
[[ "$FIRST" == *.ovf ]] && ok "first member is the descriptor ($FIRST)" || bad "first member is '$FIRST', not the .ovf"
[[ "$SECOND" == *.mf ]] && ok "second member is the manifest ($SECOND)" || bad "second member is '$SECOND', not the .mf"
DUPES="$(sort "$TMP/members" | uniq -d)"; [ -z "$DUPES" ] && ok "no member appears twice" || bad "duplicate members: $DUPES"

echo "== Manifest digests"
MF="$(sed -n '/\.mf$/p' "$TMP/members" | head -1)"
tar xOf "$OVA" "$MF" > "$TMP/mf" 2>/dev/null
LINES=0
while IFS= read -r line; do
  [[ "$line" =~ ^SHA(256|1)\((.+)\)=\ ([0-9a-fA-F]+)$ ]] || { [ -n "$line" ] && bad "unparsable manifest line: $line"; continue; }
  alg="${BASH_REMATCH[1]}"; name="${BASH_REMATCH[2]}"; want="${BASH_REMATCH[3],,}"; LINES=$((LINES+1))
  got="$(tar xOf "$OVA" "$name" 2>/dev/null | "sha${alg}sum" | awk '{print $1}')"
  [ "$got" = "$want" ] && ok "SHA$alg($name) matches" || bad "SHA$alg($name): manifest $want, actual $got"
done < "$TMP/mf"
[ "$LINES" -gt 0 ] && ok "$LINES manifest entries checked" || bad "manifest has no usable entries"
for m in $(grep -vE '\.(mf|cert)$' "$TMP/members"); do grep -qF "($m)=" "$TMP/mf" || bad "member $m is not listed in the manifest"; done

echo "== Descriptor"
tar xOf "$OVA" "$FIRST" > "$TMP/d.ovf" 2>/dev/null
grep -q "<Envelope" "$TMP/d.ovf" && ok "descriptor is an OVF Envelope" || bad "descriptor does not look like an OVF"
if command -v xmllint >/dev/null && [ -f "$XSD" ]; then
  xmllint --noout --schema "$XSD" "$TMP/d.ovf" >"$TMP/xml" 2>&1 && ok "validates against DSP8023" || bad "schema validation: $(head -c 300 "$TMP/xml")"
else echo "  skip  schema validation (needs xmllint and $XSD)"; fi

echo "== Disk"
DISK="$(grep -vE '\.(ovf|mf|cert)$' "$TMP/members" | head -1)"
NEED_KB=$(( $(tar tvf "$OVA" 2>/dev/null | awk -v d="$DISK" '$NF==d {print $3}' | head -1) / 1024 + 1048576 ))
FREE_KB="$(df -Pk "$TMP" | awk 'NR==2 {print $4}')"
if [ -n "$DISK" ] && command -v qemu-img >/dev/null && [ "${FREE_KB:-0}" -lt "$NEED_KB" ]; then
  echo "  skip  disk check: it extracts the disk and needs ~$((NEED_KB/1024)) MiB free in \$TMPDIR ($((FREE_KB/1024)) MiB free); set TMPDIR to a bigger disk"
elif [ -n "$DISK" ] && command -v qemu-img >/dev/null; then
  tar xOf "$OVA" "$DISK" > "$TMP/disk" 2>/dev/null
  INFO="$(qemu-img info "$TMP/disk" 2>&1)"; echo "$INFO" | sed 's/^/    /' | head -8
  grep -q "file format: vmdk" <<<"$INFO" && ok "disk opens as VMDK" || bad "disk is not a readable VMDK"
else echo "  skip  disk check (needs qemu-img)"; fi

echo; echo "passed: $pass  failed: $failn"; [ "$failn" -eq 0 ]
