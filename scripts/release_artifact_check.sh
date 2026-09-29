#!/bin/sh
set -eu

if [ "$#" -eq 0 ]; then
	set -- dist
fi

forbidden_pattern='MUST_NOT_PERSIST|object-must-not-persist|namespace-must-not-persist|context-must-not-persist|-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|Authorization:[[:space:]]*(Bearer|Basic)[[:space:]]'
temporary_root=$(mktemp -d)
trap 'rm -rf "$temporary_root"' EXIT HUP INT TERM

scan_tree() {
	label=$1
	path=$2
	if grep -aERn -E "$forbidden_pattern" "$path" >"$temporary_root/findings" 2>/dev/null; then
		echo "release-artifact-check: forbidden internal sentinel or credential material in $label" >&2
		cat "$temporary_root/findings" >&2
		exit 1
	fi
}

inspect_archive() {
	archive=$1
	destination=$temporary_root/archive-$(printf '%s' "$archive" | cksum | cut -d' ' -f1)
	mkdir -p "$destination"
	case "$archive" in
		*.zip) unzip -qq "$archive" -d "$destination" ;;
		*.tar|*.tar.gz|*.tgz|*.tar.xz|*.txz) tar -xf "$archive" -C "$destination" ;;
		*.deb)
			if command -v dpkg-deb >/dev/null 2>&1; then
				dpkg-deb -x "$archive" "$destination"
			else
				echo "release-artifact-check: dpkg-deb is required to inspect $archive" >&2
				exit 1
			fi
			;;
		*) return ;;
	esac
	scan_tree "$archive" "$destination"
}

inspected=0
for candidate in "$@"; do
	if [ ! -e "$candidate" ]; then
		echo "release-artifact-check: missing path: $candidate" >&2
		exit 1
	fi
	if [ -d "$candidate" ]; then
		for file in "$candidate"/*; do
			[ -e "$file" ] || continue
			case "$file" in
				*.zip|*.tar|*.tar.gz|*.tgz|*.tar.xz|*.txz|*.deb) inspect_archive "$file" ;;
			esac
		done
		scan_tree "$candidate" "$candidate"
	else
		inspect_archive "$candidate"
		scan_tree "$candidate" "$candidate"
	fi
	inspected=$((inspected + 1))
done

echo "release-artifact-check: passed ($inspected requested path(s))"
