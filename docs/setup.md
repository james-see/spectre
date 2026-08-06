# Spectre setup notes

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
