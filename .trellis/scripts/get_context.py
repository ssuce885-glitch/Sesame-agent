#!/usr/bin/env python3
"""Minimal Trellis context discovery for this repository."""

from __future__ import annotations

import argparse
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SPEC = ROOT / ".trellis" / "spec"


def list_packages() -> None:
    entries = [
        ("backend", "Go v2 runtime, store, tools, agent, workflows", SPEC / "backend" / "index.md"),
        ("guides", "Thinking checklists for implementation reviews", SPEC / "guides" / "index.md"),
    ]
    for name, desc, index in entries:
        status = "present" if index.exists() else "missing"
        rel = index.relative_to(ROOT)
        print(f"{name}\t{status}\t{rel}\t{desc}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=["packages"], required=True)
    args = parser.parse_args()
    if args.mode == "packages":
        list_packages()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
