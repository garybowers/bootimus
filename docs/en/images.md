#  Image Management Guide

Complete guide for managing ISO images, extracting boot files, and handling special cases like Debian/Ubuntu netboot.

##  Table of Contents

- [Adding Images](#adding-images)
- [Checksums and Update Notifications](#checksums-and-update-notifications)
- [Unified Kernel Images (UKI)](#unified-kernel-images-uki)
- [OpenBSD and NetBSD](#openbsd-and-netbsd)
- [Windows driver packs](#windows-driver-packs)
- [WinPE and Rescue Media](#winpe-and-rescue-media)
- [Kernel Extraction](#kernel-extraction)
- [Netboot Support](#netboot-support)
- [Ubuntu Desktop Optimisation](#ubuntu-desktop-optimisation)
- [Supported Distributions](#supported-distributions)
- [Troubleshooting](#troubleshooting)

## Adding Images

### Upload via Web Interface

1. Navigate to admin panel: `http://your-server:8081`
2. Click **"Upload ISO"** button
3. Drag and drop ISO file or click to browse
4. Optionally add description
5. Check **"Public"** to make available to all clients
6. Click **"Upload"**

**Upload limits**: 10GB per file

### Upload via API

```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Download from URL

Download ISOs directly to the server without local upload:

**Via Web Interface**:
1. Click **"Download from URL"** button
2. Enter ISO download URL
3. Add description
4. Click **"Download"**

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "filename": "ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'
```

**NB** The `filename` parameter is optional. When not given, its derived from the `url` parameter.

**Monitor progress**:
```bash
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Checksums and Update Notifications

**Verification on download.** When an ISO is fetched through **Get Images** (the built-in catalogue), Bootimus looks for a published checksum file next to it (`SHA256SUMS`, `sha256sum.txt` and similar) and hashes the download as it streams. A mismatch fails the download and removes the file, so a truncated or tampered image never reaches the boot menu. Downloads from a plain URL can be verified too by passing a `sha256` or a `checksum_url` to `/api/images/download`:

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{"url": "https://releases.ubuntu.com/24.04.4/ubuntu-24.04.4-live-server-amd64.iso",
       "checksum_url": "https://releases.ubuntu.com/24.04.4/SHA256SUMS"}'
```

Verified images show a **✓ Verified** badge in the image list and the recorded SHA-256 in the image properties panel. Images downloaded without a published checksum, or uploaded by hand, are marked **Unverified**; paste a SHA-256 or the URL of a checksum file into the properties panel and press **Verify** to hash the file in the background (`POST /api/images/verify?filename=…`, poll the same URL with `GET`).

**Newer releases.** Once a day (and on **Settings → Housekeeping → Check now**, or `POST /api/catalogue/check`) Bootimus compares installed images with the catalogue. An image downloaded from the catalogue is matched by its distribution and release line, so `ubuntu-24.04.3-desktop-amd64.iso` is flagged when `24.04.4` appears; images added before this feature are matched by filename shape. Flagged images carry an **Update available** badge and a **Download newer version** button in the properties panel, tools show the same badge with **Download newer**, and the dashboard counts outstanding updates. The check uses the online catalogue when remote profile updates are allowed and falls back to the catalogue built into the binary when `--disable-remote-profiles` is set. Untick **Check the catalogue daily** under Housekeeping to turn it off.

### Organise with Folders

ISOs placed in subdirectories are automatically grouped in the boot menu:

```
/data/isos/
├── ubuntu-24.04.iso              # ungrouped
├── linux/                        # "linux" group
│   ├── debian-12.iso
│   └── servers/                  # "servers" subgroup under "linux"
│       └── truenas-scale.iso
└── windows/                      # "windows" group
    └── win11.iso
```

Groups are auto-created on startup and when scanning. They can also be managed manually via the Groups tab in the admin UI.

### Scan Existing ISOs

If you manually copy ISOs to the data directory (including into subdirectories):

1. Copy ISO files to `/data/isos/` directory (or subdirectories)
2. Click **"Scan for ISOs"** button in admin panel
3. Bootimus detects and registers new ISOs and creates groups from folders

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Unified Kernel Images (UKI)

A unified kernel image (UKI) is a single EFI executable that bundles a Linux kernel, its initrd and its kernel command line. Bootimus boots UKIs directly, with no extraction.

**Adding a UKI**: upload the `.efi` file, download it from a URL, or copy it into `/data/isos/` and scan, exactly as for an ISO. Bootimus only registers `.efi` files that are UKIs (EFI executables with a `.linux` section); other EFI programs are rejected, and files inside an ISO's extracted folder are ignored.

**Boot parameters**: the command line embedded in the UKI's `.cmdline` section is copied into the image's boot parameters when it is first added, so you can see and change it in the image properties. Bootimus boots the image with:

```
kernel http://<server>:8080/isos/<image>.efi <boot parameters>
```

With Secure Boot disabled, these parameters replace the embedded command line, which is why the embedded one is pre-filled. With Secure Boot enabled, the systemd-stub inside most UKIs ignores them and always uses the embedded command line.

**UEFI only**: UKIs are EFI programs, so legacy BIOS clients cannot boot them. Choosing one on a BIOS client shows a message saying it needs UEFI.

## OpenBSD and NetBSD

OpenBSD and NetBSD do not boot from a Linux-style kernel and initrd. Each ships its own second-stage network loader, and that loader only speaks TFTP: it fetches its configuration file and the install kernel from the TFTP server's root directory. Bootimus supports both by extracting those files from the install ISO and serving them over its TFTP server, so the TFTP port must be reachable from clients (it already is for PXE itself).

**Adding an image**: upload `install77.iso` (or `cd77.iso`) for OpenBSD, or `NetBSD-10.1-amd64.iso` for NetBSD, then run **Extract Kernel/Initrd** as for any other ISO. Extraction copies the loaders and the install kernel into a `bsd/` folder next to the ISO and extracts the full ISO tree so the installer can fetch its sets from `http://<server>:8080/boot/<image>/iso/`.

| Distribution | Loaders taken from the ISO | Kernel |
|--------------|----------------------------|--------|
| OpenBSD | `pxeboot` (BIOS), `BOOTX64.EFI`, `BOOTIA32.EFI`, `BOOTAA64.EFI` (UEFI) from `/<version>/<arch>/` | `bsd.rd` |
| NetBSD | `pxeboot_ia32.bin` (BIOS) from `/<arch>/installation/misc/`, `bootx64.efi` (UEFI) when the ISO carries one | `netbsd-INSTALL.gz` |

**How the boot works**: the menu entry chains to a small iPXE script on the HTTP port, which records that this client (by IP address) chose that image and then chains the matching loader over TFTP from `bsd/<image>/`. For the next 15 minutes, requests from that IP for the files the loaders look for at the TFTP root (`etc/boot.conf`, `etc/random.seed`, `bsd.rd`, `bsd` for OpenBSD; `boot.cfg`, `EFI/NetBSD/boot.cfg`, `netbsd` and `netbsd-INSTALL.gz` for NetBSD) are answered from that image. The configuration file is generated on the fly and points the loader at the kernel under `bsd/<image>/`, so explicit paths also work. If the ISO lacks the loader for the client's firmware, the client sees a message saying so and returns to the menu.

**Serial console and other loader settings**: put extra loader configuration lines in the image's **Boot Parameters**, separated by semicolons. For OpenBSD they are appended to the generated `boot.conf`, for example `stty com0 115200;set tty com0`. For NetBSD they are appended to the generated `boot.cfg`, for example `consdev=com0`.

**Known limitations**:

- This support is new and has not yet been verified on real hardware. Please report what works and what does not.
- NetBSD's `pxeboot_ia32.bin` performs its own DHCP exchange after it loads and reads the DHCP `filename` field to decide between NFS and TFTP. Without a `tftp:` prefix there it defaults to NFS, which Bootimus does not provide for this purpose. If the loader reports that it cannot open `boot.cfg` or the kernel, type `boot tftp:bsd/<image>/netbsd-INSTALL.gz` at its prompt, or give NetBSD clients `filename "tftp:boot.cfg";` on your DHCP server. OpenBSD's loaders always use TFTP and need no such step.
- The per-client association is keyed by IP address, so two clients behind the same NAT address booting different BSD images within 15 minutes would receive the same configuration.
- Autoinstall for the BSD installers is not wired up yet. The installers can still fetch sets from the extracted ISO tree over HTTP.

## Windows driver packs

Open a Windows image and select **Drivers**. Upload ZIP packs containing the INF files and all their supporting files, keeping the vendor's directory structure. Each ZIP may be up to 500 MiB, with at most 10,000 entries and 1 GiB of expanded data. Bootimus checks the archive, but Windows still decides whether the drivers match the hardware and meet its signing requirements.

The same ZIP cannot be uploaded twice to an image, even under a different filename. Other images can use the same pack. Remove any existing duplicates from the list, then apply the changes.

Select **Apply changes** after adding or removing packs. Uploading or removing a pack alone does not change the boot image. The tab shows pending, applying, applied or failed status, including failures after a server restart. Status is also available at `GET /api/drivers/status?imageId=...`; `POST /api/drivers/rebuild?imageId=...` returns HTTP 202 when a rebuild is accepted.

Applying drivers requires an extracted `sources/boot.wim` and `wimtools` on the server. Bootimus rebuilds from its original WIM backup, keeps packs in separate subdirectories under `X:\drivers`, and replaces the active WIM only after a successful rebuild. Removing the last pack restores the original contents. Existing SMB startup scripts are reapplied. If an older image has no original backup, re-extract it before changing its applied drivers.

For automatic loading, configure Windows SMB and use **Patch SMB**. The startup script loads the drivers before connecting to the installation share and passes the same directory to Windows Setup. Custom WinPE media needs its own driver-loading script; layouts without `sources/boot.wim` are not supported by this driver-management workflow.

## WinPE and Rescue Media

Bootimus boots Windows installers through [wimboot](https://ipxe.org/wimboot), and the same path now covers WinPE-based rescue media such as Sergei Strelec's WinPE, Active@ Boot Disk and Hiren's BootCD PE.

**Supported layouts**: any ISO that carries a `BCD` (under `boot/` or `efi/microsoft/boot/`), `boot/boot.sdi` and at least one `.wim` file. Installer media also has `sources/install.wim` or `install.esd`; media without one is classified as the **WinPE / Rescue Media** profile (`winpe`) instead of **Windows**, so no auto-install or SMB handling is applied to it. WIM files anywhere in the first four directory levels are found, so Strelec's `SSTR/*.wim` and similar layouts work without renaming anything.

**Choosing the WIM**: after extraction, the image properties panel lists every WIM found with its size. Pick the one wimboot should load and, when a WIM holds several images, the image index. The index is only added to the wimboot line when it is greater than one, and an explicit `index=` in the boot parameters takes precedence. The same can be set through the API:

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"wim_path":"SSTR/strelec10x64Eng.wim","wim_index":1}' \
  "http://localhost:8081/api/images/winpe?filename=WinPE_Strelec.iso"
```

**How it boots**: only the selected WIM is sent to the client; wimboot synthesises the BCD and `boot.sdi` itself, which avoids the black screen some media produce when the DVD BCD is used. Boot parameters are limited to wimboot's own options (`rawbcd`, `rawwim`, `gui`, `linear`, `quiet`, `pause`, `index`, `initrdfile`), exactly as for Windows installers.

**Limitations**:

- Programs that the rescue environment expects to find on its own boot media (for example Strelec's `SSTR` folder of portable tools) are not inside the WIM, so they are missing after a PXE boot unless the environment can fetch them over the network itself. The core WinPE desktop and the tools built into the WIM work.
- Media that needs its own BCD entries (custom `bootmgr` menus, multi-WIM chooser menus) boots straight into the chosen WIM instead of showing that menu.
- Bare `.wim` files cannot be uploaded on their own yet; put them on an ISO with a `BCD` and `boot.sdi`, or use a complete rescue ISO.

## Kernel Extraction

Most modern ISOs support direct HTTP booting via iPXE's `sanboot` command, which downloads and boots the entire ISO. However, extracting the kernel and initrd provides significant benefits:

###  Benefits of Kernel Extraction

- **Faster boot times**: Only download kernel/initrd (~100MB) instead of entire ISO (1-10GB)
- **Reduced bandwidth**: Critical for networks with multiple clients
- **Better compatibility**: Some ISOs don't support `sanboot` properly
- **Network installation**: Use netboot files for Debian/Ubuntu installers
- **UEFI Secure Boot**: Extraction detects the ISO's signed shim, letting Secure Boot clients verify the kernel — see the [Secure Boot guide](secure-boot.md)

### How to Extract

**Via Web Interface**:
1. Navigate to **Images** tab
2. Find your ISO image
3. Click **"Extract"** button
4. Wait for extraction to complete

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04-live-server-amd64.iso"}'
```

### Manual Extraction

If the built-in extractor doesn't support your ISO, you can extract the boot files manually and bootimus will detect them automatically.

1. Create a directory with the same name as the ISO (minus the `.iso` extension):
   ```bash
   mkdir -p data/isos/my-custom-distro/
   ```

2. Place the kernel and initrd in that directory with these exact names:
   ```
   data/isos/
   ├── my-custom-distro.iso
   └── my-custom-distro/
       ├── vmlinuz          # kernel
       └── initrd           # initrd/initramfs
   ```

3. Click **"Scan for ISOs"** in the admin panel (or restart bootimus). The image will be automatically detected as extracted and set to kernel boot method.

This also works for ISOs in subdirectories:
```
data/isos/linux/my-custom-distro.iso
data/isos/linux/my-custom-distro/vmlinuz
data/isos/linux/my-custom-distro/initrd
```

### What Gets Extracted

Bootimus automatically detects the distribution and extracts:

- **Kernel**: `vmlinuz` (or `linux`, `bzImage`)
- **Initrd**: `initrd`, `initrd.gz`, `initrd.lz`
- **Squashfs** (Ubuntu/Debian live): `filesystem.squashfs`
- **Distribution metadata**: OS type, boot parameters

**Extracted files location**:
```
/data/isos/
├── ubuntu-24.04.iso                    # Original ISO
└── ubuntu-24.04/                       # Extracted directory
    ├── vmlinuz                         # Kernel
    ├── initrd                          # Initrd
    └── casper/
        └── filesystem.squashfs         # Squashfs filesystem
```

### Automatic Boot Method Selection

After extraction, Bootimus automatically selects the optimal boot method:

| Distribution | Boot Method | Downloads |
|--------------|-------------|-----------|
| Ubuntu Desktop (extracted) | `fetch=` | ~2.8GB (squashfs only) |
| Ubuntu Desktop (not extracted) | `url=` | ~18GB (ISO × 3) |
| Ubuntu Server (netboot) | Netboot | ~50MB (netboot files) |
| Debian Installer (netboot) | Netboot | ~30MB (netboot files) |
| Arch Linux | HTTP boot | ~100MB (kernel/initrd) |
| Fedora/RHEL | HTTP boot | ~150MB (kernel/initrd + stage2) |

## Netboot Support

Some installer ISOs (Debian, Ubuntu Server) don't contain a full OS - they're designed to download packages during installation. For these, Bootimus supports downloading official netboot files.

###  Detecting Netboot Requirement

When you extract a Debian or Ubuntu Server installer ISO, Bootimus detects it requires netboot:

**Indicators**:
- ISO contains `/install/` directory (not `/casper/`)
- Installer type (not live/desktop)
- Small ISO size (< 1GB)

**Admin panel shows**:
-  "Netboot Required" badge
- 📥 "Download Netboot" button

### Downloading Netboot Files

**Via Web Interface**:
1. Navigate to **Images** tab
2. Find the installer ISO with "Netboot Required" badge
3. Click **"Download Netboot"** button
4. Wait for download and extraction

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

### What Are Netboot Files?

Netboot files are official, minimal boot files provided by distributions:

**Debian netboot**:
- Source: `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`
- Size: ~30MB
- Contains: `vmlinuz`, `initrd.gz`, installer files

**Ubuntu netboot**:
- Source: `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`
- Size: ~50MB
- Contains: `vmlinuz`, `initrd.gz`, installer files

### How Netboot Works

1. **Client boots**: Downloads netboot kernel/initrd (~50MB)
2. **Installer starts**: Netboot initrd starts network installer
3. **Package download**: Installer downloads packages from Ubuntu/Debian mirrors
4. **Installation**: OS installed directly from internet repositories

**Benefits**:
-  Always get latest packages (not stale ISO packages)
-  Minimal bandwidth to PXE server (no ISO download)
-  Smaller storage requirements
-  Official, signed boot files

### Debian Installer Netboot

**Supported ISOs**:
- `debian-*-netinst.iso` - Network installer
- Small Debian installer ISOs with `/install/` directory

**Detection**:
```
ISO structure:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**Netboot URL**: `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`

**Boot parameters**: `priority=critical ip=dhcp`

### Ubuntu Server Netboot

**Supported ISOs**:
- `ubuntu-*-live-server-*.iso` - Live server installer with `/install/` directory
- Older Ubuntu server installers

**Detection**:
```
ISO structure:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**Netboot URL**: `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`

**Boot parameters**: `ip=dhcp`

###  Important: Ubuntu Desktop vs Server

There are **two types** of Ubuntu ISOs with different boot methods:

| Type | ISO Name Pattern | Directory | Boot Method | Netboot? |
|------|------------------|-----------|-------------|----------|
| **Desktop/Live** | `ubuntu-*-desktop-*.iso` | `/casper/` | `fetch=` or `url=` |  No |
| **Server Installer** | `ubuntu-*-live-server-*.iso` (with `/install/`) | `/install/` | Netboot |  Yes |

**Ubuntu Desktop** (`/casper/`):
- Contains full live OS
- Uses casper boot with `fetch=` or `url=`
- Extract kernel to use `fetch=` (downloads squashfs only)
- No netboot support

**Ubuntu Server Installer** (`/install/`):
- Minimal network installer
- Requires netboot files
- Downloads packages during installation
- Much more efficient

## Ubuntu Desktop Optimisation

Ubuntu Desktop ISOs use the casper live boot system. Without optimisation, they download the entire ISO **three times** (~18GB for a 6GB ISO).

###  Problem: Triple ISO Download

**Default behaviour** (no extraction):
```
Boot parameter: url=http://server/ubuntu.iso

Result:
- Download 1: Kernel verifies ISO (6GB)
- Download 2: Initrd verifies ISO (6GB)
- Download 3: Casper mounts ISO (6GB)
Total: ~18GB downloaded
```

###  Solution 1: Extract and Use `fetch=` Parameter

**After extraction**:
```
Boot parameter: fetch=http://server/ubuntu/casper/filesystem.squashfs

Result:
- Download: Only squashfs (~2.8GB)
Total: ~2.8GB downloaded
```

**How to enable**:
1. Extract kernel/initrd from ISO
2. Bootimus automatically uses `fetch=` parameter
3. Only squashfs downloaded (not entire ISO)

**Savings**: 85% reduction (18GB → 2.8GB)

###  Solution 2: Use Ubuntu Server Netboot Instead

For server deployments, use Ubuntu Server installer with netboot:

**Netboot approach**:
```
1. Upload ubuntu-server.iso
2. Extract kernel/initrd
3. Download netboot files
4. Boot with netboot (~50MB download)
5. Install from Ubuntu repositories
```

**Savings**: 99% reduction (18GB → 50MB)

### Boot Parameter Reference

**Ubuntu Desktop (casper)**:
```bash
# Default (no extraction) - downloads ISO 3 times
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp url=http://server/ubuntu.iso

# Optimised (with extraction) - downloads squashfs once
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp fetch=http://server/ubuntu/casper/filesystem.squashfs
```

**Ubuntu Server (netboot)**:
```bash
# Netboot - minimal download
ip=dhcp
```

## Supported Distributions

### Fully Tested

| Distribution | Kernel Extraction | Netboot | Notes |
|--------------|-------------------|---------|-------|
| **Arch Linux** |  Yes |  N/A | `/arch/boot/x86_64/vmlinuz-linux` |
| **Fedora Workstation** |  Yes |  N/A | `/isolinux/vmlinuz` |
| **Rocky Linux** |  Yes |  N/A | `/isolinux/vmlinuz` |
| **Debian (installer)** |  Yes |  Yes | `/install/vmlinuz` + netboot |
| **Debian Live** |  Yes |  No | `/live/vmlinuz` |
| **Ubuntu Desktop** |  Yes |  No | `/casper/vmlinuz` + fetch optimisation |
| **Ubuntu Server** |  Yes |  Yes | `/install/vmlinuz` + netboot |
| **Pop!_OS** |  Yes |  No | `/casper/vmlinuz` |
| **TrueNAS SCALE** |  Yes |  No | 25.10.2.1: UEFI QEMU installation and disk boot; full ISO fetch |
| **Proxmox VE** |  Yes |  No | `/boot/linux26` + `/boot/initrd.img` |
| **openSUSE** |  Yes |  N/A | `/boot/x86_64/loader/linux` |
| **NixOS** |  N/A |  N/A | Sanboot |

### Detection Patterns

Bootimus detects distributions by scanning for specific file patterns:

**Arch Linux**:
```
/arch/boot/x86_64/vmlinuz-linux
/arch/boot/x86_64/initramfs-linux.img
```

**Fedora/RHEL/Rocky**:
```
/isolinux/vmlinuz
/isolinux/initrd.img
```

**Ubuntu Desktop (casper)**:
```
/casper/vmlinuz or /casper/vmlinuz.efi
/casper/initrd or /casper/initrd.gz or /casper/initrd.lz
/casper/filesystem.squashfs
```

**Ubuntu Server Installer**:
```
/install/vmlinuz or /install.amd/vmlinuz
/install/initrd.gz or /install.amd/initrd.gz
```

**Debian Installer**:
```
/install/vmlinuz or /install.amd/vmlinuz
/install/initrd.gz or /install.amd/initrd.gz
```

**TrueNAS SCALE**:
```
/vmlinuz
/initrd.img
/live/filesystem.squashfs
/TrueNAS-SCALE.update
```

Bootimus recognises SCALE by its `TrueNAS-SCALE.update` payload and selects a dedicated profile. Extract the kernel/initrd as usual; the profile downloads the complete ISO and mounts it at `/cdrom` for the installer. It uses the ISO unchanged and requires systemd 254 or later in the installer.

Verified with SCALE 25.10.2.1 in UEFI QEMU with 8 GiB RAM: installation to a fresh virtual disk and subsequent disk boot. The complete ISO stays in RAM during installation. Other releases, physical hardware and Secure Boot remain untested; this profile does not cover FreeBSD-based TrueNAS CORE.

For an image extracted before this fix, re-extract it to replace the old Debian settings. Review any custom boot parameters afterwards.

**Proxmox VE**:
```
/boot/linux26
/boot/initrd.img
```

## Troubleshooting

### Extraction Failed

**Symptoms**: "Extraction failed" error in admin panel

**Common causes**:
1. **Corrupted ISO**: Re-download the ISO
2. **Unsupported ISO**: Check if distribution is supported
3. **Disk space**: Ensure sufficient space for extraction
4. **Permissions**: Check file permissions on data directory

**Debugging**:
```bash
# Check extraction logs
docker logs bootimus | grep -i extract

# Verify ISO integrity
sha256sum ubuntu.iso

# Check disk space
df -h /data/isos/

# Test manual mount
sudo mount -o loop ubuntu.iso /mnt
ls /mnt/casper/
sudo umount /mnt
```

### Netboot Download Failed

**Symptoms**: "Netboot download failed" error

**Common causes**:
1. **Network connectivity**: Cannot reach Debian/Ubuntu mirrors
2. **URL changed**: Mirror URL may have been updated
3. **Tarball extraction failed**: Corrupted download

**Solutions**:
```bash
# Test mirror connectivity
curl -I http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz

# Check server logs
docker logs bootimus | grep -i netboot

# Manually verify netboot URL
wget http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz
tar -tzf netboot.tar.gz | grep vmlinuz
```

### Boot Menu Shows Wrong Image Type

**Symptoms**: Image shows "[kernel]" badge but doesn't boot with kernel method

**Cause**: Database and filesystem out of sync

**Solution**:
```bash
# Re-extract kernel/initrd
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'

# Or re-scan ISOs
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### Client Downloads ISO Multiple Times

**Symptoms**: Ubuntu Desktop ISO downloads 3 times

**Cause**: Using `url=` parameter without extraction

**Solution**:
1. Extract kernel/initrd from ISO
2. Bootimus will automatically use `fetch=` parameter
3. Only squashfs downloaded (not entire ISO)

**Verify**:
```bash
# Check if extracted
ls -la /data/isos/ubuntu-24.04/casper/filesystem.squashfs

# Check server logs during boot
docker logs -f bootimus
# Look for: "fetch=..." instead of "url=..."
```

### Netboot Required but No Download Button

**Symptoms**: Image shows "Netboot Required" but no download button

**Cause**: Netboot URL not configured or detection failed

**Solution**:
```bash
# Check image details
curl -u admin:password http://localhost:8081/api/images | jq '.data[] | select(.filename=="debian-13.2.0-amd64-netinst.iso")'

# Verify netboot_required and netboot_url fields
# If netboot_url is empty, the ISO detection may have failed

# Try re-extracting
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

## Next Steps

-  See [Admin Console Guide](admin.md) for managing images
-  Read [Deployment Guide](deployment.md) for storage configuration
-  Configure [DHCP Server](dhcp.md) for PXE booting
-  Set up [Client Management](clients.md) for access control
