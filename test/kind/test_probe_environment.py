import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

import phase02_browser
import phase03_browser
from probe_environment import KindSource, read_kind_source


class ProbeEnvironmentTests(unittest.TestCase):
    def test_ambient_kubeconfig_is_never_a_fallback(self):
        with patch.dict(os.environ, {"KUBECONFIG": "/personal/config"}, clear=True):
            with self.assertRaisesRegex(RuntimeError, "dedicated Kind kubeconfig"):
                read_kind_source()

    def test_missing_files_and_directories_are_rejected(self):
        with tempfile.TemporaryDirectory() as root:
            for value in (root, str(Path(root) / "missing")):
                with self.subTest(value=value), patch.dict(os.environ, {"KUBEPEEP_KIND_KUBECONFIG": value}, clear=True):
                    with self.assertRaisesRegex(RuntimeError, "existing regular file"):
                        read_kind_source()

    def test_cluster_name_must_be_an_explicit_kind_dns_label(self):
        with tempfile.NamedTemporaryFile() as config:
            for cluster in ("", "-leading", "trailing-", "production/context", "UPPER", "a" * 64):
                with self.subTest(cluster=cluster), patch.dict(os.environ, {
                    "KUBEPEEP_KIND_KUBECONFIG": config.name, "KUBEPEEP_KIND_CLUSTER": cluster,
                }, clear=True):
                    with self.assertRaisesRegex(RuntimeError, "lowercase DNS label"):
                        read_kind_source()

    def test_every_kubectl_command_uses_the_dedicated_file_and_context(self):
        with tempfile.NamedTemporaryFile() as config, patch.dict(os.environ, {
            "KUBEPEEP_KIND_KUBECONFIG": config.name, "KUBEPEEP_KIND_CLUSTER": "audit-012",
            "KUBECONFIG": "/personal/config",
        }, clear=True), patch("probe_environment.subprocess.check_output", return_value="{}\n") as run:
            source = read_kind_source()
            self.assertEqual(source.command("get", "namespaces", "-o", "json"), "{}")
            self.assertEqual(run.call_args.args[0], [
                "kubectl", "--kubeconfig", str(Path(config.name).resolve()), "--context", "kind-audit-012",
                "get", "namespaces", "-o", "json",
            ])

    def test_each_browser_probe_exports_only_restricted_credentials(self):
        for probe in (phase02_browser, phase03_browser):
            with self.subTest(probe=probe.__name__), tempfile.TemporaryDirectory() as root:
                source = Mock(spec=KindSource)
                source.context = "kind-audit-012"
                cluster = {"server": "https://127.0.0.1:12345", "certificate-authority-data": "synthetic-ca"}
                source.command.side_effect = [
                    json.dumps({"clusters": [{"cluster": cluster}], "users": [{"user": {"token": "admin-never-copy"}}]}),
                    "short-lived-viewer",
                ]
                target = Path(root) / "restricted.kubeconfig"
                probe.write_restricted_kubeconfig(target, source)
                result = json.loads(target.read_text())
                self.assertEqual(result["current-context"], source.context)
                self.assertEqual(result["users"][0]["user"], {"token": "short-lived-viewer"})
                self.assertNotIn("admin-never-copy", target.read_text())
                self.assertEqual(target.stat().st_mode & 0o777, 0o600)
                self.assertIn("--duration=1h", source.command.call_args.args)

    def test_each_browser_probe_rejects_an_unprepared_dataset(self):
        for probe in (phase02_browser, phase03_browser):
            with self.subTest(probe=probe.__name__):
                source = Mock(spec=KindSource)
                source.command.return_value = json.dumps({"items": []})
                with self.assertRaisesRegex(RuntimeError, "dataset is incomplete"):
                    probe.require_dataset(source)


if __name__ == "__main__":
    unittest.main()
