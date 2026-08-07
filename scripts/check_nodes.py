#!/usr/bin/env python3
"""Spectre node health: decode RuView UDP on :5005 (CSI vs DEGRADED).

Requires host-network visibility into CSI UDP. Prefer:

  docker run --rm --net=host nicolaka/netshoot \\
    tcpdump -ni eth0 udp dst port 5005 -c 200 -nn -tt -X 2>/dev/null \\
    | python3 scripts/check_nodes.py --from-tcpdump -

Or sample the sensing WebSocket (nodes[] = CSI only):

  python3 scripts/check_nodes.py --ws
"""
from __future__ import annotations

import argparse
import collections
import json
import re
import struct
import sys
import time

MAGIC_CSI = 0xC5110001
MAGIC_FEATURE = 0xC5110006
QFLAG_DEGRADED = 1 << 5
QFLAG_NAMES = {
    0: "PRESENCE",
    1: "RESP",
    2: "HR",
    3: "ANOMALY",
    4: "ENV_SHIFT",
    5: "DEGRADED",
    6: "CALIB",
    7: "RECAL",
}


def _flags_str(flags: int) -> str:
    names = [QFLAG_NAMES[i] for i in range(8) if flags & (1 << i)]
    return ",".join(names) if names else "none"


def parse_tcpdump_x(text: str) -> list[bytes]:
    """Extract UDP payloads from `tcpdump -X` text."""
    ip_re = re.compile(r"IP\s+\S+\.(\d+)\s+>\s+\S+\.5005:\s+UDP,\s+length\s+(\d+)")
    hex_re = re.compile(r"\t0x[0-9a-f]+:\s+([0-9a-f ]+)", re.I)
    cur = None
    hexbytes: list[str] = []
    payloads: list[bytes] = []

    def flush() -> None:
        nonlocal cur, hexbytes
        if not cur or not hexbytes:
            cur = None
            hexbytes = []
            return
        raw = bytes.fromhex("".join(c for c in "".join(hexbytes) if c in "0123456789abcdef"))
        ihl = (raw[0] & 0xF) * 4
        payloads.append(raw[ihl + 8 :])
        cur = None
        hexbytes = []

    for line in text.splitlines():
        m = ip_re.search(line)
        if m:
            flush()
            cur = (int(m.group(1)), int(m.group(2)))
            continue
        m = hex_re.match(line)
        if m and cur is not None:
            parts = [t for t in m.group(1).split() if re.fullmatch(r"[0-9a-fA-F]{4}", t)]
            hexbytes.append("".join(parts))
    flush()
    return payloads


def summarize_payloads(payloads: list[bytes]) -> None:
    csi = collections.Counter()
    other = collections.Counter()
    feat: dict[int, dict] = {}

    for p in payloads:
        if len(p) < 6:
            continue
        magic = struct.unpack_from("<I", p, 0)[0]
        node = p[4]
        if magic == MAGIC_CSI:
            csi[node] += 1
            continue
        if magic == MAGIC_FEATURE and len(p) >= 60:
            flags = struct.unpack_from("<H", p, 52)[0]
            motion, presence = struct.unpack_from("<ff", p, 16)
            a = feat.setdefault(
                node,
                {"n": 0, "flags_or": 0, "motion": 0.0, "presence": 0.0},
            )
            a["n"] += 1
            a["flags_or"] |= flags
            a["motion"] = motion
            a["presence"] = presence
            continue
        other[(node, hex(magic))] += 1

    nodes = sorted(set(csi) | set(feat) | {n for n, _ in other})
    if not nodes:
        print("No RuView UDP payloads decoded.")
        return

    print(f"{'node':>4}  {'CSI':>5}  {'feat':>5}  flags              status")
    ok = True
    for node in nodes:
        f = feat.get(node, {"n": 0, "flags_or": 0, "motion": 0.0, "presence": 0.0})
        flags = f["flags_or"]
        degraded = bool(flags & QFLAG_DEGRADED)
        has_csi = csi[node] > 0
        if has_csi and not degraded:
            status = "OK"
        elif has_csi and degraded:
            status = "CSI+DEGRADED"
            ok = False
        elif degraded:
            status = "DEGRADED (no CSI)"
            ok = False
        else:
            status = "no CSI"
            ok = False
        print(
            f"{node:>4}  {csi[node]:>5}  {f['n']:>5}  "
            f"0x{flags:04x}({_flags_str(flags):<14})  {status}"
        )
        if f["n"]:
            print(f"       last motion={f['motion']:.3f} presence={f['presence']:.3f}")

    if other:
        print("other magics:", dict(other))
    missing_csi = [n for n in (1, 2, 3) if csi[n] == 0]
    if missing_csi:
        print("FAIL")
        print(
            f"\nNodes missing CSI frames: {missing_csi}\n"
            "Not a TDM slot issue — those nodes are CSI-starved.\n"
            "Plug UART (CH340), monitor for: self-ping started, CSI cb, MGMT+DATA,\n"
            "then reflash: ./scripts/flash_node.sh <PORT> <NODE_ID>"
        )
        sys.exit(1)
    if not ok:
        print("FAIL")
        sys.exit(1)
    print("PASS")
    sys.exit(0)


def check_ws(seconds: float) -> None:
    try:
        import websockets  # type: ignore
    except ImportError:
        print("pip install websockets", file=sys.stderr)
        sys.exit(2)

    import asyncio

    async def run() -> None:
        hits: collections.Counter = collections.Counter()
        async with websockets.connect("ws://127.0.0.1:3001/ws/sensing") as ws:
            t0 = time.time()
            while time.time() - t0 < seconds:
                try:
                    d = json.loads(await asyncio.wait_for(ws.recv(), timeout=1))
                except Exception:
                    continue
                for n in d.get("nodes") or []:
                    hits[n.get("node_id")] += 1
        print("sensing WS nodes[] appearances (CSI-backed only):")
        for n in sorted(hits):
            print(f"  node {n}: {hits[n]}")
        missing = [n for n in (1, 2, 3) if hits[n] == 0]
        if missing:
            print(f"missing from fusion view: {missing}")
            print("Run UDP decode for DEGRADED confirmation (see --help).")
            sys.exit(1)
        print("All node_ids 1–3 present in sensing stream.")

    asyncio.run(run())


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--ws", action="store_true", help="sample sensing WebSocket")
    ap.add_argument("--seconds", type=float, default=6.0)
    ap.add_argument(
        "--from-tcpdump",
        metavar="FILE",
        help="tcpdump -X text file, or - for stdin",
    )
    args = ap.parse_args()

    if args.ws:
        check_ws(args.seconds)
        return

    if args.from_tcpdump:
        data = sys.stdin.read() if args.from_tcpdump == "-" else open(args.from_tcpdump).read()
        summarize_payloads(parse_tcpdump_x(data))
        return

    ap.print_help()
    sys.exit(2)


if __name__ == "__main__":
    main()
