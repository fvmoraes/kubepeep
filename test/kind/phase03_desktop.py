#!/usr/bin/env python3
"""Measure native Wails window and shell startup with an isolated empty profile."""

from __future__ import annotations

import json
import os
import re
import signal
import subprocess
import tempfile
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
BINARY = ROOT / "build/bin/dist/desktop/kubePeep"
WINDOW = re.compile(r'^\s*(0x[0-9a-f]+) "KubePeep":', re.MULTILINE)
SHELL = re.compile(r'"operation":"shell_ready","duration_ms":([0-9.]+)')


def windows() -> set[str]:
    output = subprocess.check_output(["rtk", "proxy", "xwininfo", "-root", "-tree"], text=True)
    return set(WINDOW.findall(output))


def measure_once() -> dict[str, float]:
    with tempfile.TemporaryDirectory(prefix="kubepeep-phase03-desktop-") as temporary:
        folder = Path(temporary)
        environment = os.environ.copy()
        environment["GDK_BACKEND"] = "x11"
        environment["MESA_SHADER_CACHE_DISABLE"] = "true"
        for name, subdirectory in (
            ("HOME", "home"), ("XDG_CONFIG_HOME", "config"),
            ("XDG_DATA_HOME", "data"), ("XDG_CACHE_HOME", "cache"),
        ):
            location = folder / subdirectory
            location.mkdir(mode=0o700)
            environment[name] = str(location)
        kubeconfig = folder / "kubeconfig"
        kubeconfig.write_text("apiVersion: v1\nkind: Config\npreferences: {}\nclusters: []\ncontexts: []\nusers: []\n", encoding="utf-8")
        kubeconfig.chmod(0o600)
        existing = windows()
        log_path = folder / "desktop.log"
        with log_path.open("wb") as log:
            started = time.monotonic()
            process = subprocess.Popen([str(BINARY), "--kubeconfig", str(kubeconfig)], env=environment, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            window_ms: float | None = None
            shell_ms: float | None = None
            try:
                deadline = started + 10
                while time.monotonic() < deadline:
                    if process.poll() is not None:
                        raise RuntimeError(f"desktop exited before startup: {log_path.read_text(errors='replace')[:2500]}")
                    if window_ms is None and windows() - existing:
                        window_ms = (time.monotonic() - started) * 1_000
                    match = SHELL.search(log_path.read_text(errors="replace"))
                    if match:
                        shell_ms = float(match.group(1))
                    if window_ms is not None and shell_ms is not None:
                        return {"windowMs": round(window_ms, 1), "shellMs": shell_ms}
                    time.sleep(0.02)
                tree = subprocess.check_output(["rtk", "proxy", "xwininfo", "-root", "-tree"], text=True)
                matching_windows = [line for line in tree.splitlines() if re.search("kube|wails", line, re.I)]
                raise RuntimeError(f"native startup did not complete (window_ms={window_ms}, shell_ms={shell_ms}, matching_windows={matching_windows}): {log_path.read_text(errors='replace')[-2000:]}")
            finally:
                os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=5)
                time.sleep(0.1)


def main() -> None:
    if not BINARY.is_file():
        raise RuntimeError("build the native Wails application before measuring startup")
    for index in range(6):
        result = measure_once()
        print(json.dumps({"sample": index, "warmup": index == 0, **result}), flush=True)


if __name__ == "__main__":
    main()
