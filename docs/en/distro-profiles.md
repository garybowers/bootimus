# Distro Profiles Guide

Bootimus uses distro profiles to detect ISO types and generate the correct boot parameters. Profiles are data-driven — you can add support for new distributions without a code change.

## Table of Contents

- [Overview](#overview)
- [How It Works](#how-it-works)
- [Viewing Profiles](#viewing-profiles)
- [Updating Profiles](#updating-profiles)
- [Creating Custom Profiles](#creating-custom-profiles)
- [Profile Fields](#profile-fields)
- [Placeholders](#placeholders)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

Distro profiles define:
- **How to detect** which distro an ISO is (filename pattern matching)
- **Where to find** the kernel, initrd, and squashfs inside the ISO
- **What boot parameters** to use when PXE booting
- **What auto-install type** is supported (preseed, kickstart, autoinstall, etc.)

### Profile Types

| Type | Description |
|------|-------------|
| **Built-in** | Shipped with Bootimus, updated from the central repository |
| **Custom** | Created by the user, never overwritten by updates |

Custom profiles always take priority over built-in profiles when matching ISO filenames.

## How It Works

1. When an ISO is uploaded or extracted, Bootimus matches the filename against profile patterns
2. The matching profile's kernel/initrd paths are used to locate boot files inside the ISO
3. The profile's boot params become the default (editable in image Properties)
4. At boot time, placeholders in the params are resolved to actual URLs

### Profile Lifecycle

```
Build time:    distro-profiles.json embedded in binary
                        ↓
First startup:  Profiles seeded into database
                        ↓
"Check for Updates":  Latest profiles fetched from GitHub
                        ↓
User creates:   Custom profiles stored in database (never overwritten)
```

## Viewing Profiles

Navigate to **Boot > Distro Profiles** in the admin panel to see all loaded profiles with their filename patterns, boot parameters, type (Built-in/Custom), and version.

## Updating Profiles

### Automatic (Recommended)

Click **"Check for Updates"** in the Distro Profiles tab. This fetches the latest profiles from:

```
https://raw.githubusercontent.com/garybowers/bootimus/main/distro-profiles.json
```

- New profiles are added automatically
- Existing built-in profiles are updated to the latest version
- Custom profiles are never modified

### Via API

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/profiles/update
```

Response:
```json
{
  "success": true,
  "message": "Updated to version 0.1.21 (2 added, 5 updated)"
}
```

## Creating Custom Profiles

### Via Web Interface

1. Go to **Boot > Distro Profiles**
2. Click **"+ Add Custom Profile"**
3. Fill in the profile fields
4. Click **"Create Profile"**

### Via API

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/profiles/save \
  -H "Content-Type: application/json" \
  -d '{
    "profile_id": "my-distro",
    "display_name": "My Custom Distro",
    "family": "debian",
    "filename_patterns": ["mydistro", "my-distro"],
    "kernel_paths": ["/live/vmlinuz", "/boot/vmlinuz"],
    "initrd_paths": ["/live/initrd.img", "/boot/initrd"],
    "squashfs_paths": ["/live/filesystem.squashfs"],
    "default_boot_params": "boot=live initrd=initrd ip=dhcp",
    "boot_params_with_squashfs": "boot=live initrd=initrd fetch={{SQUASHFS}}",
    "auto_install_type": "preseed"
  }'
```

### Deleting Custom Profiles

Only custom profiles can be deleted. Built-in profiles are restored on the next update.

```bash
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/profiles/delete?id=my-distro"
```

## Profile Fields

| Field | Required | Description |
|-------|----------|-------------|
| `profile_id` | Yes | Unique identifier (e.g., `ubuntu`, `my-distro`) |
| `display_name` | Yes | Human-readable name shown in the UI |
| `family` | No | Distro family (e.g., `debian`, `arch`, `redhat`) — for grouping |
| `filename_patterns` | Yes | Substrings to match in ISO filenames (case-insensitive) |
| `kernel_paths` | No | Paths to try for the kernel inside the ISO (e.g., `/casper/vmlinuz`) |
| `initrd_paths` | No | Paths to try for the initrd inside the ISO |
| `squashfs_paths` | No | Paths to try for the squashfs root filesystem |
| `default_boot_params` | No | Default kernel boot parameters (with placeholder support) |
| `boot_params_with_squashfs` | No | Alternative boot params used when squashfs is detected |
| `auto_install_type` | No | Auto-install format: `preseed`, `kickstart`, `autoinstall`, `autounattend` |
| `boot_method` | No | Override boot method (e.g., `wimboot` for Windows) |

## Placeholders

Boot parameters support these placeholders, resolved at boot time:

| Placeholder | Resolves to | Example |
|-------------|-------------|---------|
| `{{BASE_URL}}` | Server HTTP URL | `http://192.168.1.10:8080` |
| `{{CACHE_DIR}}` | Extracted files directory | `ubuntu-24.04-server-amd64` |
| `{{FILENAME}}` | ISO filename (URL-encoded) | `ubuntu-24.04-server-amd64.iso` |
| `{{SQUASHFS}}` | Full URL to squashfs file | `http://192.168.1.10:8080/boot/ubuntu.../casper/filesystem.squashfs` |

### Example with Placeholders

```
boot=live initrd=initrd fetch={{SQUASHFS}} ip=dhcp
```

Resolves to:
```
boot=live initrd=initrd fetch=http://192.168.1.10:8080/boot/debian-live-13/live/filesystem.squashfs ip=dhcp
```

## Examples

### Debian-Based Live ISO

```json
{
  "profile_id": "my-debian-live",
  "display_name": "My Debian Live Spin",
  "family": "debian",
  "filename_patterns": ["my-debian"],
  "kernel_paths": ["/live/vmlinuz"],
  "initrd_paths": ["/live/initrd.img"],
  "squashfs_paths": ["/live/filesystem.squashfs"],
  "default_boot_params": "initrd=initrd boot=live priority=critical",
  "boot_params_with_squashfs": "initrd=initrd boot=live priority=critical fetch={{SQUASHFS}}"
}
```

### Arch-Based Distro

```json
{
  "profile_id": "my-arch-spin",
  "display_name": "My Arch Spin",
  "family": "arch",
  "filename_patterns": ["myarch"],
  "kernel_paths": ["/arch/boot/x86_64/vmlinuz-linux", "/boot/vmlinuz-linux"],
  "initrd_paths": ["/arch/boot/x86_64/initramfs-linux.img", "/boot/initramfs-linux.img"],
  "squashfs_paths": ["/arch/x86_64/airootfs.sfs"],
  "default_boot_params": "archisobasedir=arch archiso_http_srv={{BASE_URL}}/boot/{{CACHE_DIR}}/iso/ ip=dhcp"
}
```

### RHEL-Based Installer

```json
{
  "profile_id": "my-rhel-clone",
  "display_name": "My RHEL Clone",
  "family": "redhat",
  "filename_patterns": ["myrhel"],
  "kernel_paths": ["/images/pxeboot/vmlinuz"],
  "initrd_paths": ["/images/pxeboot/initrd.img"],
  "default_boot_params": "root=live:{{BASE_URL}}/isos/{{FILENAME}} rd.live.image inst.repo={{BASE_URL}}/boot/{{CACHE_DIR}}/iso/ rd.neednet=1 ip=dhcp",
  "auto_install_type": "kickstart"
}
```

## Troubleshooting

### ISO Not Detected as Correct Distro

Check if the ISO filename matches any profile pattern:

1. Go to **Distro Profiles** tab
2. Look at the "Filename Patterns" column
3. If no pattern matches your ISO filename, create a custom profile

### Boot Params Wrong After Extraction

1. Open image **Properties**
2. Click **"Re-detect"** next to Boot Parameters
3. Or edit the boot params manually — they support placeholders

### "Check for Updates" Failed

The update fetches from GitHub. Check:
- Server has internet access
- `raw.githubusercontent.com` is not blocked
- Try again later if GitHub is down

### Custom Profile Not Matching

Custom profiles take priority over built-in. Ensure:
- The `filename_patterns` contain substrings that match your ISO filename (case-insensitive)
- The profile ID is unique
- The profile was saved successfully

### Contributing Profiles

To add a profile to the official list for all users:
1. Fork the [Bootimus repository](https://github.com/garybowers/bootimus)
2. Edit `distro-profiles.json` in the repo root
3. Add your profile to the `profiles` array
4. Submit a pull request

This way all Bootimus users get the new profile via "Check for Updates".
