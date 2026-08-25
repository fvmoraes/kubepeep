#!/bin/sh
set -eu

security_script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
security_repo_root=$(CDPATH= cd -- "$security_script_dir/.." && pwd)
security_ref=${1:-HEAD}

security_fail() {
	printf 'security-check: %s\n' "$*" >&2
	exit 1
}

case "$security_ref" in
''|-*|*[!A-Za-z0-9._/:~^+-]*)
	security_fail "the history ref contains unsupported characters"
	;;
esac

cd "$security_repo_root"
security_object=$(git rev-parse --verify "${security_ref}^{object}") ||
	security_fail "the security ref does not resolve to a Git object"
security_object_type=$(git cat-file -t "$security_object") ||
	security_fail "the security ref cannot be inspected"
case "$security_object_type" in
commit | tag) ;;
*) security_fail "the security ref must resolve to a commit or tag" ;;
esac

security_tip=$(git rev-parse --verify "${security_object}^{commit}") ||
	security_fail "the history ref does not resolve to a commit"

security_identity_violations=$(
	git log "$security_tip" --format='%H%x09%ae%x09%ce' |
		awk -F '\t' '
			function safe_identity(value) {
				return value ~ /@users\.noreply\.github\.com$/ || value == "noreply@github.com"
			}
			!safe_identity($2) || !safe_identity($3) { print $1 }
		'
)
if [ -n "$security_identity_violations" ]; then
	printf '%s\n' "security-check: commit identities must use GitHub noreply addresses; offending commits:" >&2
	printf '%s\n' "$security_identity_violations" >&2
	exit 1
fi

security_message_metadata_violations=$(
	git log "$security_tip" --format='%H' |
		while IFS= read -r security_commit; do
			security_message=$(git log -1 --format='%B' "$security_commit")
			if printf '%s\n' "$security_message" |
				grep -Eq '/home/[A-Za-z0-9._-]+|/Users/[A-Za-z0-9._-]+|[A-Za-z]:\\Users\\[A-Za-z0-9._-]+|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}'; then
				printf '%s\n' "$security_commit"
			fi
		done
)
if [ -n "$security_message_metadata_violations" ]; then
	printf '%s\n' "security-check: commit messages must not contain machine paths or email addresses; offending commits:" >&2
	printf '%s\n' "$security_message_metadata_violations" >&2
	exit 1
fi

security_machine_path_commits=$(
	git log "$security_tip" --format='%H' --no-patch \
		-G'/home/[A-Za-z0-9._-]+|/Users/[A-Za-z0-9._-]+|[A-Za-z]:\\Users\\[A-Za-z0-9._-]+' -- .
)
if [ -n "$security_machine_path_commits" ]; then
	printf '%s\n' "security-check: machine-specific absolute paths are forbidden; offending commits:" >&2
	printf '%s\n' "$security_machine_path_commits" >&2
	exit 1
fi

security_risky_paths=$(
	git log "$security_tip" --format= --name-only --diff-filter=ACMRT |
		awk '
			{
				security_path = tolower($0)
				if (security_path ~ /(^|\/)\.codebase-memory\//) { print; next }
				if (security_path ~ /(^|\/)\.env($|\.)/ && security_path !~ /(^|\/)\.env\.example$/) { print; next }
				if (security_path ~ /(^|\/)(id_rsa|id_ed25519)($|\.)/) { print; next }
				if (security_path ~ /(^|\/)(kubeconfig|credentials)($|\.)/) { print; next }
				if (security_path ~ /\.(kubeconfig|key|pem|p12|pfx|db|sqlite|sqlite3|log|zip|tgz|gz|zst|7z|exe|dll|dylib|so)$/) { print; next }
			}
		'
)
if [ -n "$security_risky_paths" ]; then
	printf '%s\n' "security-check: sensitive or generated file names are forbidden anywhere in history; offending paths:" >&2
	printf '%s\n' "$security_risky_paths" >&2
	exit 1
fi

security_staged_risky_paths=$(
	git diff --cached --name-only --diff-filter=ACMRT |
		awk '
			{
				security_path = tolower($0)
				if (security_path ~ /(^|\/)\.codebase-memory\//) { print; next }
				if (security_path ~ /(^|\/)\.env($|\.)/ && security_path !~ /(^|\/)\.env\.example$/) { print; next }
				if (security_path ~ /(^|\/)(id_rsa|id_ed25519)($|\.)/) { print; next }
				if (security_path ~ /(^|\/)(kubeconfig|credentials)($|\.)/) { print; next }
				if (security_path ~ /\.(kubeconfig|key|pem|p12|pfx|db|sqlite|sqlite3|log|zip|tgz|gz|zst|7z|exe|dll|dylib|so)$/) { print; next }
			}
		'
)
if [ -n "$security_staged_risky_paths" ]; then
	printf '%s\n' "security-check: staged sensitive or generated file names are forbidden; offending paths:" >&2
	printf '%s\n' "$security_staged_risky_paths" >&2
	exit 1
fi

security_staged_machine_paths=$(
	git diff --cached --name-only \
		-G'/home/[A-Za-z0-9._-]+|/Users/[A-Za-z0-9._-]+|[A-Za-z]:\\Users\\[A-Za-z0-9._-]+' -- .
)
if [ -n "$security_staged_machine_paths" ]; then
	printf '%s\n' "security-check: staged machine-specific paths are forbidden; offending files:" >&2
	printf '%s\n' "$security_staged_machine_paths" >&2
	exit 1
fi

if ! git diff --cached --quiet --; then
	go run github.com/zricethezav/gitleaks/v8@v8.28.0 git . \
		--config .gitleaks.toml \
		--pre-commit \
		--staged \
		--no-banner \
		--redact
fi

if ! git log "$security_tip" --format='%B' |
	go run github.com/zricethezav/gitleaks/v8@v8.28.0 stdin \
		--config .gitleaks.toml \
		--no-banner \
		--redact; then
	security_fail "a likely secret was found in commit messages"
fi

if [ "$security_object_type" = tag ]; then
	security_tagger_email=$(git cat-file tag "$security_object" |
		sed -n 's/^tagger .* <\([^>]*\)> [0-9][0-9]* [+-][0-9][0-9][0-9][0-9]$/\1/p')
	case "$security_tagger_email" in
	*@users.noreply.github.com | noreply@github.com) ;;
	*) security_fail "annotated tags must use a GitHub noreply tagger identity" ;;
	esac

	security_tag_message=$(git cat-file tag "$security_object" | sed '1,/^$/d')
	if printf '%s\n' "$security_tag_message" |
		grep -Eq '/home/[A-Za-z0-9._-]+|/Users/[A-Za-z0-9._-]+|[A-Za-z]:\\Users\\[A-Za-z0-9._-]+|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}'; then
		security_fail "annotated tag messages must not contain machine paths or email addresses"
	fi
	if ! printf '%s\n' "$security_tag_message" |
		go run github.com/zricethezav/gitleaks/v8@v8.28.0 stdin \
			--config .gitleaks.toml \
			--no-banner \
			--redact; then
		security_fail "a likely secret was found in the annotated tag message"
	fi
fi

go run github.com/zricethezav/gitleaks/v8@v8.28.0 git . \
	--config .gitleaks.toml \
	--log-opts="$security_tip" \
	--max-decode-depth=2 \
	--no-banner \
	--redact

printf '%s\n' "security-check: staged content, history, identities, paths, messages, tags, and secrets passed"
