# Spectre

**Wi‑Fi that sees the room.**

Spectre turns three ESP32‑S3 boards into a multistatic CSI mesh — presence, motion, and occupancy without cameras on the wire. Place nodes at the corners of a space; they stream Channel State Information to your laptop, where a sensing server fuses the field.

```
        [node 1] -------------- [node 2]
            \                    /
             \     room air     /
              \                /
               \              /
                [node 3] ··· you
                     │
                     ▼
              Spectre aggregator
```

## Why three

One node senses along a link. Two add coverage. **Three** give geometric diversity for multistatic fusion — the practical minimum for a room-scale mesh. Spectre defaults to a triad (`TDM_TOTAL=3`).

## Hardware

- 3× ESP32‑S3 (DevKitC‑1 N16R8 or similar; 8MB+ flash, PSRAM recommended)
- Data‑capable USB cables (charge‑only cables will waste your afternoon)
- For boards whose UART port does not power the MCU: power via USB + flash via UART/CH340

## Quick start

```bash
cd ~/p/spectre
cp .env.example .env
# edit WIFI_SSID / WIFI_PASS / TARGET_IP (your Mac LAN IP)

chmod +x scripts/*.sh

# Pull latest ESP32-S3 flash images from RuView (or a specific tag)
./scripts/fetch_firmware.sh
# ./scripts/fetch_firmware.sh v0.8.4-esp32
# ./scripts/fetch_firmware.sh --list

# Flash boards one at a time on the UART/serial port
./scripts/flash_node.sh /dev/cu.wchusbserialXXXX 1
./scripts/flash_node.sh /dev/cu.wchusbserialXXXX 2
./scripts/flash_node.sh /dev/cu.wchusbserialXXXX 3
```

`firmware/` may already contain bins from the last fetch/commit. Re-run
`fetch_firmware.sh` whenever you want upstream updates — it overwrites
`firmware/*.bin` and writes `firmware/SOURCE.txt` with the release tag.

Place nodes ~2–3 m apart, ~1 m off the floor, not in a straight line.

### Aggregator

Nodes UDP‑stream CSI to `TARGET_IP:5005`. Run [RuView](https://github.com/ruvnet/RuView)’s sensing server on that host:

```bash
# after installing Rust + cloning RuView
cd /path/to/RuView/v2
cargo run -p wifi-densepose-sensing-server -- --http-port 3000 --source auto
```

Open the dashboard URL the server prints (typically `http://127.0.0.1:3000`).

## What’s in this repo

| Path | Purpose |
|------|---------|
| `firmware/` | Prebuilt ESP32‑S3 CSI node images (from RuView) |
| `scripts/fetch_firmware.sh` | Download latest (or pinned) RuView ESP32 release bins |
| `scripts/flash_node.sh` | One‑command flash + NVS provision |
| `scripts/provision.py` | Wi‑Fi / node‑id / TDM NVS writer |
| `.env.example` | Credentials template (never commit `.env`) |
| `docs/setup.md` | Cables, ports, triad layout |

## Status

- Node 1 already validated on ESP32‑S3 (Wi‑Fi join + CSI stream target)
- Nodes 2–3: flash with the same script, unique `NODE_ID`

## Credits

Firmware binaries and provisioning tooling are based on **[RuView / wifi-densepose esp32-csi-node](https://github.com/ruvnet/RuView)** (MIT). Spectre is an opinionated triad deploy kit and ops layer around that stack.

## License

MIT — see [LICENSE](LICENSE). Upstream RuView remains MIT; see [THIRD_PARTY.md](THIRD_PARTY.md).
