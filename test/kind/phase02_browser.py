#!/usr/bin/env python3
"""Measure Phase 2 cache/watch behaviour on the explicit 200x10 Kind dataset."""

from __future__ import annotations

import json
import os
import re
import subprocess
import tempfile
import time
from pathlib import Path

from app_e2e import Client, create_scope, select_context, select_scope


ROOT = Path(__file__).resolve().parents[2]
CONTEXT = "kind-kubepeep-f4"
DATASET = "200x10"
NAMESPACES = {f"kp-bench-{index:04d}" for index in range(200)}


def command(*args: str, environment: dict[str, str] | None = None) -> str:
    return subprocess.check_output(args, env=environment, text=True, stderr=subprocess.DEVNULL).strip()


def require_dataset() -> None:
    raw = command("kubectl", "--context", CONTEXT, "get", "namespaces", "-l", f"kubepeep.dev/dataset={DATASET}", "-o", "json")
    names = {item["metadata"]["name"] for item in json.loads(raw)["items"]}
    if names != NAMESPACES | {"kp-benchmark-system"}:
        raise RuntimeError("the dedicated 200x10 dataset is incomplete or differs from the probe contract")


def write_restricted_kubeconfig(path: Path) -> None:
    source = json.loads(command("kubectl", "config", "view", "--raw", "--minify", "--context", CONTEXT, "-o", "json"))
    token = command("kubectl", "--context", CONTEXT, "-n", "kp-benchmark-system", "create", "token", "global-viewer", "--duration=1h")
    config = {
        "apiVersion": "v1", "kind": "Config", "current-context": CONTEXT,
        "clusters": [{"name": CONTEXT, "cluster": source["clusters"][0]["cluster"]}],
        "contexts": [{"name": CONTEXT, "context": {"cluster": CONTEXT, "user": "phase02-global-viewer"}}],
        "users": [{"name": "phase02-global-viewer", "user": {"token": token}}],
    }
    path.write_text(json.dumps(config), encoding="utf-8")
    path.chmod(0o600)


def wait_for_origin(binary: Path, environment: dict[str, str], process: subprocess.Popen[bytes]) -> str:
    for _ in range(60):
        if process.poll() is not None:
            raise RuntimeError("the isolated KubePeep process exited before becoming ready")
        try:
            status = command(str(binary), "status", environment=environment)
        except subprocess.CalledProcessError:
            time.sleep(1)
            continue
        match = re.fullmatch(r"running pid=\d+ port=(\d+) protocol=kubepeep-control/v1", status)
        if match:
            return f"http://127.0.0.1:{match.group(1)}"
        time.sleep(1)
    raise RuntimeError("the isolated KubePeep process did not report a loopback port")


def process_stats(pid: int) -> dict[str, int]:
    values: dict[str, int] = {}
    for line in Path(f"/proc/{pid}/status").read_text(encoding="utf-8").splitlines():
        if line.startswith("VmRSS:"):
            values["rssBytes"] = int(line.split()[1]) * 1024
        elif line.startswith("Threads:"):
            values["threads"] = int(line.split()[1])
    return values


def extract_report(output: str, prefix: str) -> dict[str, object]:
    matches = re.findall(re.escape(prefix) + r" (\{.*\})", output)
    if not matches:
        raise RuntimeError(f"Playwright output omitted {prefix}")
    return json.loads(matches[-1])


def main() -> None:
    require_dataset()
    binary = (ROOT / "dist/kubePeep").resolve()
    if not binary.is_file():
        raise RuntimeError("build dist/kubePeep before running the Phase 2 probe")
    report_path = ROOT / "test/kind/.state/phase02-real.json"

    with tempfile.TemporaryDirectory(prefix="kubepeep-phase02-") as temporary:
        temporary_path = Path(temporary)
        environment = os.environ.copy()
        for name, folder in (("HOME", "home"), ("XDG_CONFIG_HOME", "config"), ("XDG_DATA_HOME", "data"), ("XDG_CACHE_HOME", "cache"), ("XDG_RUNTIME_DIR", "runtime")):
            location = temporary_path / folder
            location.mkdir(mode=0o700)
            environment[name] = str(location)
        kubeconfig = temporary_path / "kubeconfig"
        write_restricted_kubeconfig(kubeconfig)
        environment["KUBECONFIG"] = str(kubeconfig)
        data_root = Path(environment["HOME"]) / ".kubePeep"
        data_root.mkdir(mode=0o700)
        config = data_root / "config.yaml"
        config.write_text(
            "version: 1\n"
            "server:\n  port: null\n  openBrowser: false\n  shutdownTimeout: 10s\n"
            "dashboard:\n  blockTimeout: 8s\n"
            "resources:\n  collectionTimeout: 30s\n"
            "observability:\n  otel:\n    enabled: false\n    endpoint: null\n    protocol: http/protobuf\n    insecure: false\n  metrics:\n    enabled: true\n",
            encoding="utf-8",
        )
        config.chmod(0o600)

        with (temporary_path / "process.log").open("wb") as log:
            process = subprocess.Popen(
                [str(binary), "start", "--no-browser", "--kubeconfig", str(kubeconfig), "--context", CONTEXT, "--namespace", "kp-bench-0000"],
                env=environment, stdout=log, stderr=subprocess.STDOUT,
            )
            try:
                origin = wait_for_origin(binary, environment, process)
                client = Client(origin)
                status, _ = client.bootstrap()
                status = select_context(client, status)
                scope = create_scope(client, status, "phase02-large", "all", [], None)
                status = select_scope(client, status, scope)
                if status["selection"].get("namespaceCount", 0) < 200:
                    raise RuntimeError("the Phase 2 all scope did not resolve the 200 namespace dataset")
                baseline = process_stats(process.pid)

                browser_environment = os.environ.copy()
                browser_environment["KUBEPEEP_PHASE02_ORIGIN"] = origin
                completed = subprocess.run(
                    ["rtk", "npm", "run", "test:e2e", "--", "phase02-real.spec.ts", "--workers=1"],
                    cwd=ROOT / "web", env=browser_environment, check=False, text=True, capture_output=True,
                )
                print(completed.stdout, end="")
                print(completed.stderr, end="")
                if completed.returncode != 0:
                    raise RuntimeError("the Phase 2 Playwright probe failed")
                time.sleep(1)
                final = process_stats(process.pid)
                report = {
                    "schemaVersion": 1,
                    "dataset": DATASET,
                    "resolvedNamespaces": status["selection"]["namespaceCount"],
                    "memoryBudgetBytes": 224 << 20,
                    "baseline": baseline,
                    "final": final,
                    "navigation": extract_report(completed.stdout, "Phase 02 real Kind navigation:"),
                    "tiers": extract_report(completed.stdout, "Phase 02 real Kind tiers:"),
                }
                if final.get("rssBytes", 0) >= report["memoryBudgetBytes"]:
                    raise RuntimeError(f"process RSS exceeded aggregate memory budget: {report}")
                temporary_report = report_path.with_suffix(".tmp")
                temporary_report.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
                temporary_report.chmod(0o600)
                temporary_report.replace(report_path)
                print(f"Phase 02 report: {json.dumps(report, sort_keys=True)}")
            finally:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=10)


if __name__ == "__main__":
    main()
