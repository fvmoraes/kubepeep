#!/usr/bin/env bash
# Exercise the release CLI in an isolated repository; never touch local tags.
set -euo pipefail

repository=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
release_tool=${RELEASE_TOOL:-$repository/scripts/release.sh}
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
cd "$fixture"
git init -q
git config user.name 'Release test'
git config user.email 'release-test@users.noreply.github.com'
git -c core.hooksPath=/dev/null commit -qm 'fix: initial fixture' --allow-empty
git tag 0.6.2

assert_equal() {
	if [ "$2" != "$3" ]; then
		printf 'FAIL %s: got <%s>, want <%s>\n' "$1" "$2" "$3" >&2
		exit 1
	fi
	printf 'PASS %s\n' "$1"
}

git -c core.hooksPath=/dev/null commit -qm 'feat: complete v0.7' --allow-empty
assert_equal 'minor bump' "$("$release_tool" bump-type 0.6.2)" minor
assert_equal 'version 0.7.0' "$("$release_tool" next-version 0.6.2 minor)" 0.7.0
git -c core.hooksPath=/dev/null commit -qm 'fix: compatibility change' -m 'BREAKING-CHANGE: fixture contract' --allow-empty
assert_equal 'breaking footer' "$("$release_tool" bump-type 0.6.2)" major

cat > CHANGELOG.md <<'CHANGELOG'
# Changelog

## [0.6.2] - 2026-09-08

### Fixed
- Previous version.
CHANGELOG
"$release_tool" changelog 0.7.0 2026-09-29 0.6.2
cp CHANGELOG.md generated.md
"$release_tool" changelog 0.7.0 2026-09-30 0.6.2
cmp generated.md CHANGELOG.md
assert_equal 'single changelog section after retry' "$(grep -Fc '## [0.7.0] -' CHANGELOG.md)" 1

cat > CHANGELOG.md <<'CHANGELOG'
# Changelog

## [0.7.0] - 2026-09-29

### Added
- Reviewed notes with a literal \n and backticks `kept`.

## [0.6.2] - 2026-09-08

### Fixed
- Previous version.
CHANGELOG
cp CHANGELOG.md curated.md
"$release_tool" changelog 0.7.0 2026-09-30 0.6.2
cmp curated.md CHANGELOG.md
notes=$("$release_tool" notes 0.6.2 0.7.0 minor)
case "$notes" in
	*'Reviewed notes with a literal \n and backticks `kept`.'*) ;;
	*) echo 'FAIL curated release notes lost' >&2; exit 1 ;;
esac
case "$notes" in
	*'Previous version.'*) echo 'FAIL release notes include older section' >&2; exit 1 ;;
esac
echo 'PASS curated notes survive regeneration and exclude older entries'

printf '%s\n' '{"name":"fixture","info":{"productVersion":"0.6.2","productName":"KubePeep"}}' > wails.json
"$release_tool" metadata 0.7.0
assert_equal 'native product version' "$(node -p 'require("./wails.json").info.productVersion')" 0.7.0
assert_equal 'other native metadata preserved' "$(node -p 'require("./wails.json").info.productName')" KubePeep
cp wails.json expected.json
for invalid in v0.7.0 0.7 00.7.0 '0.7.0;exit'; do
	if "$release_tool" metadata "$invalid" >/dev/null 2>&1; then
		echo "FAIL invalid version accepted: $invalid" >&2; exit 1
	fi
	cmp expected.json wails.json
done
echo 'PASS invalid metadata versions rejected without changing configuration'

# More than a pipe buffer after the matching line catches grep -q + pipefail
# turning a correct MINOR into PATCH when printf receives SIGPIPE.
mkdir fake-bin
cat > fake-bin/git <<'GIT'
#!/usr/bin/env bash
printf '%s\n' 'feat: early matching commit'
for ((i=0; i<6000; i++)); do printf 'fix: historical commit %s\n' "$i"; done
GIT
chmod +x fake-bin/git
assert_equal 'large history retains the matching bump' "$(PATH="$fixture/fake-bin:$PATH" "$release_tool" bump-type 0.6.2)" minor
echo 'release-tooling: all checks passed'
