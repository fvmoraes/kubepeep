#!/usr/bin/env python3
"""Run the opt-in Phase 3 browser probe against the dedicated Kind dataset.

The 50x10 benchmark dataset must already be applied explicitly. This probe
creates a temporary restricted kubeconfig and app profile, then removes both.
It neither installs resources nor deletes the Kind cluster or dataset.
"""

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
NAMESPACES = [f"kp-bench-{index:04d}" for index in range(50)]


def command(*args: str) -> str:
    return subprocess.check_output(args, text=True, stderr=subprocess.DEVNULL).strip()


def require_dataset() -> None:
    raw = command("kubectl", "--context", CONTEXT, "get", "namespaces", "-l", "kubepeep.dev/dataset=50x10", "-o", "json")
    names = {item["metadata"]["name"] for item in json.loads(raw)["items"]}
    if names != set(NAMESPACES + ["kp-benchmark-system"]):
        raise RuntimeError("the dedicated 50x10 dataset is incomplete or differs from the probe contract")


def write_restricted_kubeconfig(path: Path) -> None:
    source = json.loads(command("kubectl", "config", "view", "--raw", "--minify", "--context", CONTEXT, "-o", "json"))
    token = command("kubectl", "--context", CONTEXT, "-n", "kp-benchmark-system", "create", "token", "global-viewer", "--duration=1h")
    config = {
        "apiVersion": "v1", "kind": "Config", "current-context": CONTEXT,
        "clusters": [{"name": CONTEXT, "cluster": source["clusters"][0]["cluster"]}],
        "contexts": [{"name": CONTEXT, "context": {"cluster": CONTEXT, "user": "phase03-global-viewer"}}],
        "users": [{"name": "phase03-global-viewer", "user": {"token": token}}],
    }
    path.write_text(json.dumps(config), encoding="utf-8")
    path.chmod(0o600)


def wait_for_origin(binary: Path, environment: dict[str, str], process: subprocess.Popen[bytes]) -> str:
    for _ in range(60):
        if process.poll() is not None:
            raise RuntimeError("the isolated KubePeep process exited before becoming ready")
        try:
            status = command_with_environment(environment, str(binary), "status")
        except subprocess.CalledProcessError:
            time.sleep(1)
            continue
        match = re.fullmatch(r"running pid=\d+ port=(\d+) protocol=kubepeep-control/v1", status)
        if match:
            return f"http://127.0.0.1:{match.group(1)}"
        time.sleep(1)
    raise RuntimeError("the isolated KubePeep process did not report a loopback port")


def command_with_environment(environment: dict[str, str], *args: str) -> str:
    return subprocess.check_output(args, env=environment, text=True, stderr=subprocess.DEVNULL).strip()


def main() -> None:
    require_dataset()
    binary = (ROOT / "dist/kubePeep").resolve()
    if not binary.is_file():
        raise RuntimeError("build dist/kubePeep before running the Phase 3 probe")

    with tempfile.TemporaryDirectory(prefix="kubepeep-phase03-") as temporary:
        temporary_path = Path(temporary)
        environment = os.environ.copy()
        for name, folder in (
            ("HOME", "home"), ("XDG_CONFIG_HOME", "config"),
            ("XDG_DATA_HOME", "data"), ("XDG_CACHE_HOME", "cache"),
            ("XDG_RUNTIME_DIR", "runtime"),
        ):
            location = temporary_path / folder
            location.mkdir(mode=0o700)
            environment[name] = str(location)
        kubeconfig = temporary_path / "kubeconfig"
        write_restricted_kubeconfig(kubeconfig)
        environment["KUBECONFIG"] = str(kubeconfig)

        with (temporary_path / "process.log").open("wb") as log:
            process = subprocess.Popen(
                [str(binary), "start", "--no-browser", "--kubeconfig", str(kubeconfig), "--context", CONTEXT, "--namespace", NAMESPACES[0]],
                env=environment, stdout=log, stderr=subprocess.STDOUT,
            )
            try:
                origin = wait_for_origin(binary, environment, process)
                client = Client(origin)
                status, _ = client.bootstrap()
                status = select_context(client, status)
                scope = create_scope(client, status, "phase03-medium", "list", NAMESPACES, NAMESPACES[0])
                status = select_scope(client, status, scope)
                if status["selection"].get("namespaceCount") != 50:
                    raise RuntimeError("the Phase 3 medium scope did not resolve all 50 namespaces")
                browser_environment = os.environ.copy()
                browser_environment["KUBEPEEP_PHASE03_ORIGIN"] = origin
                subprocess.run(
                    ["rtk", "npm", "run", "test:e2e", "--", "phase03-real.spec.ts", "--workers=1"],
                    cwd=ROOT / "web", env=browser_environment, check=True,
                )
            finally:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=10)


if __name__ == "__main__":
    main()
