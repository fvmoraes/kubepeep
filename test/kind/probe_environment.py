"""Explicit connection settings for opt-in real Kubernetes browser probes."""

from __future__ import annotations

import os
import re
import subprocess
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class KindSource:
    kubeconfig: Path
    context: str

    def command(self, *args: str) -> str:
        return subprocess.check_output(
            ["kubectl", "--kubeconfig", str(self.kubeconfig), "--context", self.context, *args],
            text=True, stderr=subprocess.DEVNULL,
        ).strip()


def read_kind_source() -> KindSource:
    value = os.environ.get("KUBEPEEP_KIND_KUBECONFIG", "")
    if not value:
        raise RuntimeError(
            "set KUBEPEEP_KIND_KUBECONFIG to a dedicated Kind kubeconfig; ambient kubeconfigs are never used"
        )
    kubeconfig = Path(value).expanduser().resolve()
    if not kubeconfig.is_file():
        raise RuntimeError("KUBEPEEP_KIND_KUBECONFIG must name an existing regular file")
    cluster = os.environ.get("KUBEPEEP_KIND_CLUSTER", "kubepeep-f4")
    if len(cluster) > 63 or not re.fullmatch(r"[a-z0-9](?:[a-z0-9-]*[a-z0-9])?", cluster):
        raise RuntimeError("KUBEPEEP_KIND_CLUSTER must be a lowercase DNS label")
    return KindSource(kubeconfig, f"kind-{cluster}")
