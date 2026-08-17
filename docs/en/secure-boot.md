# UEFI Secure Boot

How to netboot machines that have UEFI Secure Boot enabled, without touching their firmware settings.

## Table of Contents

- [How it works](#how-it-works)
- [Enabling Secure Boot support](#enabling-secure-boot-support)
- [Booting images](#booting-images)
- [What works and what doesn't](#what-works-and-what-doesnt)
- [Advanced: self-signed bootloaders for NBD/NFS](#advanced-self-signed-bootloaders-for-nbdnfs)
- [Troubleshooting](#troubleshooting)

## How it works

Bootimus ships a built-in bootloader set called `secureboot-official`. It contains only officially signed release binaries, so every link in the boot chain is verified against keys stock firmware already trusts — no certificate enrolment, no firmware changes on clients:

```
Firmware (Secure Boot on, stock Microsoft keys)
  → ipxe-shimx64.efi     Microsoft-signed shim from the iPXE release
    → ipxe.efi           signed by the iPXE project's Secure Boot CA,
                         which the shim carries as its vendor certificate
      → autoexec.ipxe    fetched over TFTP; Bootimus generates it per client
        → boot menu      as normal
```

When you boot an extracted image, the generated menu entry loads the kernel and initrd, then chain-loads the **distro's own Microsoft-signed shim** out of the extracted ISO with iPXE's `shim` command. That shim verifies the distro kernel's signature, completing the chain:

```ipxe
kernel http://server:8080/boot/ubuntu-26.04/vmlinuz ...
initrd http://server:8080/boot/ubuntu-26.04/initrd
iseq ${platform} efi && shim http://server:8080/boot/ubuntu-26.04/iso/EFI/boot/bootx64.efi ||
boot
```

Bootimus detects the shim automatically during ISO extraction. The `iseq … ||` guard makes the line a no-op on BIOS clients and on iPXE builds without the `shim` command, and iPXE only invokes a loaded shim when Secure Boot is actually enforced — machines with Secure Boot disabled boot exactly as before.

## Enabling Secure Boot support

1. Open **Bootloaders** in the admin UI and select **`secureboot-official`** as the active set (or set it per client on the client's edit form).
2. If you use the built-in proxyDHCP, you are done — it advertises `ipxe-shimx64.efi` to UEFI clients automatically.
3. If you run an external DHCP server, change its UEFI bootfile name to `ipxe-shimx64.efi` (and `ipxe-shimaa64.efi` for ARM64 clients). See the [DHCP guide](dhcp.md).

The official iPXE binaries carry no embedded Bootimus script; they rely on iPXE ≥ 2.0 fetching `autoexec.ipxe` over TFTP, which Bootimus serves. Keep UDP port 69 reachable from clients.

## Booting images

For Secure Boot clients, use **extracted (kernel/initrd) boot** for your images — upload the ISO and press **Extract** in the image list. During extraction Bootimus records the ISO's `EFI/BOOT/BOOTX64.EFI` (or `BOOTAA64.EFI`) shim and wires it into the menu automatically.

Images extracted before Secure Boot support was added need a re-extract to pick up their shim: delete the image's boot folder and extract again.

Windows installs boot through the official Microsoft-signed `wimboot`, which the `secureboot-official` set includes; everything wimboot then loads from `boot.wim` is Microsoft-signed.

## What works and what doesn't

| Scenario | Under Secure Boot |
|----------|------------------|
| Extracted Linux ISOs from distros that support Secure Boot (Ubuntu, Debian, Fedora, openSUSE, Rocky, Alma, …) | ✅ Works |
| Windows installer (wimboot) | ✅ Works |
| Distros that do not support Secure Boot on their own media (Arch, Alpine, Void, …) | ❌ Their kernels are unsigned — same as booting their USB stick. Disable Secure Boot for these. |
| `sanboot` (whole-ISO) boot | ❌ Use extracted boot instead |
| NBD / NFS boot methods | ❌ They boot Bootimus's own unsigned kernel — see [below](#advanced-self-signed-bootloaders-for-nbdnfs) |
| Tool images (ShredOS, memtest, …) | ❌ Unsigned kernels |

## Advanced: self-signed bootloaders for NBD/NFS

The NBD and NFS boot methods load Bootimus's own boot-environment kernel, which no public CA will sign. If you need those under Secure Boot, build a self-signed set and enrol your certificate on each client once:

```bash
scripts/generate-signing-key.sh
BOOTIMUS_SIGNING_KEY=… BOOTIMUS_SIGNING_CERT=… scripts/build-secureboot-set.sh
```

Copy the output to `<data>/bootloaders/secureboot/`, select it in the UI, then enrol the certificate on each client with `mokutil --import bootimus-signing.crt` (or via the firmware's key management). For most deployments, disabling Secure Boot on the affected machines is the simpler answer.

## Troubleshooting

- **"Security Violation" splash from the firmware before iPXE loads** — the client isn't fetching `ipxe-shimx64.efi`. Check your DHCP bootfile name and that the `secureboot-official` set is active.
- **iPXE loads but no menu appears** — the official binaries need `autoexec.ipxe` over TFTP. Check UDP 69 between client and server.
- **One "Security Violation" line in iPXE's log while booting an image** — expected and harmless; it is iPXE probing the shim image, not a failure.
- **Menu keys unresponsive on some hardware** — iPXE v2.0.0 has a known keyboard regression in interactive menus on certain machines. Set a default image or use next-boot selection as a workaround, or switch that client to the `default` set if it doesn't need Secure Boot.
- **Kernel refuses to load under Secure Boot** — the image was probably extracted before shim support existed (re-extract it), or the distro doesn't support Secure Boot at all.
