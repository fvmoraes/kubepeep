#!/bin/sh
set -eu

test_root=$(mktemp -d)
cleanup() {
	rm -rf -- "$test_root"
}
trap cleanup EXIT HUP INT TERM

release_dir=$test_root/release
payload_dir=$test_root/payload
fake_bin=$test_root/fake-bin
test_home=$test_root/home
mkdir -p "$release_dir" "$payload_dir" "$fake_bin" "$test_home/.kubePeep"
printf 'preserve-me\n' >"$test_home/.kubePeep/sentinel"

cat >"$payload_dir/kubePeep" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version) printf '%s\n' 'version=0.1.0 commit=synthetic build_date=2026-08-17T00:00:00Z' ;;
  status) exit 3 ;;
  *) exit 0 ;;
esac
EOF
chmod 755 "$payload_dir/kubePeep"
archive=kubepeep-linux-amd64.tar.gz
publish_payload() {
	tar -czf "$release_dir/$archive" -C "$payload_dir" kubePeep
	checksum=$(sha256sum "$release_dir/$archive" | awk '{print $1}')
	printf '%s  %s\n' "$checksum" "$archive" >"$release_dir/checksums.txt"
}
publish_payload

cat >"$fake_bin/curl" <<'EOF'
#!/bin/sh
set -eu
output=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) output=$2; shift 2 ;;
    https://*) url=$1; shift ;;
    *) shift ;;
  esac
done
[ -n "$output" ] && [ -n "$url" ]
[ -z "${KUBEPEEP_DOWNLOAD_MARKER:-}" ] || printf 'downloaded\n' >>"$KUBEPEEP_DOWNLOAD_MARKER"
cp "$KUBEPEEP_FAKE_RELEASE/${url##*/}" "$output"
EOF
chmod 755 "$fake_bin/curl"

cat >"$fake_bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
  -s) printf '%s\n' Linux ;;
  -m) printf '%s\n' "${KUBEPEEP_FAKE_ARCH:-amd64}" ;;
  *) printf '%s\n' Linux ;;
esac
EOF
chmod 755 "$fake_bin/uname"

test_path=$fake_bin:$PATH
KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null
installed=$test_home/.local/bin/kubePeep
[ -x "$installed" ]
"$installed" version | grep -F 'version=0.1.0' >/dev/null

transaction_download_marker=$test_root/transaction-downloaded
mkdir "$test_home/.local/bin/.kubePeep.install.lock"
if KUBEPEEP_DOWNLOAD_MARKER=$transaction_download_marker KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer ignored a concurrent transaction lock' >&2
	exit 1
fi
[ ! -e "$transaction_download_marker" ] || {
	echo 'installer downloaded release assets before acquiring its transaction lock' >&2
	exit 1
}
rmdir "$test_home/.local/bin/.kubePeep.install.lock"

cat >"$installed" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version) printf '%s\n' 'version=0.0.9 commit=synthetic build_date=2026-08-17T00:00:00Z' ;;
  status) exit 3 ;;
  *) exit 0 ;;
esac
EOF
chmod 755 "$installed"
publish_payload
cat >"$fake_bin/mv" <<'EOF'
#!/bin/sh
set -eu
source_path=
target_path=
for argument in "$@"; do
  case "$argument" in
    -*) ;;
    *)
      [ -n "$source_path" ] || source_path=$argument
      target_path=$argument
      ;;
  esac
done
case "$source_path" in
  */.kubePeep.install.*)
    if [ -n "${KUBEPEEP_ATOMIC_TARGET:-}" ] && [ "$target_path" = "$KUBEPEEP_ATOMIC_TARGET" ]; then
      [ -f "$target_path" ] || exit 86
    fi
    ;;
  */.kubePeep.backup.*)
    if [ "${KUBEPEEP_FAIL_ROLLBACK:-}" = 1 ] && [ -n "${KUBEPEEP_ATOMIC_TARGET:-}" ] && [ "$target_path" = "$KUBEPEEP_ATOMIC_TARGET" ]; then
      exit 87
    fi
    ;;
esac
exec /bin/mv "$@"
EOF
chmod 755 "$fake_bin/mv"
KUBEPEEP_ATOMIC_TARGET=$installed KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null
"$installed" version | grep -F 'version=0.1.0' >/dev/null
[ "$(grep -F -c '# >>> kubePeep installer PATH >>>' "$test_home/.profile")" -eq 1 ]

