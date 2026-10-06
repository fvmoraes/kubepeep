#!/usr/bin/env python3
"""Check repository Markdown links without network access or extra packages."""

import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parent.parent
LINK = re.compile(r"\]\((<[^>]+>|[^\s)]+)(?:\s+\"[^\"]*\")?\)")
HEADING = re.compile(r"^ {0,3}#{1,6}\s+(.+?)(?:\s+#+)?$")


def prose_lines(text):
    """Keep line numbers while excluding fenced examples from validation."""
    fence = None
    for number, line in enumerate(text.splitlines(), 1):
        marker = re.match(r"^\s*(`{3,}|~{3,})", line)
        if marker:
            token = marker[1]
            if fence is None:
                fence = token
            elif token[0] == fence[0] and len(token) >= len(fence):
                fence = None
            continue
        if fence is None:
            yield number, line


def anchors(text):
    result = set()
    for _, line in prose_lines(text):
        result.update(re.findall(r'<a\s+(?:id|name)=["\']([^"\']+)', line))
        heading = HEADING.match(line)
        if heading:
            title = re.sub(r"\[([^]]+)\]\([^)]*\)", r"\1", heading[1])
            title = re.sub(r"<[^>]*>", "", title).lower()
            slug = re.sub(r"[^\w\- ]", "", title).replace(" ", "-")
            unique = slug
            suffix = 0
            while unique in result:
                suffix += 1
                unique = f"{slug}-{suffix}"
            result.add(unique)
    return result


def check_file(path, anchor_cache):
    errors = []
    for number, line in prose_lines(path.read_text(encoding="utf-8")):
        for match in LINK.finditer(line):
            target = match[1].strip("<>")
            url = urlsplit(target)
            if url.scheme or url.netloc:
                continue
            target_path = unquote(url.path)
            if not target_path:
                resolved = path
            elif target_path.startswith("/"):
                resolved = ROOT / target_path.lstrip("/")
            else:
                resolved = path.parent / target_path
            resolved = resolved.resolve()
            if not resolved.is_relative_to(ROOT):
                errors.append((number, target, "outside repository"))
            elif not resolved.exists():
                errors.append((number, target, "missing path"))
            elif url.fragment and resolved.suffix.lower() == ".md":
                if resolved not in anchor_cache:
                    anchor_cache[resolved] = anchors(resolved.read_text(encoding="utf-8"))
                if unquote(url.fragment) not in anchor_cache[resolved]:
                    errors.append((number, target, "missing anchor"))
    return errors


def main():
    # Include new documents before staging; ignore local tooling and build output.
    output = subprocess.check_output(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
        cwd=ROOT,
    )
    files = sorted({ROOT / name.decode() for name in output.split(b"\0") if name})
    failures = 0
    count = 0
    anchor_cache = {}
    for path in files:
        if path.suffix.lower() != ".md" or not path.is_file():
            continue
        count += 1
        for number, target, reason in check_file(path, anchor_cache):
            print(f"{path.relative_to(ROOT)}:{number}: {reason}: {target}")
            failures += 1
    print(f"docs-check: {count} documents, {failures} broken local links")
    return int(failures > 0)


if __name__ == "__main__":
    sys.exit(main())
