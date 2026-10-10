#!/usr/bin/env bash
# Native macOS round trip: exercise real hdiutil, not a command stub.
set -euo pipefail

repository=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_root=$(mktemp -d)
mounted=false
cleanup() {
	status=$?
	trap - EXIT
	if [ "$mounted" = true ]; then
		if ! hdiutil detach "$test_root/mounted" && ! hdiutil detach -force "$test_root/mounted"; then
			exit 1
		fi
	fi
	rm -rf "$test_root"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

app="$test_root/source with spaces/kubePeep.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$test_root/mounted"
python3 - "$app" <<'PY'
from pathlib import Path
import sys

app = Path(sys.argv[1])
binary = app / 'Contents/MacOS/kubePeep'
with binary.open('wb') as stream:
    stream.write(b'kubepeep packaging regression\n')
    stream.seek(80 * 1024 * 1024)
    stream.write(b'end\n')
binary.chmod(0o755)
(app / 'Contents/Resources/runtime').symlink_to('../MacOS/kubePeep')
PY
"$repository/scripts/release/create-dmg.sh" "$app" "$test_root/result.dmg"
hdiutil attach -readonly -nobrowse -noautoopen -mountpoint "$test_root/mounted" "$test_root/result.dmg"
mounted=true
cmp "$app/Contents/MacOS/kubePeep" "$test_root/mounted/kubePeep.app/Contents/MacOS/kubePeep"
test -x "$test_root/mounted/kubePeep.app/Contents/MacOS/kubePeep"
test "$(readlink "$test_root/mounted/kubePeep.app/Contents/Resources/runtime")" = '../MacOS/kubePeep'
hdiutil detach "$test_root/mounted"
mounted=false
echo 'DMG round trip passed: content, executable mode, symlinks, and sparse payload'
