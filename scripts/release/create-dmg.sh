#!/usr/bin/env bash
# Build a sized, writable image first; never rely on -srcfolder auto-sizing.
set -euo pipefail

if [ "$#" -ne 2 ] || [ ! -d "$1/Contents/MacOS" ]; then
	echo 'usage: create-dmg.sh APP.app OUTPUT.dmg' >&2
	exit 2
fi
app=$(cd "$1" && pwd)
mkdir -p "$(dirname "$2")"
output=$(cd "$(dirname "$2")" && pwd)/$(basename "$2")
work=$(mktemp -d "${TMPDIR:-/tmp}/kubepeep-dmg.XXXXXX")
mountpoint=$work/mounted
mounted=false
stage=capacity

cleanup() {
	status=$?
	trap - EXIT
	if [ "$status" -ne 0 ]; then
		echo "DMG packaging failed at stage=$stage" >&2
		df -h "$work" "$(dirname "$output")" >&2 || true
	fi
	if [ "$mounted" = true ]; then
		if ! hdiutil detach "$mountpoint" && ! hdiutil detach -force "$mountpoint"; then
			echo "Could not detach the packaging volume; temporary files retained at $work" >&2
			exit 1
		fi
	fi
	rm -rf "$work"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Count logical bytes too: du alone undercounts sparse/compressed binaries.
# Allow twice the payload plus 64 MiB for filesystem metadata and slack.
size_mib=$(python3 - "$app" "$work" "$(dirname "$output")" <<'PY'
import os
import shutil
import sys

app, work, output = sys.argv[1:]
payload = 0
for root, directories, files in os.walk(app):
    for name in directories + files:
        entry = os.lstat(os.path.join(root, name))
        payload += max(entry.st_size, entry.st_blocks * 512) + 4096
mib = 1024 * 1024
size = max(128, 2 * ((payload + mib - 1) // mib) + 64)
print(f"DMG payload budget: {payload} bytes; image capacity: {size} MiB", file=sys.stderr)
# Writable and compressed images coexist until conversion finishes.
for path, required in ((work, 2 * size * mib), (output, size * mib)):
    free = shutil.disk_usage(path).free
    print(f"DMG disk space: path={path} free={free} required={required}", file=sys.stderr)
    if free < required:
        sys.exit(f"Insufficient host disk space at {path}; free build caches or use a larger temporary volume")
print(size)
PY
)

stage=create
# A blank image is writable by default; -format requires a source image/folder.
hdiutil create -size "${size_mib}m" -fs HFS+ -volname KubePeep "$work/writable.dmg"
stage=attach
mkdir "$mountpoint"
hdiutil attach -nobrowse -noautoopen -mountpoint "$mountpoint" "$work/writable.dmg"
mounted=true
stage=copy
ditto "$app" "$mountpoint/$(basename "$app")"
stage=detach
hdiutil detach "$mountpoint"
mounted=false
stage=compress
hdiutil convert "$work/writable.dmg" -format UDZO -o "$work/packaged.dmg"
stage=verify
hdiutil verify "$work/packaged.dmg"
stage=publish-file
mv -f "$work/packaged.dmg" "$output"
echo "DMG ready: $output"