if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 01.1.0 >/dev/null 2>&1; then
	echo 'installer accepted a release version with a leading zero' >&2
	exit 1
fi
"$installed" version | grep -F 'version=0.1.0' >/dev/null

publish_payload
dd if=/dev/zero of="$release_dir/checksums.txt" bs=1048577 count=1 2>/dev/null
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer accepted an oversized checksum list' >&2
	exit 1
fi
"$installed" version | grep -F 'version=0.1.0' >/dev/null

publish_payload
dd if=/dev/zero of="$release_dir/$archive" bs=1 count=0 seek=268435457 2>/dev/null
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer accepted an oversized release archive' >&2
	exit 1
fi
"$installed" version | grep -F 'version=0.1.0' >/dev/null

publish_payload
mv "$release_dir/$archive" "$release_dir/$archive.saved"
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer accepted a missing archive' >&2
	exit 1
fi
mv "$release_dir/$archive.saved" "$release_dir/$archive"
"$installed" version | grep -F 'version=0.1.0' >/dev/null

tar -czf "$release_dir/$archive" -C "$payload_dir" kubePeep kubePeep
checksum=$(sha256sum "$release_dir/$archive" | awk '{print $1}')
printf '%s  %s\n' "$checksum" "$archive" >"$release_dir/checksums.txt"
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer accepted duplicate binary entries' >&2
	exit 1
fi
"$installed" version | grep -F 'version=0.1.0' >/dev/null

tar -czf "$release_dir/$archive" --transform='s|^kubePeep$|../kubePeep|' -C "$payload_dir" kubePeep
checksum=$(sha256sum "$release_dir/$archive" | awk '{print $1}')
printf '%s  %s\n' "$checksum" "$archive" >"$release_dir/checksums.txt"
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer accepted an archive traversal path' >&2
	exit 1
fi
"$installed" version | grep -F 'version=0.1.0' >/dev/null

cat >"$payload_dir/kubePeep" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version) printf '%s\n' 'version=0.1.0suffix commit=synthetic build_date=2026-08-17T00:00:00Z' ;;
  status) exit 3 ;;
  *) exit 0 ;;
esac
EOF
chmod 755 "$payload_dir/kubePeep"
publish_payload
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer accepted an inexact candidate version token' >&2
	exit 1
fi
"$installed" version | grep -F 'version=0.1.0' >/dev/null

