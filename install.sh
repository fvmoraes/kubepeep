#!/bin/sh
set -eu

repository_url=https://github.com/fvmoraes/kubepeep
binary_name=kubePeep
version=
install_dir=
uninstall=false
purge_data=false
confirm_purge=false
maximum_checksums_bytes=1048576
maximum_archive_bytes=268435456
installer_lock=
install_dir_created=false
install_dir_physical=
tmp_root=
staged=
backup=
had_backup=false
replacement_active=false
preserve_recovery=false

cleanup() {
	if [ "$replacement_active" = true ]; then
		if [ "$had_backup" = true ] && [ -n "$backup" ] && [ -f "$backup" ]; then
			if mv -f "$backup" "$binary_path" 2>/dev/null; then
				backup=
				replacement_active=false
			else
				preserve_recovery=true
			fi
		elif [ -n "${binary_path:-}" ]; then
			rm -f "$binary_path" 2>/dev/null || true
			replacement_active=false
		fi
	fi
	if [ -n "$staged" ]; then
		case "$staged" in
			"$install_dir_physical"/.kubePeep.install.*) rm -f "$staged" 2>/dev/null || true ;;
		esac
	fi
	if [ -n "$backup" ] && [ "$preserve_recovery" = false ]; then
		case "$backup" in
			"$install_dir_physical"/.kubePeep.backup.*) rm -f "$backup" 2>/dev/null || true ;;
		esac
	fi
	[ -z "$tmp_root" ] || rm -rf "$tmp_root"
	if [ -n "$installer_lock" ]; then
		rmdir "$installer_lock" 2>/dev/null || true
	fi
	if [ "$install_dir_created" = true ] && [ -n "$install_dir_physical" ]; then
		rmdir "$install_dir_physical" 2>/dev/null || true
	fi
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

usage() {
	cat <<'EOF'
usage:
  install.sh --version X.Y.Z [--install-dir PATH]
  install.sh --uninstall [--install-dir PATH] [--purge-data --confirm-purge]

The installer downloads only immutable, versioned GitHub release assets and
requires their SHA-256 checksum. Data is preserved on uninstall unless both
purge flags are supplied.
EOF
}

fail() {
	printf 'kubePeep installer: %s\n' "$1" >&2
	exit 1
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--version)
			[ "$#" -ge 2 ] || fail "--version requires a value"
			version=$2
			shift 2
			;;
		--install-dir)
			[ "$#" -ge 2 ] || fail "--install-dir requires a value"
			install_dir=$2
			shift 2
			;;
		--uninstall)
			uninstall=true
			shift
			;;
		--purge-data)
			purge_data=true
			shift
			;;
		--confirm-purge)
			confirm_purge=true
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			fail "unknown argument: $1"
			;;
	esac
done

