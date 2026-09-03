#!/usr/bin/env bash
# ============================================================================
# KubePeep release tooling — SemVer (sem prefixo "v") por Conventional Commits.
#
# Usado pelo workflow .github/workflows/release.yml e executável localmente
# para validação. Filosofia de referência: release.yml do Ginger.
#
# Subcomandos:
#   bump-type [BASE_REF]              → major|minor|patch (analisando commits)
#   next-version BASE_VERSION TYPE    → próxima versão X.Y.Z
#   notes   PREVIOUS VERSION TYPE     → corpo da GitHub Release (stdout)
#   changelog VERSION DATE PREVIOUS   → insere entrada no CHANGELOG.md
#
# Convenções:
#   feat  → MINOR      fix → PATCH      BREAKING (feat!:/fix!:/rodapé) → MAJOR
#   Tags oficiais NUNCA têm prefixo "v" (1.4.2; a móvel é "latest").
# ============================================================================
set -euo pipefail

REPO_URL="https://github.com/fvmoraes/kubepeep"

cmd_bump_type() {
	local base="${1:-}" range=""
	[ -n "$base" ] && range="${base}..HEAD"
	local subjects bodies
	subjects=$(git log --pretty=%s $range)
	bodies=$(git log --pretty=%B $range)

	if printf '%s\n' "$bodies" | grep -qE '^[a-zA-Z]+(\([^)]*\))?!:'; then
		echo major
	elif printf '%s\n' "$bodies" | grep -q '^BREAKING CHANGE:'; then
		echo major
	elif printf '%s\n' "$subjects" | grep -qE '^feat(\([^)]*\))?:'; then
		echo minor
	else
		echo patch
	fi
}

cmd_next_version() {
	local base="${1:-0.0.0}" type="${2:-patch}" major=0 minor=0 patch=0
	IFS=. read -r major minor patch <<EOF
$base
EOF
	case "$type" in
		major) echo "$((major + 1)).0.0" ;;
		minor) echo "${major:-0}.$((minor + 1)).0" ;;
		patch) echo "${major:-0}.${minor:-0}.$((patch + 1))" ;;
		*) echo "unknown bump type: $type" >&2; exit 2 ;;
	esac
}

cmd_notes() {
	local previous="${1:-}" version="${2:-}" type="${3:-patch}" range=""
	[ -n "$previous" ] && range="${previous}..HEAD"

	local breaking added changed fixed security
	breaking=$(git log --pretty=%s $range | grep -E '^[a-zA-Z]+(\([^)]*\))?!:' || true)
	added=$(git log --pretty=%s $range | grep -E '^feat(\([^)]*\))?:' || true)
	fixed=$(git log --pretty=%s $range | grep -E '^fix(\([^)]*\))?:' || true)
	security=$(git log --pretty=%B $range | grep -E '^security(\([^)]*\))?:' || true)
	changed=$(git log --pretty=%s $range | grep -vE '^feat(\([^)]*\))?:|^fix(\([^)]*\))?:' || true)

	echo "KubePeep ${version}"
	echo
	if [ -n "$breaking" ]; then
		echo "## ⚠ Breaking Changes"
		printf '%s\n' "$breaking" | sed 's/^/- /'
		echo
	fi
	for pair in "Added:$added" "Changed:$changed" "Fixed:$fixed" "Security:$security"; do
		local section="${pair%%:*}" items="${pair#*:}"
		[ -n "$items" ] || continue
		echo "## ${section}"
		printf '%s\n' "$items" | sed 's/^/- /'
		echo
	done
	echo "## Download"
	echo
	echo "Permanently-updated links: ${REPO_URL}/blob/main/docs/download.md"
	echo
	echo "## Full Changelog"
	if [ -n "$previous" ]; then
		echo "**Full Changelog**: ${REPO_URL}/compare/${previous}...${version}"
	else
		echo "**Full Changelog**: ${REPO_URL}/releases/tag/${version}"
	fi
}

cmd_changelog() {
	local version="${1:?version required}" date="${2:?date required}" previous="${3:-}"
	local body entry
	body=$(cmd_notes "$previous" "$version" patch | tail -n +3)
	entry="## [${version}] - ${date}

${body}
"
	if [ -f CHANGELOG.md ]; then
		awk -v entry="$entry" '
			BEGIN { inserted = 0 }
			{
				if (!inserted && $0 ~ /^## \[/) { printf "%s\n", entry; inserted = 1 }
				print
			}
			END { if (!inserted) printf "%s\n", entry }
		' CHANGELOG.md > CHANGELOG.md.new
		mv CHANGELOG.md.new CHANGELOG.md
	else
		{ printf '%s\n' "$entry"; } > CHANGELOG.md
	fi
	grep -q "^\[${version}\]:" CHANGELOG.md || \
		echo "[${version}]: ${REPO_URL}/releases/tag/${version}" >> CHANGELOG.md
}

case "${1:-}" in
	bump-type)    shift; cmd_bump_type "$@" ;;
	next-version) shift; cmd_next_version "$@" ;;
	notes)        shift; cmd_notes "$@" ;;
	changelog)    shift; cmd_changelog "$@" ;;
	*)
		echo "usage: release.sh {bump-type [BASE]|next-version BASE TYPE|notes PREV VER TYPE|changelog VER DATE PREV}" >&2
		exit 2
		;;
esac
