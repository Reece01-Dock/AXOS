# Hardware: ASUS ROG Rapture GT-AX6000

Reference notes for the target device. Verify anything marked *(verify)* against
the actual unit — some details vary by hardware revision.

## SoC / platform

| Component | Detail |
|---|---|
| SoC | Broadcom **BCM4912** (HND platform, 5.04 SDK line) |
| CPU | Quad-core ARMv8 (aarch64) @ 2.0 GHz |
| RAM | 1 GB DDR |
| Flash | 256 MB NAND *(verify exact layout with `cat /proc/mtd`)* |
| Wi-Fi radios | 2.4 GHz 4×4 (AX up to ~1148 Mbps) + 5 GHz 4×4 160 MHz (AX up to ~4804 Mbps) — Broadcom, proprietary driver (`wl`) |
| Switch/Ethernet | Integrated + external PHYs; **2× 2.5GbE** (one WAN, one LAN), **4× 1GbE LAN** |
| USB | 1× USB 3.2 Gen 1, 1× USB 2.0 |
| Acceleration | Broadcom **Flow Cache / Archer / runner** hardware NAT and flow acceleration — proprietary, must be preserved |

Implications:

- **aarch64**: AXOS userspace (Go binaries) cross-compiles with
  `GOOS=linux GOARCH=arm64`. Static binaries, no libc dependency issues.
- **1 GB RAM**: comfortable for a Go daemon + monitoring DB, but historical data
  and logs go to USB storage, not RAM/flash.
- **NAND flash**: limited write endurance. JFFS is fine for small config;
  anything chatty (metrics, logs, packages) lives on the USB SSD.

## Merlin support

The GT-AX6000 is supported by Asuswrt-Merlin on the **3004.388.x** firmware line
(source: `RMerl/asuswrt-merlin.ng`). Its build lives in the HND 5.04 source tree
(`release/src-rt-5.04axhnd.675x` — confirm the exact directory in the checkout;
see `docs/build-environment.md`).

## Useful on-device inspection commands (stock Merlin, SSH)

```sh
nvram get productid          # GT-AX6000
nvram get firmver; nvram get buildno; nvram get extendno
cat /proc/cpuinfo            # 4x aarch64 cores
free                          # RAM
cat /proc/mtd                # flash partition layout — SAVE THIS OUTPUT
nvram show | wc -c           # nvram usage
ip -d link                   # interfaces; identify the 2.5GbE ports
ethctl phy ...               # Broadcom PHY tool (verify availability)
wl -i eth6 status; wl -i eth7 status   # radio status (interface names: verify)
fc status                    # flow cache state (hardware acceleration)
archerctl status             # Archer acceleration (verify availability)
cat /sys/class/thermal/thermal_zone*/temp   # temperatures
```

> **Before any custom flash**: capture and commit (to `docs/device-facts/`, on a
> private branch if sensitive) the output of the commands above plus
> `nvram show`, so we have ground truth for interface names, MTD layout and
> acceleration status. Several docs in this repo mark items *(verify)* that this
> capture will resolve.

## Interface naming (to be confirmed on device)

On BCM4912 HND devices, Ethernet ports typically appear as `eth0`–`eth5` (one of
which is the 2.5G WAN and one the 2.5G LAN) and Wi-Fi radios as `eth6`/`eth7`
(wl0/wl1), with `br0` as the LAN bridge. **Do not hardcode these anywhere** —
`axosd` must discover interfaces at runtime (`ip -j link`, `nvram get wan_ifname`,
`nvram get lan_ifnames`), and the mapping gets recorded in the device-facts
capture.