[ -n "${HOME:-}" ] || fail "HOME is required"
case "$HOME" in
	/*) ;;
	*) fail "HOME must be an absolute path" ;;
esac

home_physical=$(cd "$HOME" 2>/dev/null && pwd -P) || fail "HOME cannot be resolved"
[ "$home_physical" != "/" ] || fail "refusing to use the filesystem root as HOME"

default_install_dir=$home_physical/.local/bin
if [ -z "$install_dir" ]; then
	install_dir=$default_install_dir
fi
case "$install_dir" in
	/*) ;;
	*) fail "install directory must be absolute" ;;
esac

profile_path=$home_physical/.profile
path_start='# >>> kubePeep installer PATH >>>'
path_end='# <<< kubePeep installer PATH <<<'

validate_managed_path() {
	if [ -e "$profile_path" ] || [ -L "$profile_path" ]; then
		[ -f "$profile_path" ] && [ ! -L "$profile_path" ] || fail "refusing a non-regular .profile path"
	else
		return 0
	fi
	awk -v start="$path_start" -v end="$path_end" '
		$0 == start {
			starts++
			if (managed || starts > 1) invalid=1
			managed=1
			next
		}
		$0 == end {
			ends++
			if (!managed || ends > 1) invalid=1
			managed=0
			next
		}
		END {
			if (invalid || managed || starts != ends) exit 2
		}
	' "$profile_path" >/dev/null || fail "managed PATH markers in .profile are malformed"
}

remove_managed_path() {
	[ "$install_dir" = "$default_install_dir" ] || return 0
	[ -f "$profile_path" ] || return 0
	validate_managed_path
	tmp_profile=$(mktemp "$home_physical/.profile.kubepeep.XXXXXX") || fail "could not stage the user PATH file"
	awk -v start="$path_start" -v end="$path_end" '
		$0 == start { managed=1; next }
		$0 == end { managed=0; next }
		!managed { print }
	' "$profile_path" >"$tmp_profile" || {
		rm -f "$tmp_profile"
		fail "could not update the user PATH file"
	}
	chmod --reference="$profile_path" "$tmp_profile" 2>/dev/null || chmod 600 "$tmp_profile"
	mv -f "$tmp_profile" "$profile_path"
}

add_managed_path() {
	[ "$install_dir" = "$default_install_dir" ] || return 0
	case ":${PATH:-}:" in
		*":$install_dir:"*) return 0 ;;
	esac
	validate_managed_path
	tmp_profile=$(mktemp "$home_physical/.profile.kubepeep.XXXXXX") || fail "could not stage the user PATH file"
	if [ -f "$profile_path" ]; then
		awk -v start="$path_start" -v end="$path_end" '
			$0 == start { managed=1; next }
			$0 == end { managed=0; next }
			!managed { print }
		' "$profile_path" >"$tmp_profile" || {
			rm -f "$tmp_profile"
			fail "could not update the user PATH file"
		}
	fi
	{
		printf '\n%s\n' "$path_start"
		printf '%s\n' 'case ":$PATH:" in'
		printf '%s\n' '  *":$HOME/.local/bin:"*) ;;'
		printf '%s\n' '  *) PATH="$HOME/.local/bin:$PATH"; export PATH ;;'
		printf '%s\n' 'esac'
		printf '%s\n' "$path_end"
	} >>"$tmp_profile" || {
		rm -f "$tmp_profile"
		fail "could not update the user PATH file"
	}
	if [ -f "$profile_path" ]; then
		chmod --reference="$profile_path" "$tmp_profile" 2>/dev/null || chmod 600 "$tmp_profile"
	else
		chmod 600 "$tmp_profile"
	fi
	mv -f "$tmp_profile" "$profile_path"
}

binary_path=$install_dir/$binary_name
data_root=$home_physical/.kubePeep

prepare_install_directory() {
	if [ ! -e "$install_dir" ]; then
		mkdir -p "$install_dir" || fail "install directory could not be created"
		install_dir_created=true
	fi
	[ -d "$install_dir" ] || fail "install directory is not a directory"
	install_dir_physical=$(cd "$install_dir" 2>/dev/null && pwd -P) || fail "install directory cannot be resolved"
	[ "$install_dir_physical" != "/" ] || fail "refusing to install in the filesystem root"
	binary_path=$install_dir_physical/$binary_name
	lock_candidate=$install_dir_physical/.kubePeep.install.lock
	if ! mkdir "$lock_candidate" 2>/dev/null; then
		fail "another install or uninstall transaction is already in progress"
	fi
	installer_lock=$lock_candidate
}

validate_existing_binary() {
	if [ -e "$binary_path" ] || [ -L "$binary_path" ]; then
		[ ! -L "$binary_path" ] || fail "refusing a symlinked binary path"
		[ -f "$binary_path" ] || fail "refusing a non-regular binary path"
	fi
}

process_is_running() {
	[ -x "$binary_path" ] || return 1
	"$binary_path" status >/dev/null 2>&1
}

has_exact_version_token() {
	printf '%s\n' "$1" | awk -v expected="$2" '
		{ for (field=1; field<=NF; field++) if ($field == expected) found=1 }
		END { exit found ? 0 : 1 }
	'
}

validate_purge_local_data() {
	[ "$purge_data" = true ] || return 0
	[ "$confirm_purge" = true ] || fail "--purge-data also requires --confirm-purge"
	[ "$data_root" = "$home_physical/.kubePeep" ] || fail "refusing non-canonical data path"
	[ "$data_root" != "/" ] || fail "refusing to purge the filesystem root"
	[ ! -L "$data_root" ] || fail "refusing to purge a symlinked data root"
	if [ -e "$data_root/runtime/kubePeep.lock" ]; then
		fail "runtime lock exists; stop kubePeep before purging data"
	fi
}

purge_local_data() {
	[ "$purge_data" = true ] || return 0
	rm -rf "$data_root"
	printf 'Purged local data at %s\n' "$data_root"
}

if [ "$uninstall" = true ]; then
	[ -z "$version" ] || fail "--version cannot be combined with --uninstall"
	[ "$confirm_purge" = false ] || [ "$purge_data" = true ] || fail "--confirm-purge requires --purge-data"
	prepare_install_directory
	validate_existing_binary
	validate_managed_path
	validate_purge_local_data
	if process_is_running; then
		fail "kubePeep is running; stop it before uninstalling"
	fi
	if [ -e "$binary_path" ]; then
		rm -f "$binary_path"
	fi
	remove_managed_path
	purge_local_data
	if [ "$purge_data" = false ]; then
		printf 'Removed kubePeep; local data was preserved at %s\n' "$data_root"
	fi
	exit 0
fi

[ "$purge_data" = false ] && [ "$confirm_purge" = false ] || fail "purge flags require --uninstall"
[ -n "$version" ] || fail "--version X.Y.Z is required"
version=${version#v}
case "$version" in
	*[!0-9.]*|.*|*.|*..*) fail "version must be an exact X.Y.Z release" ;;
esac
[ "$(printf '%s' "$version" | awk -F. 'NF == 3 && $1 ~ /^(0|[1-9][0-9]*)$/ && $2 ~ /^(0|[1-9][0-9]*)$/ && $3 ~ /^(0|[1-9][0-9]*)$/ { print "valid" }')" = valid ] || fail "version must be an exact X.Y.Z release"

command -v curl >/dev/null 2>&1 || fail "curl with HTTPS support is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

case "$(uname -s)" in
	Linux) target_os=linux ;;
	Darwin) target_os=darwin ;;
	*) fail "unsupported operating system" ;;
esac
case "$(uname -m)" in
	x86_64|amd64) target_arch=amd64 ;;
	aarch64|arm64) target_arch=arm64 ;;
	*) fail "unsupported architecture" ;;
esac

archive_name=kubepeep-${target_os}-${target_arch}.tar.gz
release_url=$repository_url/releases/download/$version
prepare_install_directory
validate_existing_binary
validate_managed_path
if [ -e "$binary_path" ] && process_is_running; then
	fail "kubePeep is running; stop it before upgrading"
fi
tmp_root=$(mktemp -d 2>/dev/null) || fail "could not create a temporary directory"

download() {
	asset=$1
	destination=$2
	maximum=$3
	blocks=$(( (maximum + 511) / 512 ))
	(
		ulimit -f "$blocks"
		curl --fail --location --silent --show-error --proto '=https' --tlsv1.2 \
			--max-filesize "$maximum" "$release_url/$asset" --output "$destination"
	)
}

download checksums.txt "$tmp_root/checksums.txt" "$maximum_checksums_bytes" || fail "could not download checksums.txt"
download "$archive_name" "$tmp_root/$archive_name" "$maximum_archive_bytes" || fail "could not download $archive_name"

checksums_size=$(wc -c <"$tmp_root/checksums.txt" | tr -d '[:space:]')
archive_size=$(wc -c <"$tmp_root/$archive_name" | tr -d '[:space:]')
case "$checksums_size:$archive_size" in
	*[!0-9:]*) fail "downloaded release asset has an invalid size" ;;
esac
[ "$checksums_size" -le "$maximum_checksums_bytes" ] || fail "checksums.txt exceeds its size limit"
[ "$archive_size" -le "$maximum_archive_bytes" ] || fail "release archive exceeds its size limit"

expected=$(awk -v name="$archive_name" '$2 == name || $2 == "*" name { if (found++) exit 2; print tolower($1) }' "$tmp_root/checksums.txt") || fail "checksum list contains duplicate entries"
[ "$(printf '%s' "$expected" | awk 'length($0) == 64 && $0 ~ /^[0-9a-f]+$/ { print "valid" }')" = valid ] || fail "release checksum entry is missing or invalid"
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmp_root/$archive_name" | awk '{print tolower($1)}')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$tmp_root/$archive_name" | awk '{print tolower($1)}')
else
	fail "sha256sum or shasum is required"
fi
[ "$actual" = "$expected" ] || fail "SHA-256 verification failed"

mkdir -p "$tmp_root/extract"
binary_entries=$(tar -tzf "$tmp_root/$archive_name" | awk -v name="$binary_name" '$0 == name { count++ } END { print count + 0 }') || fail "archive listing failed"
[ "$binary_entries" -eq 1 ] || fail "archive must contain exactly one root kubePeep binary"
if tar -tzf "$tmp_root/$archive_name" | awk '
	BEGIN { FS="[/\\\\]" }
	/^[\/\\]/ { unsafe=1 }
	{ for (part=1; part<=NF; part++) if ($part == "..") unsafe=1 }
	END { exit unsafe ? 0 : 1 }
'; then
	fail "archive contains an unsafe path"
fi
# Limit the extracted file at the OS level as well as through the archive
# download budget, so a valid-looking compressed stream cannot expand without
# bound. POSIX specifies ulimit -f in 512-byte blocks.
(
	ulimit -f 524288
	tar -xzf "$tmp_root/$archive_name" -C "$tmp_root/extract" "$binary_name"
) || fail "archive extraction failed or exceeded its size limit"
candidate=$tmp_root/extract/$binary_name
[ -f "$candidate" ] && [ ! -L "$candidate" ] || fail "archive does not contain the expected binary"
candidate_size=$(wc -c <"$candidate" | tr -d '[:space:]')
[ "$candidate_size" -gt 0 ] && [ "$candidate_size" -le "$maximum_archive_bytes" ] || fail "archive binary has an invalid size"
chmod 755 "$candidate"
candidate_version=$($candidate version 2>/dev/null) || fail "downloaded binary did not execute"
has_exact_version_token "$candidate_version" "version=$version" || fail "downloaded binary version does not match the requested release"

validate_existing_binary
validate_managed_path
staged=$(mktemp "$install_dir_physical/.kubePeep.install.XXXXXX") || fail "could not stage the downloaded binary"
if ! cp "$candidate" "$staged" || ! chmod 755 "$staged"; then
	rm -f "$staged"
	staged=
	fail "could not stage the downloaded binary"
fi

if [ -e "$binary_path" ]; then
	if process_is_running; then
		fail "kubePeep is running; stop it before upgrading"
	fi
	backup=$(mktemp "$install_dir_physical/.kubePeep.backup.XXXXXX") || fail "could not allocate a rollback path"
	if ! cp -p "$binary_path" "$backup"; then
		rm -f "$backup"
		backup=
		fail "could not create the rollback backup"
	fi
	had_backup=true
fi
if ! mv -f "$staged" "$binary_path"; then
	rm -f "$staged"
	staged=
	[ "$had_backup" = false ] || rm -f "$backup"
	backup=
	fail "atomic binary replacement failed"
fi
staged=
replacement_active=true
if ! installed_version=$($binary_path version 2>/dev/null) || ! has_exact_version_token "$installed_version" "version=$version"; then
	if [ "$had_backup" = true ]; then
		if ! mv -f "$backup" "$binary_path"; then
			preserve_recovery=true
			fail "post-install verification failed and rollback failed"
		fi
		backup=
	else
		rm -f "$binary_path"
	fi
	replacement_active=false
	fail "post-install verification failed; the previous binary was restored"
fi
add_managed_path
if [ -n "$backup" ] && ! rm -f "$backup"; then
	preserve_recovery=true
	replacement_active=false
	fail "installation completed but the rollback backup could not be removed: $backup"
fi
backup=
replacement_active=false

printf 'Installed kubePeep %s at %s\n' "$version" "$binary_path"
printf 'Next: kubePeep start\n'