cat >"$payload_dir/kubePeep" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version)
    case "$0" in
      */.local/bin/*) printf '%s\n' 'version=9.9.9 commit=synthetic build_date=2026-08-17T00:00:00Z' ;;
      *) printf '%s\n' 'version=0.1.0 commit=synthetic build_date=2026-08-17T00:00:00Z' ;;
    esac
    ;;
  status) exit 3 ;;
  *) exit 0 ;;
esac
EOF
chmod 755 "$payload_dir/kubePeep"
publish_payload
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer did not fail its post-install version check' >&2
	exit 1
fi
"$installed" version | grep -F 'version=0.1.0' >/dev/null
grep -F '# >>> kubePeep installer PATH >>>' "$test_home/.profile" >/dev/null

if KUBEPEEP_FAIL_ROLLBACK=1 KUBEPEEP_ATOMIC_TARGET=$installed KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer reported success when rollback could not complete' >&2
	exit 1
fi
rollback_backup=$(find "$test_home/.local/bin" -maxdepth 1 -type f -name '.kubePeep.backup.*')
[ -n "$rollback_backup" ] && [ "$(printf '%s\n' "$rollback_backup" | wc -l | tr -d '[:space:]')" -eq 1 ] || {
	echo 'installer did not preserve exactly one recovery binary after rollback failure' >&2
	exit 1
}
"$rollback_backup" version | grep -F 'version=0.1.0' >/dev/null
/bin/mv -f "$rollback_backup" "$installed"

printf '%064d  %s\n' 0 "$archive" >"$release_dir/checksums.txt"
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer accepted an invalid checksum' >&2
	exit 1
fi
"$installed" version | grep -F 'version=0.1.0' >/dev/null

publish_payload
cat "$release_dir/checksums.txt" >>"$release_dir/checksums.txt.duplicate"
cat "$release_dir/checksums.txt" >>"$release_dir/checksums.txt.duplicate"
mv "$release_dir/checksums.txt.duplicate" "$release_dir/checksums.txt"
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer accepted duplicate checksum entries' >&2
	exit 1
fi
"$installed" version | grep -F 'version=0.1.0' >/dev/null

custom_install_dir=$test_root/custom-bin
HOME=$test_home PATH=$test_path ./install.sh --uninstall --install-dir "$custom_install_dir" >/dev/null
[ -x "$installed" ]
grep -F '# >>> kubePeep installer PATH >>>' "$test_home/.profile" >/dev/null || {
	echo 'custom-directory uninstall removed the default installation PATH block' >&2
	exit 1
}

HOME=$test_home PATH=$test_path ./install.sh --uninstall >/dev/null
[ ! -e "$installed" ]
[ -f "$test_home/.kubePeep/sentinel" ]
if grep -F '# >>> kubePeep installer PATH >>>' "$test_home/.profile" >/dev/null; then
	echo 'managed PATH block survived uninstall' >&2
	exit 1
fi

mkdir "$test_home/.local/bin/.kubePeep.install.lock"
if HOME=$test_home PATH=$test_path ./install.sh --uninstall >/dev/null 2>&1; then
	echo 'installer ignored a concurrent install transaction lock' >&2
	exit 1
fi
rmdir "$test_home/.local/bin/.kubePeep.install.lock"

symlink_probe=$test_root/symlink-probe
symlink_marker=$test_root/symlink-executed
cat >"$symlink_probe" <<'EOF'
#!/bin/sh
printf 'executed\n' >"$KUBEPEEP_SYMLINK_MARKER"
exit 3
EOF
chmod 755 "$symlink_probe"
ln -s "$symlink_probe" "$installed"
if KUBEPEEP_SYMLINK_MARKER=$symlink_marker HOME=$test_home PATH=$test_path \
	./install.sh --uninstall >/dev/null 2>&1; then
	echo 'installer removed a symlinked binary' >&2
	exit 1
fi
[ ! -e "$symlink_marker" ] || {
	echo 'installer executed a symlinked binary before rejecting it' >&2
	exit 1
}
rm "$installed"

cat >"$test_home/.profile" <<'EOF'
preserve-before
# >>> kubePeep installer PATH >>>
preserve-after-malformed-marker
EOF
profile_before=$(sha256sum "$test_home/.profile" | awk '{print $1}')
if HOME=$test_home PATH=$test_path ./install.sh --uninstall >/dev/null 2>&1; then
	echo 'installer accepted malformed managed PATH markers' >&2
	exit 1
fi
profile_after=$(sha256sum "$test_home/.profile" | awk '{print $1}')
[ "$profile_after" = "$profile_before" ] || {
	echo 'installer changed .profile after rejecting malformed markers' >&2
	exit 1
}
printf 'preserve-profile\n' >"$test_home/.profile"

preserved_data=$test_home/.kubePeep.preserved
outside_data=$test_root/outside-data
mv "$test_home/.kubePeep" "$preserved_data"
mkdir -p "$outside_data"
printf 'outside-must-survive\n' >"$outside_data/sentinel"
ln -s "$outside_data" "$test_home/.kubePeep"
if HOME=$test_home PATH=$test_path ./install.sh --uninstall --purge-data --confirm-purge >/dev/null 2>&1; then
	echo 'installer purged through a symlinked data root' >&2
	exit 1
fi
[ -f "$outside_data/sentinel" ]
rm "$test_home/.kubePeep"
mv "$preserved_data" "$test_home/.kubePeep"

mkdir -p "$test_home/.kubePeep/runtime"
: >"$test_home/.kubePeep/runtime/kubePeep.lock"
if HOME=$test_home PATH=$test_path ./install.sh --uninstall --purge-data --confirm-purge >/dev/null 2>&1; then
	echo 'installer purged data while the runtime lock existed' >&2
	exit 1
fi
rm "$test_home/.kubePeep/runtime/kubePeep.lock"

mkdir -p "$installed"
if KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer replaced a non-regular binary path' >&2
	exit 1
fi
[ -d "$installed" ]
rmdir "$installed"

HOME=$test_home PATH=$test_path ./install.sh --uninstall --purge-data --confirm-purge >/dev/null
[ ! -e "$test_home/.kubePeep" ]

if KUBEPEEP_FAKE_ARCH=riscv64 KUBEPEEP_FAKE_RELEASE=$release_dir HOME=$test_home PATH=$test_path \
	./install.sh --version 0.1.0 >/dev/null 2>&1; then
	echo 'installer accepted an unsupported architecture' >&2
	exit 1
fi

echo 'install.sh tests passed'
