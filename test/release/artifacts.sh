#!/bin/sh
set -eu

repository=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
scanner=$repository/scripts/release/check-artifacts.sh
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

expect_rejected() {
	if "$scanner" "$1" >"$test_root/output" 2>&1; then
		echo "artifact scanner accepted $2" >&2
		exit 1
	fi
}

mkdir -p "$test_root/clean/nested" "$test_root/payload" "$test_root/release/nested"
printf 'public application data\n' >"$test_root/clean/nested/kubePeep"
tar -czf "$test_root/clean/nested/package.tar.gz" -C "$test_root/clean/nested" kubePeep
"$scanner" "$test_root/clean" >/dev/null

printf 'MUST_NOT_PERSIST\n' >"$test_root/payload/private-data"
expect_rejected "$test_root/payload/private-data" 'a plain sentinel'
if grep -q MUST_NOT_PERSIST "$test_root/output"; then
	echo 'artifact scanner disclosed rejected content' >&2
	exit 1
fi
tar -czf "$test_root/release/nested/package.tar.gz" -C "$test_root/payload" private-data
expect_rejected "$test_root/release" 'an archive in a nested directory'
expect_rejected "$test_root/release/nested/package.tar.gz" 'a directly requested archive'

mkdir "$test_root/outer"
tar -czf "$test_root/outer/bundle.tar.gz" -C "$test_root/release" nested
expect_rejected "$test_root/outer" 'an archive inside another archive'

mkdir -p "$test_root/hidden/.payload"
cp "$test_root/payload/private-data" "$test_root/hidden/.payload/data"
expect_rejected "$test_root/hidden" 'a sentinel in a hidden directory'
expect_rejected "$test_root/missing" 'a missing requested path'
mkdir "$test_root/empty"
expect_rejected "$test_root/empty" 'an empty artifact tree'

# Never traverse links outside the artifact tree or loop through bundle links.
ln -s "$test_root/payload" "$test_root/clean/external"
ln -s . "$test_root/clean/loop"
"$scanner" "$test_root/clean" >/dev/null

mkdir "$test_root/fake-bin"
cat >"$test_root/fake-bin/grep" <<'EOF'
#!/bin/sh
exit 2
EOF
chmod +x "$test_root/fake-bin/grep"
if PATH="$test_root/fake-bin:$PATH" "$scanner" "$test_root/clean" >"$test_root/output" 2>&1; then
	echo 'artifact scanner ignored an inspection error' >&2
	exit 1
fi

# Wails resolves -o below build/bin, outside the CLI's dist directory.
mkdir -p "$test_root/workspace/scripts/release" "$test_root/workspace/dist" "$test_root/workspace/build/bin/dist/desktop"
cp "$scanner" "$test_root/workspace/scripts/release/"
cp "$test_root/clean/nested/kubePeep" "$test_root/workspace/dist/kubePeep"
cp "$test_root/payload/private-data" "$test_root/workspace/build/bin/dist/desktop/kubePeep"
if (cd "$test_root/workspace" && make --no-print-directory -f "$repository/Makefile" release-artifact-check) >"$test_root/output" 2>&1; then
	echo 'release-artifact-check ignored the Wails build output' >&2
	exit 1
fi
cp "$test_root/clean/nested/kubePeep" "$test_root/workspace/build/bin/dist/desktop/kubePeep"
(cd "$test_root/workspace" && make --no-print-directory -f "$repository/Makefile" release-artifact-check) >/dev/null

echo 'release-artifact-check regression tests passed'
