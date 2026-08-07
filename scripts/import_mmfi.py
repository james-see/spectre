#!/usr/bin/env python3
"""Import MM-Fi (or MmFiDataset-layout) CSI + COCO-17 keypoints into Spectre recordings.

Produces for each action/sequence:
  data/recordings/mmfi_<id>.csi.jsonl
  data/recordings/mmfi_<id>.pose.jsonl
  data/recordings/mmfi_<id>.meta.json

Official MM-Fi tree (optional):
  ${MMFI_ROOT}/E01/S01/A01/wifi-csi/*.mat  + keypoint .npy nearby

RuView-flat layout (preferred for small slices):
  ${ROOT}/S01/A01/csi.npy|csi_amplitude.npy + labels.npy|keypoints.npy

CSI is pooled/resampled to 56 amplitude bins (ESP32-ish width). Keypoints stay
normalized [0,1] when already normalized; pixel coords are scaled by image size
if values look like pixels (max > 2).

Usage:
  python3 scripts/import_mmfi.py --root ~/data/MMFi --env E01 --max-sequences 8
  python3 scripts/import_mmfi.py --root ~/data/mmfi_flat --max-sequences 20
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path

import numpy as np

COCO_NAMES = [
    "nose",
    "left_eye",
    "right_eye",
    "left_ear",
    "right_ear",
    "left_shoulder",
    "right_shoulder",
    "left_elbow",
    "right_elbow",
    "left_wrist",
    "right_wrist",
    "left_hip",
    "right_hip",
    "left_knee",
    "right_knee",
    "left_ankle",
    "right_ankle",
]

TARGET_SC = 56
DT_MS = 50.0  # MM-Fi ~20 Hz


def resample_amp(row: np.ndarray, n_out: int = TARGET_SC) -> list[float]:
    row = np.asarray(row, dtype=np.float64).ravel()
    if row.size == 0:
        return [0.0] * n_out
    if row.size == n_out:
        return row.tolist()
    x_old = np.linspace(0.0, 1.0, num=row.size)
    x_new = np.linspace(0.0, 1.0, num=n_out)
    return np.interp(x_new, x_old, row).tolist()


def pool_csi_to_rows(arr: np.ndarray) -> np.ndarray:
    """Reduce CSI tensor to [T, n_sc] amplitudes."""
    a = np.asarray(arr, dtype=np.float64)
    if a.ndim == 2:
        return a
    if a.ndim == 3:
        # [T, ant, sc] or [T, tx*rx, sc]
        return np.mean(np.abs(a), axis=1)
    if a.ndim == 4:
        # [T, tx, rx, sc]
        return np.mean(np.abs(a), axis=(1, 2))
    raise ValueError(f"unsupported CSI ndim={a.ndim} shape={a.shape}")


def load_npy(path: Path) -> np.ndarray:
    return np.load(str(path), allow_pickle=False)


def load_mat_csi(path: Path) -> np.ndarray | None:
    try:
        from scipy.io import loadmat  # type: ignore
    except ImportError:
        print("scipy not installed; skipping .mat:", path, file=sys.stderr)
        return None
    mat = loadmat(str(path))
    for key, val in mat.items():
        if key.startswith("_"):
            continue
        if isinstance(val, np.ndarray) and val.size > 100:
            return np.asarray(val)
    return None


def normalize_keypoints(kps: np.ndarray) -> np.ndarray:
    """Return [T, 17, 3] with x,y in ~[0,1], z passthrough or 0."""
    k = np.asarray(kps, dtype=np.float64)
    if k.ndim == 2 and k.shape[1] == 51:
        k = k.reshape(-1, 17, 3)
    elif k.ndim == 2 and k.shape[1] == 34:
        # [T, 17*2] → add z=0
        k = k.reshape(-1, 17, 2)
        z = np.zeros((k.shape[0], 17, 1))
        k = np.concatenate([k, z], axis=-1)
    elif k.ndim == 3 and k.shape[1] == 17 and k.shape[2] == 2:
        z = np.zeros((k.shape[0], 17, 1))
        k = np.concatenate([k, z], axis=-1)
    elif k.ndim == 3 and k.shape[1] == 17 and k.shape[2] >= 3:
        k = k[:, :, :3]
    else:
        raise ValueError(f"unsupported keypoint shape {k.shape}")

    xy = k[:, :, :2]
    if float(np.nanmax(np.abs(xy))) > 2.0:
        # assume pixel space on ~640x480
        k = k.copy()
        k[:, :, 0] = np.clip(k[:, :, 0] / 640.0, 0.0, 1.0)
        k[:, :, 1] = np.clip(k[:, :, 1] / 480.0, 0.0, 1.0)
    else:
        k = k.copy()
        k[:, :, 0] = np.clip(k[:, :, 0], 0.0, 1.0)
        k[:, :, 1] = np.clip(k[:, :, 1], 0.0, 1.0)
    return k


def find_first(dir_path: Path, names: list[str]) -> Path | None:
    for n in names:
        p = dir_path / n
        if p.is_file():
            return p
    return None


def write_recording(
    out_dir: Path,
    rec_id: str,
    csi_rows: np.ndarray,
    kps: np.ndarray,
    origin: str,
) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    csi_path = out_dir / f"{rec_id}.csi.jsonl"
    pose_path = out_dir / f"{rec_id}.pose.jsonl"
    meta_path = out_dir / f"{rec_id}.meta.json"

    t = min(csi_rows.shape[0], kps.shape[0])
    with csi_path.open("w", encoding="utf-8") as fc, pose_path.open(
        "w", encoding="utf-8"
    ) as fp:
        for i in range(t):
            amps = resample_amp(csi_rows[i])
            ts = (i * DT_MS) / 1000.0
            fc.write(
                json.dumps(
                    {
                        "timestamp": ts,
                        "subcarriers": amps,
                        "rssi": -50.0,
                        "noise_floor": -90.0,
                        "features": {"origin": origin, "teacher": "mmfi"},
                    },
                    separators=(",", ":"),
                )
                + "\n"
            )
            kpline = []
            for j, name in enumerate(COCO_NAMES):
                x, y, z = (float(kps[i, j, 0]), float(kps[i, j, 1]), float(kps[i, j, 2]))
                conf = 0.0 if (math.isnan(x) or math.isnan(y)) else 1.0
                kpline.append(
                    {
                        "name": name,
                        "x": x,
                        "y": y,
                        "z": z,
                        "confidence": conf,
                    }
                )
            fp.write(
                json.dumps(
                    {
                        "t_ms": i * DT_MS,
                        "keypoints": kpline,
                        "source": "mmfi",
                    },
                    separators=(",", ":"),
                )
                + "\n"
            )

    meta = {
        "id": rec_id,
        "session_name": rec_id,
        "label": "pose",
        "status": "completed",
        "frames": t,
        "pose_frames": t,
        "has_pose": True,
        "teacher": "mmfi",
        "origin": origin,
        "csi_adapter": "mmfi_pool56",
        "format": "csi",
    }
    meta_path.write_text(json.dumps(meta, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {rec_id} frames={t}")


def import_flat_action(action_dir: Path, out_dir: Path, prefix: str) -> bool:
    amp_path = find_first(action_dir, ["csi_amplitude.npy", "csi.npy", "wifi_csi.npy"])
    lab_path = find_first(
        action_dir, ["labels.npy", "keypoints.npy", "gt_keypoints.npy"]
    )
    if not amp_path or not lab_path:
        return False
    csi = pool_csi_to_rows(load_npy(amp_path))
    kps = normalize_keypoints(load_npy(lab_path))
    rel = f"{action_dir.parent.name}_{action_dir.name}"
    rec_id = f"{prefix}_{rel}".replace("/", "_")
    write_recording(out_dir, rec_id, csi, kps, origin=str(action_dir))
    return True


def import_official_action(action_dir: Path, out_dir: Path) -> bool:
    """E01/S01/A01 style folder."""
    wifi = action_dir / "wifi-csi"
    # keypoints often under rgb/ or root
    lab = None
    for cand in [
        action_dir / "gt_keypoints.npy",
        action_dir / "keypoints.npy",
        action_dir / "labels.npy",
        action_dir / "rgb" / "keypoints.npy",
        action_dir / "rgb" / "pose.npy",
    ]:
        if cand.is_file():
            lab = cand
            break
    if lab is None:
        # scan rgb for *keypoint*.npy
        rgb = action_dir / "rgb"
        if rgb.is_dir():
            for p in sorted(rgb.glob("*.npy")):
                if "key" in p.name.lower() or "pose" in p.name.lower():
                    lab = p
                    break
    if lab is None:
        return False

    csi_arr = None
    if wifi.is_dir():
        for p in sorted(wifi.glob("*.npy")):
            csi_arr = pool_csi_to_rows(load_npy(p))
            break
        if csi_arr is None:
            for p in sorted(wifi.glob("*.mat")):
                raw = load_mat_csi(p)
                if raw is not None:
                    csi_arr = pool_csi_to_rows(raw)
                    break
    if csi_arr is None:
        return False

    kps = normalize_keypoints(load_npy(lab))
    parts = action_dir.parts
    # .../E01/S01/A01
    tag = "_".join(parts[-3:]) if len(parts) >= 3 else action_dir.name
    rec_id = f"mmfi_{tag}"
    write_recording(out_dir, rec_id, csi_arr, kps, origin=str(action_dir))
    return True


def iter_sequence_dirs(root: Path, env: str | None) -> list[Path]:
    dirs: list[Path] = []
    # Official: E01/S01/A01
    envs = [root / env] if env else sorted(root.glob("E*"))
    for e in envs:
        if not e.is_dir():
            continue
        for s in sorted(e.glob("S*")):
            if not s.is_dir():
                continue
            for a in sorted(s.glob("A*")):
                if a.is_dir():
                    dirs.append(a)
    if dirs:
        return dirs
    # Flat: S01/A01
    for s in sorted(root.glob("S*")):
        if not s.is_dir():
            continue
        for a in sorted(s.glob("A*")):
            if a.is_dir():
                dirs.append(a)
    if dirs:
        return dirs
    # Single directory with npy pair
    if find_first(root, ["csi.npy", "csi_amplitude.npy"]):
        return [root]
    return []


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--root", type=Path, required=True, help="MM-Fi root or flat subject root")
    ap.add_argument(
        "--out",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "data" / "recordings",
    )
    ap.add_argument("--env", type=str, default=None, help="Limit to E01..E04")
    ap.add_argument("--max-sequences", type=int, default=16)
    ap.add_argument("--prefix", type=str, default="mmfi")
    args = ap.parse_args()

    if not args.root.is_dir():
        print(f"root not found: {args.root}", file=sys.stderr)
        return 1

    seqs = iter_sequence_dirs(args.root, args.env)
    if not seqs:
        print("no sequences found under", args.root, file=sys.stderr)
        return 1

    n_ok = 0
    for d in seqs:
        if n_ok >= args.max_sequences:
            break
        ok = import_official_action(d, args.out) or import_flat_action(
            d, args.out, args.prefix
        )
        if ok:
            n_ok += 1
        else:
            print(f"skip (missing csi/keypoints): {d}", file=sys.stderr)

    print(f"imported {n_ok} sequences → {args.out}")
    return 0 if n_ok else 2


if __name__ == "__main__":
    raise SystemExit(main())
