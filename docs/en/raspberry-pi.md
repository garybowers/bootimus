# Raspberry Pi Netboot

Netboot Raspberry Pi 3 and 4 family boards with no SD card. Bootimus embeds everything the Pi's firmware asks for and hands the board straight into the standard UEFI/iPXE boot menu.

## Table of Contents

- [Supported boards](#supported-boards)
- [How it works](#how-it-works)
- [One-time board preparation](#one-time-board-preparation)
- [DHCP setup](#dhcp-setup)
- [Per-machine overrides](#per-machine-overrides)
- [Limitations](#limitations)

## Supported boards

| Board | Status |
|-------|--------|
| Pi 4B, Pi 400, CM4 | ✅ Supported |
| Pi 3B, 3B+, CM3 | ✅ Supported |
| Pi 5 | ❌ No mature UEFI (EDK2) port yet |
| Pi 2 and earlier, Zero | ❌ No netboot-capable firmware |

## How it works

The Pi's boot ROM/EEPROM fetches its firmware over TFTP, and Bootimus serves the whole chain from embedded assets — nothing to stage, nothing to flash:

```
Pi EEPROM/bootcode netboot (no SD card)
  → TFTP: bootcode.bin / start4.elf + fixup + config.txt
    → config.txt [pi3]/[pi4] filters pick the right UEFI build
      → RPI_EFI_3.fd or RPI_EFI_4.fd (Tianocore EDK2)
        → UEFI PXE → ipxe-arm64.efi → Bootimus boot menu
```

From the menu onwards a Pi is just another ARM64 UEFI client: extracted images, per-client image assignment, auto-install — everything works the same as on x86.

The firmware requests files under an `<8-hex-serial>/` path prefix first; Bootimus strips it automatically, so a whole fleet boots from one shared set of files with no per-board setup.

## One-time board preparation

The only thing Bootimus cannot do over the network is tell a Pi to *try* netbooting — that is firmware policy stored on the board:

- **Pi 4 / 400 / CM4**: set the EEPROM boot order once, e.g. via `raspi-config` (Advanced Options → Boot Order → Network Boot), or `BOOT_ORDER=0xf12` with `rpi-eeprom-config`. Newer EEPROMs fall back to network automatically when no SD card is present.
- **Pi 3B+**: network boot support is in the boot ROM — nothing to configure; just boot without an SD card.
- **Pi 3B / CM3**: boot once from an SD card with `program_usb_boot_mode=1` in `config.txt` to set the (irreversible) OTP boot bit.

## DHCP setup

The easiest path is Bootimus's built-in proxyDHCP (`--proxy-dhcp`): it recognises Raspberry Pi MAC addresses and answers the firmware with the `Raspberry Pi Boot` vendor option it requires, then serves the ARM64 UEFI stage its `ipxe-arm64.efi` bootfile — while your existing DHCP server keeps handing out IPs untouched.

With an external DHCP server instead, you need both stages covered:

1. The **firmware stage** needs `next-server` (option 66) pointing at Bootimus *and* vendor option 43 containing the string `Raspberry Pi Boot`.
2. The **UEFI stage** needs the bootfile `ipxe-arm64.efi` for client architecture 11 (UEFI ARM64).

Many DHCP GUIs (OPNsense/pfSense among them) cannot express per-architecture bootfiles or the Pi vendor option — on those networks, run the proxyDHCP alongside. See the [DHCP guide](dhcp.md).

## Per-machine overrides

To give a specific board different firmware files or a custom `config.txt`, place files in the data directory under the board's serial number:

```
<data>/raspberrypi/<serial>/config.txt      # this Pi only
<data>/raspberrypi/config.txt               # all Pis, overrides embedded copy
```

Anything not overridden falls back to the embedded assets. The serial is the 8-hex-digit value from `cat /proc/cpuinfo` on the Pi, and shows up in Bootimus's TFTP logs when the board first tries to netboot.

## Limitations

- **Wired Ethernet only** — the Pi firmware cannot netboot over WiFi.
- **UEFI settings do not persist.** The netbooted firmware has no writable variable store, so it always boots with pftf defaults — including the 3 GB RAM limit default on Pi 4. If your OS needs all the RAM, build an `RPI_EFI.fd` with the limit disabled and drop it in as an override.
- **Pi 5 is not supported** until its EDK2 port matures; the same applies to any board without UEFI firmware.
- The assets are refreshed with `scripts/build-raspberrypi-assets.sh`, which pins and checksum-verifies the [pftf](https://github.com/pftf/RPi4) UEFI releases. The Broadcom boot blobs it bundles are licensed for use with Raspberry Pi devices only (`LICENCE.broadcom.txt` ships alongside).
