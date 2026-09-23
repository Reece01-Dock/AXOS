# Flashing & recovery — GT-AX6000

**Rule zero: verify recovery works *before* flashing anything custom.**
A custom firmware project without a proven recovery path is a brick generator.

## Recovery paths (verify all of these on stock firmware first)

1. **Bootloader rescue mode (primary recovery).**
   - Power off. Hold **RESET**. Power on while holding RESET until the power LED
     blinks slowly, then release.
   - The bootloader brings up a minimal network stack on **192.168.1.1**.
   - Set your PC to a static IP (e.g. 192.168.1.10/24), connect to a LAN port.
   - Restore firmware either via the bootloader's **mini web server**
     (browse to `http://192.168.1.1`) or the **ASUS Firmware Restoration tool**
     (Windows). On this HND generation the mini web server is the usual method —
     confirm which works during the verification pass and record it here.
   - Flash a **stock ASUS** image kept locally for this purpose.

2. **Factory defaults / NVRAM reset.**
   - Web UI: Administration → Restore/Save/Upload Setting → Factory default, or
   - Hold **WPS** while powering on until power LED flashes, then release
     (hard NVRAM reset) — verify the exact procedure on this model and record it.

3. **Dual-boot / previous image**: the GT-AX6000 stores firmware in flash with
   bootloader-managed images (*verify*: check `cat /proc/mtd` and bootloader
   output for evidence of dual image slots on this model). If dual-image exists,
   document how the bootloader selects/falls back.

### Recovery kit (keep it ready before every flash)

- Latest **stock ASUS** firmware file for GT-AX6000, downloaded and checksummed.
- Latest known-good **Merlin release** image.
- ASUS Firmware Restoration tool installed on a Windows machine/VM.
- A wired laptop with a static-IP profile saved (192.168.1.10/24).
- Printed/offline copy of this page (the router being down means no internet).
- Settings backup (`Administration → Restore/Save/Upload Setting`) **and** a
  JFFS backup, taken before the flash.

## Normal flash procedure

1. Take settings + JFFS backups; note current firmware version.
2. Flash via web UI: Administration → Firmware Upgrade → upload image.
3. Wait for it to fully reboot (be patient — first boot after flash is slower).
4. Run the smoke checklist below.
5. Any failure → restore previous image via recovery, then diagnose offline.

## Post-flash smoke checklist (Milestone 1 steps 4–10)

```
[ ] Router boots (power LED steady, LAN link lights)
[ ] Web UI reachable and responsive
[ ] SSH login works
[ ] `nvram get buildno` shows the expected version
[ ] WAN gets an address; internet works from a wired client
[ ] 1GbE LAN ports pass traffic
[ ] Both 2.5GbE ports link at 2.5G (check `ethctl`/link partner) and pass traffic
[ ] 2.4 GHz SSID visible, client connects, passes traffic
[ ] 5 GHz SSID visible, client connects at expected PHY rate, passes traffic
[ ] `fc status` (flow cache) shows hardware acceleration enabled
[ ] Temperatures sane (`/sys/class/thermal`)
```

## Flash log

Record every flash here (date, image, source ref, result). This table is part of
the safety story — keep it honest.

| Date | Image | Built from (repo ref / patches) | Result |
|---|---|---|---|
| 2026-09-23 | `GT-AX6000_3006_102.9_beta1_nand_squashfs.pkgtb` (70,443,084 B, sha256 `2d8c403ea2252e13bbe857450fc10778894b9c17b0d7c156a54621f2d500b85b`) | Stock Merlin `3006.102-wifi6` @ `d832d71c8b…`, **no AXOS patches**; flashed via web UI from ASUS stock `3.0.0.6.102_37436` | **PASS** — boots, web UI, SSH (LAN, port 22), WAN PPPoE + internet, both Wi-Fi bands up (`Reece-Net`), flow cache L2/L3 on, privacy CGI present in `httpd`. 2.5GbE PHY present (eth5) but no cable linked at flash time. |
| 2026-09-23 | `GT-AX6000_3006_102.9_beta1_nand_squashfs.pkgtb` (70,443,084 B, sha256 `193be109f44987fb52486613cb4c0b1fa0ebc607290005c8d822386df1ca84e6`) | Same wifi6 commit + **only** `0001-axos-login-title-marker.patch`; flashed via web UI from prior stock Merlin | **PASS** — boots, SSH, login tab title **`ASUS Login (AXOS)`** (M1 step 16). Backup at `AXOS-backups/pre-axos-marker-20260923T194612Z/`. |
| — | — | — | — |
