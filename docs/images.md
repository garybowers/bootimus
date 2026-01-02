#  Image Management Guide

Complete guide for managing ISO images, extracting boot files, and handling special cases like Debian/Ubuntu netboot.

##  Table of Contents

- [Adding Images](#adding-images)
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
    "description": "Ubuntu 24.04 LTS Server"
  }'
```

**Monitor progress**:
```bash
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Scan Existing ISOs

If you manually copy ISOs to the data directory:

1. Copy ISO files to `/data/isos/` directory
2. Click **"Scan for ISOs"** button in admin panel
3. Bootimus detects and registers new ISOs

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Kernel Extraction

Most modern ISOs support direct HTTP booting via iPXE's `sanboot` command, which downloads and boots the entire ISO. However, extracting the kernel and initrd provides significant benefits:

###  Benefits of Kernel Extraction

- **Faster boot times**: Only download kernel/initrd (~100MB) instead of entire ISO (1-10GB)
- **Reduced bandwidth**: Critical for networks with multiple clients
- **Better compatibility**: Some ISOs don't support `sanboot` properly
- **Network installation**: Use netboot files for Debian/Ubuntu installers

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
| **Ubuntu Desktop** |  Yes |  No | `/casper/vmlinuz` + fetch optimisation |
| **Ubuntu Server** |  Yes |  Yes | `/install/vmlinuz` + netboot |

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
