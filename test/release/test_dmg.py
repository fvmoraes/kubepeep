"""Portable failure-path tests; dmg.sh covers the real macOS round trip."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


REPOSITORY = Path(__file__).resolve().parents[2]
SCRIPT = REPOSITORY / "scripts/release/create-dmg.sh"


class DmgPackagingTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.app = self.root / "source with spaces/kubePeep.app"
        binary = self.app / "Contents/MacOS/kubePeep"
        binary.parent.mkdir(parents=True)
        with binary.open("wb") as stream:
            stream.seek(80 * 1024 * 1024)
            stream.write(b"end")
        self.output = self.root / "output with spaces/result.dmg"
        self.output.parent.mkdir()
        self.output.write_text("existing artifact")
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.log = self.root / "commands"
        self.tmp = self.root / "tmp"
        self.tmp.mkdir()
        self.env = dict(os.environ, PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
                        TMPDIR=str(self.tmp), DMG_TEST_LOG=str(self.log))
        self.stub("hdiutil", '''
printf '%s\\n' "$*" >> "$DMG_TEST_LOG"
if [ "$1" = "${DMG_FAIL_STAGE:-}" ]; then exit 1; fi
if [ "$1" = convert ]; then
    while [ "$1" != -o ]; do shift; done
    printf 'compressed image' > "$2"
fi
''')
        self.stub("ditto", '''
printf 'copy\\n' >> "$DMG_TEST_LOG"
if [ "${DMG_FAIL_STAGE:-}" = copy ]; then exit 1; fi
''')

    def stub(self, name, body):
        script = self.bin / name
        script.write_text("#!/bin/sh\nset -eu\n" + body)
        script.chmod(0o755)

    def run_packager(self, **env):
        return subprocess.run(["bash", str(SCRIPT), str(self.app), str(self.output)],
                              env=dict(self.env, **env), capture_output=True, text=True)

    def test_sparse_payload_gets_explicit_capacity_and_verified_output(self):
        result = self.run_packager()
        self.assertEqual(result.returncode, 0, result.stderr)
        commands = self.log.read_text().splitlines()
        create = commands[0].split()
        capacity = int(create[create.index("-size") + 1].removesuffix("m"))
        self.assertGreaterEqual(capacity, 224)  # 80 MiB logical payload plus slack
        self.assertNotIn("-srcfolder", create)
        self.assertEqual(create[create.index("-fs") + 1], "HFS+")
        self.assertEqual([line.split()[0] for line in commands],
                         ["create", "attach", "copy", "detach", "convert", "verify"])
        self.assertEqual(self.output.read_text(), "compressed image")
        self.assertEqual(list(self.tmp.iterdir()), [])

    def test_failures_preserve_previous_output_and_clean_temporary_files(self):
        for stage in ("create", "attach", "copy", "detach", "convert", "verify"):
            with self.subTest(stage=stage):
                self.log.unlink(missing_ok=True)
                # Allow cleanup detach after a simulated normal detach failure.
                self.stub("hdiutil", '''
printf '%s\\n' "$*" >> "$DMG_TEST_LOG"
if [ "$1" = "$DMG_FAIL_STAGE" ] && [ "${2:-}" != -force ]; then exit 1; fi
if [ "$1" = convert ]; then
    while [ "$1" != -o ]; do shift; done
    printf 'compressed image' > "$2"
fi
''')
                result = self.run_packager(DMG_FAIL_STAGE=stage)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("DMG packaging failed at stage=", result.stderr)
                self.assertEqual(self.output.read_text(), "existing artifact")
                self.assertEqual(list(self.tmp.iterdir()), [])
                if stage in ("copy", "detach"):
                    self.assertIn("detach ", self.log.read_text())

    def test_insufficient_host_capacity_fails_before_creating_image(self):
        # Stub only the free-space probe; never fill the developer/CI disk.
        modules = self.root / "modules"
        modules.mkdir()
        (modules / "shutil.py").write_text(
            "from types import SimpleNamespace\n"
            "def disk_usage(path): return SimpleNamespace(free=0)\n")
        result = self.run_packager(PYTHONPATH=str(modules))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Insufficient host disk space", result.stderr)
        self.assertFalse(self.log.exists())
        self.assertEqual(self.output.read_text(), "existing artifact")
        self.assertEqual(list(self.tmp.iterdir()), [])


if __name__ == "__main__":
    unittest.main()
