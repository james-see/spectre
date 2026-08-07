#!/usr/bin/env python3
"""Write a tiny synthetic MM-Fi-layout slice for Spectre import smoke tests."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import numpy as np

sys.path.insert(0, str(Path(__file__).resolve().parent))
from import_mmfi import write_recording  # noqa: E402


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--out",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "data" / "recordings",
    )
    ap.add_argument("--frames", type=int, default=120)
    args = ap.parse_args()

    t = args.frames
    # Fake CSI [T, 114] with slow modulation
    sc = 114
    phase = np.linspace(0, 4 * np.pi, t)[:, None]
    csi = 2.0 + 0.5 * np.sin(phase + np.linspace(0, 1, sc)[None, :])

    # Stick figure walking in normalized coords
    kps = np.zeros((t, 17, 3), dtype=np.float64)
    base = np.array(
        [
            [0.50, 0.18, 0],  # nose
            [0.48, 0.16, 0],
            [0.52, 0.16, 0],
            [0.46, 0.17, 0],
            [0.54, 0.17, 0],
            [0.42, 0.30, 0],  # shoulders
            [0.58, 0.30, 0],
            [0.38, 0.42, 0],
            [0.62, 0.42, 0],
            [0.36, 0.54, 0],
            [0.64, 0.54, 0],
            [0.45, 0.55, 0],  # hips
            [0.55, 0.55, 0],
            [0.44, 0.72, 0],
            [0.56, 0.72, 0],
            [0.43, 0.90, 0],
            [0.57, 0.90, 0],
        ]
    )
    for i in range(t):
        swing = 0.04 * np.sin(i / 8.0)
        kps[i] = base
        kps[i, 9, 0] += swing  # wrists / ankles
        kps[i, 10, 0] -= swing
        kps[i, 15, 0] -= swing
        kps[i, 16, 0] += swing
        kps[i, :, 0] = np.clip(kps[i, :, 0], 0, 1)
        kps[i, :, 1] = np.clip(kps[i, :, 1], 0, 1)

    write_recording(args.out, "mmfi_fixture_S01_A01", csi, kps, origin="fixture")
    write_recording(args.out, "mmfi_fixture_S01_A02", csi, kps, origin="fixture")
    print(f"fixture → {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
