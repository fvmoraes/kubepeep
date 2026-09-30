#!/bin/sh
set -eu

if [ "$#" -eq 0 ]; then
	set -- dist
fi

forbidden_pattern='MUST_NOT_PERSIST|object-must-not-persist|namespace-must-not-persist|context-must-not-persist|-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|Authorization:[[:space:]]*(Bearer|Basic)[[:space:]]'
temporary_root=$(mktemp -d)
trap 'rm -rf "$temporary_root"' EXIT HUP INT TERM
scanned_files=0

scan_file() {
	if grep -aE "$forbidden_pattern" -- "$1" >/dev/null 2>"$temporary_root/scan-error"; then
		echo "release-artifact-check: forbidden internal sentinel or credential material in $1" >&2
		exit 1
	else
		status=$?
		if [ "$status" -ne 1 ]; then
			echo "release-artifact-check: could not inspect $1" >&2
			cat "$temporary_root/scan-error" >&2
			exit 1
		fi
	fi
}

inspect_archive() (
	case "$1" in
		*.zip|*.tar|*.tar.gz|*.tgz|*.tar.xz|*.txz|*.deb) ;;
		*) return ;;
	esac
	if [ "$2" -ge 16 ]; then
		echo "release-artifact-check: archive nesting limit exceeded in $1" >&2
		exit 1
	fi
	destination=$(mktemp -d "$temporary_root/archive.XXXXXX")
	case "$1" in
		*.zip) unzip -qq "$1" -d "$destination" ;;
		*.tar|*.tar.gz|*.tgz|*.tar.xz|*.txz) tar -xf "$1" -C "$destination" ;;
		*.deb)
			if command -v dpkg-deb >/dev/null 2>&1; then
				dpkg-deb -x "$1" "$destination"
			else
				echo "release-artifact-check: dpkg-deb is required to inspect $1" >&2
				exit 1
			fi
			;;
	esac
	inspect_path "$destination" "$(($2 + 1))"
)

inspect_path() {
	# App bundles contain symlinks to files already present in their tree. Do
	# not follow links into unrelated files or recurse through link cycles.
	if [ -L "$1" ]; then
		return
	fi
	if [ -d "$1" ]; then
		if [ ! -r "$1" ] || [ ! -x "$1" ]; then
			echo "release-artifact-check: could not inspect directory $1" >&2
			exit 1
		fi
		for entry in "$1"/* "$1"/.[!.]* "$1"/..?*; do
			[ -e "$entry" ] || [ -L "$entry" ] || continue
			inspect_path "$entry" "$2"
		done
	elif [ -f "$1" ]; then
		scan_file "$1"
		scanned_files=$((scanned_files + 1))
		inspect_archive "$1" "$2"
	else
		echo "release-artifact-check: unsupported file: $1" >&2
		exit 1
	fi
}

inspected=0
for candidate in "$@"; do
	if [ ! -e "$candidate" ]; then
		echo "release-artifact-check: missing path: $candidate" >&2
		exit 1
	fi
	inspect_path "$candidate" 0
	inspected=$((inspected + 1))
done

if [ "$scanned_files" -eq 0 ]; then
	echo 'release-artifact-check: no regular files to inspect' >&2
	exit 1
fi

echo "release-artifact-check: passed ($inspected requested path(s))"
