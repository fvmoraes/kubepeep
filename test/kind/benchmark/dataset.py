#!/usr/bin/env python3
"""Emit one deterministic, explicit-only Kubernetes benchmark dataset.

JSON documents are emitted because JSON is a strict YAML subset. The harness
writes stdout atomically and never applies the result automatically.
"""

from __future__ import annotations

import argparse
import json
import sys
from collections.abc import Iterator
from typing import Any

MANAGED_BY = "kubepeep-benchmark-lab"
SYSTEM_NAMESPACE = "kp-benchmark-system"
PAUSE_IMAGE = "registry.k8s.io/pause:3.10@sha256:ee6521f290b2168b6e0935a181d4cff9be1ac3f505666ef0e3c98fae8199917a"
NAMESPACE_COUNTS = (1, 10, 25, 50, 100, 200)
POD_COUNTS = (10, 50, 100, 250, 500)


def labels(**extra: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/managed-by": MANAGED_BY,
        "app.kubernetes.io/part-of": "kubepeep-performance-benchmark",
        **extra,
    }


def metadata(name: str, namespace: str | None = None, **extra_labels: str) -> dict[str, Any]:
    value: dict[str, Any] = {"name": name, "labels": labels(**extra_labels)}
    if namespace:
        value["namespace"] = namespace
    return value


def service_account(name: str) -> dict[str, Any]:
    return {
        "apiVersion": "v1",
        "kind": "ServiceAccount",
        "metadata": metadata(name, SYSTEM_NAMESPACE, **{"kubepeep.dev/profile": name}),
        "automountServiceAccountToken": False,
    }


def role(name: str, namespace: str, watch: bool) -> dict[str, Any]:
    verbs = ["get", "list", "watch"] if watch else ["get", "list"]
    return {
        "apiVersion": "rbac.authorization.k8s.io/v1",
        "kind": "Role",
        "metadata": metadata(name, namespace),
        "rules": [{"apiGroups": [""], "resources": ["pods"], "verbs": verbs}],
    }


def role_binding(name: str, namespace: str, role_name: str, subject: str) -> dict[str, Any]:
    return {
        "apiVersion": "rbac.authorization.k8s.io/v1",
        "kind": "RoleBinding",
        "metadata": metadata(name, namespace),
        "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": role_name},
        "subjects": [{"kind": "ServiceAccount", "name": subject, "namespace": SYSTEM_NAMESPACE}],
    }


def pod(namespace: str, namespace_index: int, pod_index: int) -> dict[str, Any]:
    return {
        "apiVersion": "v1",
        "kind": "Pod",
        "metadata": metadata(
            f"pod-{pod_index:06d}",
            namespace,
            **{
                "kubepeep.dev/namespace-index": str(namespace_index),
                "kubepeep.dev/pod-band": str(pod_index % 10),
            },
        ),
        "spec": {
            "automountServiceAccountToken": False,
            "terminationGracePeriodSeconds": 0,
            "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
            "containers": [
                {
                    "name": "pause",
                    "image": PAUSE_IMAGE,
                    "imagePullPolicy": "IfNotPresent",
                    "resources": {
                        "requests": {"cpu": "1m", "memory": "2Mi"},
                        "limits": {"cpu": "10m", "memory": "8Mi"},
                    },
                    "securityContext": {
                        "allowPrivilegeEscalation": False,
                        "readOnlyRootFilesystem": True,
                        "privileged": False,
                        "capabilities": {"drop": ["ALL"]},
                    },
                }
            ],
        },
    }


def documents(namespace_count: int, pods_per_namespace: int) -> Iterator[dict[str, Any]]:
    yield {
        "apiVersion": "v1",
        "kind": "Namespace",
        "metadata": metadata(SYSTEM_NAMESPACE, **{"kubepeep.dev/dataset": f"{namespace_count}x{pods_per_namespace}"}),
    }
    for name in ("global-viewer", "namespace-viewer", "mixed-viewer", "no-watch-viewer"):
        yield service_account(name)
    yield {
        "apiVersion": "rbac.authorization.k8s.io/v1",
        "kind": "ClusterRole",
        "metadata": metadata("kp-benchmark-global-reader"),
        "rules": [
            {"apiGroups": [""], "resources": ["pods"], "verbs": ["get", "list", "watch"]},
            {"apiGroups": [""], "resources": ["namespaces"], "verbs": ["get", "list"]},
        ],
    }
    yield {
        "apiVersion": "rbac.authorization.k8s.io/v1",
        "kind": "ClusterRoleBinding",
        "metadata": metadata("kp-benchmark-global-reader"),
        "roleRef": {
            "apiGroup": "rbac.authorization.k8s.io",
            "kind": "ClusterRole",
            "name": "kp-benchmark-global-reader",
        },
        "subjects": [{"kind": "ServiceAccount", "name": "global-viewer", "namespace": SYSTEM_NAMESPACE}],
    }

    for namespace_index in range(namespace_count):
        namespace = f"kp-bench-{namespace_index:04d}"
        yield {
            "apiVersion": "v1",
            "kind": "Namespace",
            "metadata": metadata(
                namespace,
                **{
                    "kubepeep.dev/dataset": f"{namespace_count}x{pods_per_namespace}",
                    "kubepeep.dev/mixed-access": "allowed" if namespace_index % 2 == 0 else "unbound",
                },
            ),
        }
        yield role("kp-benchmark-read-watch", namespace, watch=True)
        yield role("kp-benchmark-read-only", namespace, watch=False)
        yield role_binding("kp-benchmark-namespace-viewer", namespace, "kp-benchmark-read-watch", "namespace-viewer")
        yield role_binding("kp-benchmark-no-watch-viewer", namespace, "kp-benchmark-read-only", "no-watch-viewer")
        if namespace_index % 2 == 0:
            yield role_binding("kp-benchmark-mixed-viewer", namespace, "kp-benchmark-read-watch", "mixed-viewer")
        for pod_index in range(pods_per_namespace):
            yield pod(namespace, namespace_index, pod_index)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="emit a deterministic KubePeep benchmark dataset to stdout")
    parser.add_argument("--namespaces", type=int, choices=NAMESPACE_COUNTS, default=10)
    parser.add_argument("--pods-per-namespace", type=int, choices=POD_COUNTS, default=100)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    count = 0
    try:
        for document in documents(args.namespaces, args.pods_per_namespace):
            if count:
                sys.stdout.write("---\n")
            json.dump(document, sys.stdout, sort_keys=True, separators=(",", ":"))
            sys.stdout.write("\n")
            count += 1
    except BrokenPipeError:
        return 0
    print(
        f"benchmark-dataset: emitted {args.namespaces} namespaces x "
        f"{args.pods_per_namespace} pods ({count} objects); not applied",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
