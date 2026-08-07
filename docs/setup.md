# Spectre setup notes

## Fetch firmware

Flash images are **not** baked into the flash script. Download them from
upstream RuView ESP32 releases:

```bash
./scripts/fetch_firmware.sh              # latest *-esp32 GitHub release
./scripts/fetch_firmware.sh --list       # show tags
./scripts/fetch_firmware.sh v0.8.4-esp32 # pin a tag
```

What it does:

1. Resolves the newest non-draft `*esp32*` release on `ruvnet/RuView` (or uses your tag).
2. Downloads the **8MB flash bundle** zip when available (`bootloader`, `partition-table`, `ota_data_initial`, app).
3. Normalizes the app binary to `firmware/esp32-csi-node.bin` (what `flash_node.sh` expects).
4. Optionally pulls a 4MB app binary when the release includes one.
5. Writes `firmware/SOURCE.txt` with repo, tag, and timestamp.

Requires: `curl`, `unzip`, `python3`. Override repo with `RUVIEW_REPO=owner/name` if needed.

After fetching, flash as usual:

```bash
./scripts/flash_node.sh /dev/cu.wchusbserialXXXX 1
```

## Ports on Mac

| Device | Typical path | Use |
|--------|--------------|-----|
| CH340 / external UART | `/dev/cu.wchusbserial*` | Flash + provision |
| SiLabs CP210x (DevKit UART) | `/dev/cu.SLAB_USBtoUART` | Flash + provision |
| Native USB-JTAG | `/dev/cu.usbmodem*` | Console after boot (optional) |

Charge-only cables enumerate nothing. Prefer a known data cable on the UART path.

## Dual-cable DevKitC-1

Some DevKitC-1 boards do not power from the UART port. Use:

1. **USB** port → power  
2. **UART** (or external CH340) → flash  

Flash only the UART/CH340 device.

## Triad layout

```
NODE_ID 1  tdm-slot 0
NODE_ID 2  tdm-slot 1
NODE_ID 3  tdm-slot 2
TDM_TOTAL=3
```

Spread diagonally across the room; avoid a single line.

## Credentials

Never commit `.env`. Provisioning writes SSID/password into device NVS only.

## Aggregator on OrbStack

```bash
# ensure OrbStack is running (docker context: orbstack)
docker compose -f ~/p/spectre/docker-compose.yml up -d
open http://127.0.0.1:3000/ui/index.html
```

- HTTP/WS: `127.0.0.1:3000` / `:3001`
- CSI UDP: host `:5005` (must match `TARGET_IP` / `TARGET_PORT` in `.env`)
- `CSI_SOURCE=esp32`, unauthenticated bind allowed for local lab use

Stop: `docker compose -f ~/p/spectre/docker-compose.yml down`
