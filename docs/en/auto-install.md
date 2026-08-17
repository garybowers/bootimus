# Auto-Install Guide

Run unattended installs across Windows, Ubuntu, Debian, and Red Hat-family distros. Drop a config file in once, attach it to an image, and every PXE boot finishes installation without a single keystroke.

## Table of Contents

- [Overview](#overview)
- [Supported Formats](#supported-formats)
- [The File Library](#the-file-library)
- [Attaching Files](#attaching-files)
- [Resolution Order](#resolution-order)
- [Placeholders](#placeholders)
- [Examples](#examples)
- [Windows Notes](#windows-notes)
- [REST API](#rest-api)
- [Troubleshooting](#troubleshooting)

## Overview

Auto-install configs are stored in a per-distro library under `data/autoinstall/`. You can:

- **Manage files in the UI** under the **Auto-Install** tab — create, edit, upload, download, and delete configs without touching the filesystem.
- **Set a default** per image (every client booting that image gets the same config).
- **Override per client** or **per client group** when one machine needs a different config (different hostname, disk layout, role, etc.).
- **Templatise** with placeholders like `{{HOSTNAME}}` and `{{IP}}` resolved at serve time using the booting client's identity.

Bootimus serves the script over HTTP on the auto-install endpoint, and — for Windows — stages `AutoUnattend.xml` on the SMB install share so `setup.exe /unattend:` picks it up automatically.

## Supported Formats

| Distro family | Format | File extension | Detected as |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| Anything else | raw | any | `generic` |

The extension drives both the in-UI label and the `Content-Type` header when the file is served.

## The File Library

All auto-install files live under `data/autoinstall/<distro>/<filename>`:

```
data/autoinstall/
├── windows/
│   ├── kiosk.xml
│   └── server-2022.xml
├── ubuntu/
│   ├── default.yaml
│   └── lab-bench.yaml
├── debian/
│   └── server.cfg
└── rocky/
    └── workstation.ks
```

The `<distro>` segment must match a known distro profile ID (see [Distro Profiles](distro-profiles.md)). The directory is created automatically on first start.

### Add files via the UI

**Auto-Install** tab → **New File** opens the editor with a distro picker, filename field, and a syntax-friendly textarea. **Upload File** takes any local file and drops it into the chosen distro folder.

### Add files manually

Just drop them in:

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

They appear in the UI immediately — no restart, no scan.

## Attaching Files

Auto-install files have no effect until you wire them up. There are three places to attach one:

### Image (default)

**Images** tab → open an image's **Properties** → **Auto-Install** section → pick a file. Every client that boots this image gets this config unless something more specific overrides it.

### Client (per-machine override)

**Clients** tab → open a client → **Auto-Install File** dropdown. Use this when one specific machine needs a different config (e.g., a build server vs the rest of the desk fleet).

### Client Group (per-fleet override)

**Groups** tab → open a group → **Auto-Install File**. Applies to every client in the group. Useful for "all the workstations in lab 3" scenarios.

## Resolution Order

When a client requests its auto-install file, Bootimus walks this hierarchy:

```
1. Per-client override        (Client.AutoInstallFile)
2. Per-group override         (ClientGroup.AutoInstallFile, if client is in a group)
3. Image default              (Image.AutoInstallFile)
4. Inline legacy script       (Image.AutoInstallScript — pre-0.1.58 setups)
5. → 404 (no auto-install configured)
```

The first non-empty match wins. The endpoint logs the source it served from:

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## Placeholders

These tokens are substituted per-client at serve time:

| Token | Replaced with |
|-------|---------------|
| `{{MAC}}` | Client MAC address (lowercase, colon-separated) |
| `{{CLIENT_NAME}}` | Friendly name from the Clients table |
| `{{HOSTNAME}}` | Same as `{{CLIENT_NAME}}` (alias for clarity in configs) |
| `{{IP}}` | Client IP that issued the request |
| `{{SERVER_ADDR}}` | Bootimus server address |
| `{{IMAGE_NAME}}` | Display name of the booting image |
| `{{IMAGE_FILENAME}}` | ISO filename of the booting image |

Placeholders are plain string substitution — no escaping. Quote them appropriately for the target format (XML, YAML, etc.).

## Examples

### Ubuntu Server (cloud-init)

`data/autoinstall/ubuntu/default.yaml`:

```yaml
#cloud-config
autoinstall:
  version: 1
  identity:
    hostname: {{HOSTNAME}}
    username: ubuntu
    password: "$6$rounds=4096$..."  # mkpasswd -m sha-512
  ssh:
    install-server: true
    allow-pw: false
    authorized-keys:
      - ssh-ed25519 AAAA...
  storage:
    layout:
      name: lvm
  late-commands:
    - curtin in-target -- systemctl enable --now serial-getty@ttyS0.service
```

Boot params (the relevant image already has these by default for Ubuntu):

```
autoinstall ds=nocloud-net;s=http://{{SERVER_ADDR}}:8080/autoinstall/{{IMAGE_FILENAME}}/
```

### Debian (preseed)

`data/autoinstall/debian/server.cfg`:

```
d-i debian-installer/locale string en_GB.UTF-8
d-i keyboard-configuration/xkb-keymap select gb
d-i netcfg/get_hostname string {{HOSTNAME}}
d-i netcfg/get_domain string lan
d-i partman-auto/method string lvm
d-i partman-auto/choose_recipe select atomic
d-i passwd/root-login boolean false
d-i passwd/user-fullname string Admin
d-i passwd/username string admin
d-i passwd/user-password-crypted password $6$rounds=4096$...
d-i pkgsel/include string openssh-server
d-i grub-installer/bootdev string default
d-i finish-install/reboot_in_progress note
```

### Rocky / Fedora (kickstart)

`data/autoinstall/rocky/workstation.ks`:

```
text
lang en_GB.UTF-8
keyboard gb
timezone Europe/London --utc
network --bootproto=dhcp --hostname={{HOSTNAME}}
rootpw --lock
user --name=admin --groups=wheel --password=$6$rounds=4096$... --iscrypted
sshkey --username=admin "ssh-ed25519 AAAA..."
bootloader --location=mbr
clearpart --all --initlabel
autopart --type=lvm
%packages
@^minimal-environment
openssh-server
%end
```

### Windows 11 / Server (autounattend)

`data/autoinstall/windows/kiosk.xml`: standard `<unattend>` document — see [Microsoft's autounattend reference](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/). Placeholders work inside any text node:

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## Windows Notes

Windows installs are SMB-driven. When an image has an autounattend file attached, Bootimus:

1. Stages `AutoUnattend.xml` on the SMB install share when patching `boot.wim`.
2. Patches `startnet.cmd` so WinPE copies it to `X:\AutoUnattend.xml` (the local RAM disk) at boot.
3. Launches Setup as `setup.exe /unattend:X:\AutoUnattend.xml`.

Without the per-image autounattend file, Setup runs interactively as before.

**Reboot resilience.** WinPE reboots mid-install and reconnects from the same client IP. The bundled Samba config sets `reset on zero vc = yes` and disables oplocks so the second `net use` doesn't trip on stale session state. If you've replaced `data/smb/smb.conf` with your own, mirror these settings.

**Undoing the patch.** The first SMB patch keeps a pristine copy of `boot.wim` as `boot.wim.orig` next to it. **Images** → **Unpatch SMB** restores that original and removes the image's SMB share, so the ISO boots exactly as uploaded. Images patched by older Bootimus versions have no backup — re-extract the image first (which produces a fresh pristine copy), then unpatch.

## REST API

Everything in the UI is also a REST call.

```bash
# List all auto-install files
curl -u admin:pw http://localhost:8081/api/autoinstall-files

# Read a file
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/get?distro=ubuntu&filename=default.yaml"

# Create or overwrite a file
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/save \
  -H "Content-Type: application/json" \
  -d '{"distro":"ubuntu","filename":"default.yaml","content":"#cloud-config\n..."}'

# Upload a file
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/upload \
  -F "distro=windows" \
  -F "filename=kiosk.xml" \
  -F "file=@./kiosk.xml"

# Download
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/download?distro=ubuntu&filename=default.yaml" -o default.yaml

# Delete
curl -u admin:pw -X POST "http://localhost:8081/api/autoinstall-files/delete?distro=ubuntu&filename=default.yaml"
```

Attach a file to an image:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

Attach a file to a client:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

The auto-install endpoint clients hit at boot:

```
GET /autoinstall/<image-filename>/?mac=<mac>
```

The `mac` query param is appended automatically by the boot menu so per-client overrides resolve correctly.

## Troubleshooting

### 404 from `/autoinstall/...`

`no auto-install configuration for this image/client` — nothing is attached at any level of the resolution chain. Either attach a file to the image, the client, or its group, or check that `auto_install_file` actually points at a file that exists under `data/autoinstall/`.

### Placeholders rendered literally

`{{HOSTNAME}}` showing up as the literal string in the installed system means the file was served before the substitution ran — usually because the client booted by IP only and the request didn't include a `mac` query param. Confirm the boot menu is generating URLs of the form `/autoinstall/<iso>/?mac=<mac>`.

### Wrong file served

Resolution is most-specific-first. If a client has its own override and you don't expect it, that's why the image-level default isn't being used. Check the server log line:

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

The `source:` field tells you exactly which slot won.

### Windows Setup runs interactively

- The image must have an autounattend file attached (image properties → Auto-Install).
- Re-patch `boot.wim` after attaching: **Images** → **Patch SMB** (or it re-patches automatically on next boot).
- Confirm the SMB share is reachable from the client (`net view \\<server>` from WinPE).

### "AutoUnattend.xml not on share, running interactive setup"

Logged by `startnet.cmd` when the file isn't where it expects. Either the staging step failed (check the Bootimus server log around the time of the patch) or the SMB share lost the file. Re-run the SMB patch from the image properties.

### `net use fails after VM reboot`

Fixed in 0.1.58 by enabling `reset on zero vc = yes` in the bundled Samba config. If you maintain a custom `smb.conf`, add:

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## Next Steps

- See [Image Management](images.md) for attaching files to images.
- See [Client Management](clients.md) for per-client overrides.
- See [Distro Profiles](distro-profiles.md) for the underlying profile IDs that map to library subdirectories.
