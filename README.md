# Bootimus - Modern PXE/HTTP Boot Server

**A production-ready, self-contained PXE and HTTP boot server** written in Go with embedded iPXE bootloaders, SQLite/PostgreSQL support, and a full-featured web admin interface. Deploy in seconds with a single binary or Docker container.

## There Be Dragons!

This is an early-stage work-in-progress project - there may be bugs. Please raise an issue for any unexpected behaviour you encounter.

### AI Disclosure

I've used Claude CLI to help with some parts of this project - mostly making the web UI pretty, as I'm NOT a frontend developer. I also used it to generate the docs, but I review them manually - no automatically-generated AI code goes into the project without review from myself.

## Features

- **Single binary, zero config**: Everything bundled - bootloaders, web UI, database. Just run it
- **Standalone PXE**: Built-in proxyDHCP responder lets Bootimus drive PXE on any LAN without touching the existing DHCP server
- **50+ distro support**: Automatic kernel/initrd extraction with a generic fallback scanner for unknown ISOs
- **Built-in diagnostic tools**: GParted, Clonezilla, Memtest86+, SystemRescue, ShredOS, Netboot.xyz - one-click download and enable from the admin UI
- **Custom tools**: Add your own PXE-bootable tools with configurable boot methods (kernel, chain, memdisk)
- **Per-client access control**: Assign specific images per MAC address, toggle public image visibility per client
- **Client auto-discovery**: Clients are automatically detected when they PXE boot, like DHCP leases - promote to static when ready
- **Next boot action**: Set a one-time boot image for a client with optional Wake-on-LAN - auto-clears after use
- **Hardware inventory**: Automatic collection of CPU, memory, manufacturer, serial number, and NIC info from PXE clients
- **JWT authentication**: Secure token-based auth with a dedicated login page
- **LDAP/Active Directory**: Optional LDAP backend with group-based admin access - local accounts always work as fallback
- **Swappable bootloaders**: Ship with embedded iPXE, or bring your own custom bootloader sets
- **Modern admin UI**: Sidebar navigation, consistent toolbars, real-time colour-coded logs, REST API
- **Multi-database**: SQLite out of the box, PostgreSQL for production
- **Docker and bare metal**: Multi-arch images (amd64/arm64) or a single static binary

## Screenshots

| Admin Dashboard | Upload ISOs | Download from URL |
|----------------|-------------|-------------------|
| ![Admin Interface](docs/admin_1.png) | ![Upload](docs/admin_2.png) | ![Download](docs/admin_3.png) |

## Quick Start

### Docker (Recommended)

```bash
# Create data directory
mkdir -p data

# Run with SQLite (no database container needed)
docker run -d \
  --name bootimus \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -p 8081:8081/tcp \
  -v $(pwd)/data:/data \
  garybowers/bootimus:latest

# Check logs for admin password
docker logs bootimus | grep "Password"

# Access admin interface
open http://localhost:8081
```

### Standalone Binary

```bash
# Download binary
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64
chmod +x bootimus-amd64

# Run (SQLite mode - no database required)
./bootimus-amd64 serve

# Admin panel: http://localhost:8081
```

### Docker Compose

```bash
git clone https://github.com/garybowers/bootimus
cd bootimus
docker-compose up -d
```

## Documentation

- **[Deployment Guide](docs/deployment.md)** - Docker, binary, networking, and storage
- **[Image Management](docs/images.md)** - Upload ISOs, extract kernels, netboot support
- **[USB Appliance](docs/appliance.md)** - Flashable Alpine+bootimus image for portable PXE servers
- **[Admin Console](docs/admin.md)** - Web UI and REST API reference
- **[DHCP Configuration](docs/dhcp.md)** - Configure your DHCP server
- **[Client Management](docs/clients.md)** - MAC-based access control, auto-discovery, next boot
- **[Authentication](docs/authentication.md)** - JWT auth, LDAP/Active Directory setup
- **[Distro Profiles](docs/distro-profiles.md)** - Data-driven distro detection and boot params

## Boot Tools

Bootimus includes a built-in tools system for diagnostic and utility software. Tools can be downloaded and enabled from the admin UI under the **Tools** section. When enabled, they appear in a **Tools** submenu in the PXE boot menu.

| Tool | Description |
|------|-------------|
| **GParted Live** | Partition editor for managing disk partitions |
| **Clonezilla Live** | Disk cloning and imaging |
| **Memtest86+** | Memory testing and diagnostics |
| **SystemRescue** | Full rescue toolkit (file recovery, disk repair, network tools) |
| **ShredOS** | Secure disk wiping based on nwipe |
| **Netboot.xyz** | Chainloads into hundreds of OS installers and tools |

Download URLs are shown in the UI and can be overridden to point at local mirrors or newer versions.

### Custom Tools

You can add your own PXE-bootable tools via the **"+ Add Custom Tool"** button in the Tools section. Custom tools support:

- **Boot methods**: Kernel/initrd, chain (EFI), or memdisk
- **Archive types**: ZIP, single binary, or ISO
- **Boot parameters**: With `{{HTTP_URL}}` placeholder for server URL substitution
- **Download from URL**: Specify any HTTP/HTTPS URL for the tool files

## Bootloader Management

Bootimus ships with embedded iPXE bootloaders for UEFI (x86_64, ARM64) and Legacy BIOS. You can also use custom bootloader sets:

1. Create a subfolder in `{data-dir}/bootloaders/` (e.g. `ipxe-custom/`)
2. Place your custom bootloader files in it
3. Select the set from the **Bootloaders** section in the admin UI

The built-in set is always available as a fallback. Files not present in the active custom set are served from the built-in set automatically.

## Supported Distributions

### Arch-based
- Arch Linux, CachyOS, EndeavourOS, Manjaro, Garuda, Artix, BlackArch, Parabola, SteamOS

### Debian/Ubuntu-based
- Ubuntu (all flavours), Debian, Linux Mint, Pop!_OS, Kali, Parrot, Zorin, elementary OS, MX Linux, antiX, Devuan, PureOS, Deepin, LMDE, TrueNAS SCALE, Proxmox

### Red Hat-based
- Fedora, CentOS, Rocky Linux, AlmaLinux, Oracle Linux, Nobara, Mageia

### Other Linux
- openSUSE, NixOS, Alpine, Gentoo, Void, Slackware, Solus, Tiny Core, Clear Linux

### Other
- FreeBSD, Windows (via wimboot)

For distributions not in this list, the **generic boot scanner** automatically walks the ISO filesystem to find kernel and initrd files and attempts to extract boot parameters from syslinux/grub configuration files.

Need to add a new distro? Create a **custom distro profile** from the admin UI — no code change required. See the [Distro Profiles Guide](docs/distro-profiles.md). You can also contribute profiles to the official list via pull request.

## ISO Organisation

ISOs can be organised into groups by placing them in subdirectories:

```
data/isos/
├── ubuntu-24.04.iso              # ungrouped, appears in main menu
├── linux/                        # creates "linux" group
│   ├── debian-12.iso             # in "linux" submenu
│   └── servers/                  # creates "servers" subgroup
│       └── truenas-scale.iso     # in "linux > servers" submenu
└── windows/                      # creates "windows" group
    └── win11.iso                 # in "windows" submenu
```

Groups are auto-created on startup and when scanning for ISOs. They can also be managed manually via the admin UI.

## Roadmap

- iPXE colour theming (blocked on iPXE firmware compatibility)
- NetBSD/OpenBSD support

## Why Bootimus Over iVentoy?

| Feature | Bootimus | iVentoy |
|---------|----------|---------|
| **Language** | Go | C |
| **Single Binary** | Yes | No |
| **Embedded Bootloaders** | Yes | No |
| **Standalone PXE** | Built-in proxyDHCP — no DHCP reconfig needed | Requires external DHCP changes |
| **Database** | SQLite / PostgreSQL | File-based |
| **Web UI** | Modern sidebar UI with REST API | Basic HTML |
| **Authentication** | JWT + LDAP/AD | None |
| **Boot Logging** | Full tracking with live streaming | Limited |
| **MAC-based ACL** | Granular per-client | No |
| **ISO Upload** | Web upload + URL download | Manual copy |
| **Boot Tools** | GParted, Clonezilla, Memtest86+, etc. | No |
| **Bootloader Management** | Swappable sets via UI | No |
| **Docker Support** | Multi-arch | Limited |
| **API-First** | RESTful API | No |
| **Licence** | Apache 2.0 | GPL |

## DHCP Configuration

Bootimus has two options for the DHCP side of PXE.

### Option 1: Built-in proxyDHCP (recommended)

Bootimus ships with a built-in proxyDHCP responder. Enable it and your existing DHCP server (router, Pi-hole, Windows DHCP, anything) needs **zero PXE configuration** — it keeps handing out IPs as normal, and Bootimus answers only the PXE-specific bits on the same broadcast domain.

```bash
bootimus serve --proxy-dhcp
# or: BOOTIMUS_PROXY_DHCP_ENABLED=true
```

Binds UDP/67; needs `CAP_NET_BIND_SERVICE` or root. Off by default so existing installs aren't surprised.

### Option 2: Configure your DHCP server manually

If you'd rather keep PXE config on your existing DHCP server, point it at Bootimus. Example for ISC DHCP:

```conf
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    next-server 192.168.1.10;  # Bootimus server IP

    # Chain to HTTP after iPXE loads
    if exists user-class and option user-class = "iPXE" {
        filename "http://192.168.1.10:8080/menu.ipxe";
    }
    # UEFI systems
    elsif option arch = 00:07 or option arch = 00:09 {
        filename "ipxe.efi";
    }
    # Legacy BIOS
    else {
        filename "undionly.kpxe";
    }
}
```

See [DHCP Configuration Guide](docs/dhcp.md) for Dnsmasq, MikroTik, Ubiquiti, and more.

## Building from Source

```bash
# Clone repository
git clone https://github.com/garybowers/bootimus
cd bootimus

# Build and run locally
make build
make run

# Build container image locally
make docker-build

# Start services via docker compose
make docker-up

# Build all platform binaries for GitHub release
make release

# Build and push multi-arch container to Docker Hub
make docker-push

# Push amd64 only (faster, skips arm64 QEMU emulation)
make docker-push PLATFORMS=linux/amd64
```

Run `make help` for all available targets.

## Security Considerations

- **Read-only TFTP**: TFTP server is read-only (no write operations)
- **Path sanitisation**: All file paths sanitised to prevent directory traversal
- **MAC address verification**: ISOs served only to authorised clients
- **Admin authentication**: JWT token-based auth with bcrypt password hashing, optional LDAP/AD backend
- **Separate admin port**: Admin interface isolated from boot network
- **Audit logs**: All boot attempts logged with client/image/success tracking

## Troubleshooting

### Permission Denied on Port 67 or 69

Bootimus binds privileged UDP ports: 69 for TFTP, and 67 if `--proxy-dhcp` is enabled.

```bash
# Run as root
sudo ./bootimus serve

# Or grant capabilities once to the binary
sudo setcap 'cap_net_bind_service=+ep' ./bootimus

# Or use Docker with NET_BIND_SERVICE (default image already runs as root)
docker run --cap-add NET_BIND_SERVICE ...

# Or use a non-privileged TFTP port
./bootimus serve --tftp-port 6969
```

### Windows Unattended Install (`--windows-smb`)

Off by default. When enabled, bootimus starts an isolated `smbd` child process that exposes each extracted Windows ISO as a read-only guest SMB share, and patches the WinPE `boot.wim` so `setup.exe` auto-mounts that share and runs — no manual `net use` from the WinPE prompt.

Requirements:
- `smbd` on PATH (`apt install samba` / `pacman -S samba`). The Docker image ships it. If missing, the feature self-disables with a clear log line — bootimus still starts.
- `wimlib-imagex` (already required for Windows driver injection).
- Port 445 reachable from clients. Windows' `net use` ignores non-445 SMB ports, so `--windows-smb-port` is for testing only.
- If you're running standalone with `setcap` instead of root, grant `smbd` the same capability so the forked child can bind 445:

  ```bash
  sudo setcap 'cap_net_bind_service=+eip' /usr/sbin/smbd
  ```

  (Docker users skip this — the image runs as root.)

Enable it:

```bash
# Standalone
./bootimus serve --windows-smb

# Config file (bootimus.yaml)
windows_smb:
  enabled: true
  port: 445
```

Docker Compose — uncomment both the env var and the port mapping in [docker-compose.yml](docker-compose.yml):

```yaml
services:
  bootimus:
    environment:
      BOOTIMUS_WINDOWS_SMB_ENABLED: "true"
    ports:
      - "445:445/tcp"
```

Per-image state is shown with an **SMB** chip in the image list. The image-properties panel gains a **Patch SMB** button for re-running the patch on an already-extracted image (useful after enabling the flag, or if the first patch failed).

### Client Not Booting / No PXE Offer

Most common first-time PXE failures:

```bash
# 1. Check Bootimus is seeing the client's DHCP request
docker logs bootimus | grep -E 'proxyDHCP|TFTP'

# 2. Same broadcast domain — PXE DHCP is L2 broadcast.
#    In Docker, the container must use macvlan/ipvlan or network_mode: host.
#    The default bridge network will NOT work; docker0 traps broadcasts.

# 3. Two DHCP servers advertising PXE? Pick one.
#    If proxyDHCP is enabled, strip PXE options from your router's DHCP.

# 4. Firewall
sudo ufw allow 67/udp    # proxyDHCP (if enabled)
sudo ufw allow 69/udp    # TFTP
sudo ufw allow 8080/tcp  # HTTP boot

# 5. Check the client can reach HTTP
curl -v http://<bootimus-ip>:8080/menu.ipxe
```

### No ISOs in Menu

```bash
# Check data directory
ls -la data/isos/

# Scan for ISOs via API
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/scan

# Enable public access to images
curl -H "Authorization: Bearer $TOKEN" -X PUT http://localhost:8081/api/images?filename=ubuntu.iso \
  -H "Content-Type: application/json" \
  -d '{"public": true, "enabled": true}'
```

### UEFI Secure Boot Enabled on Target

Bootimus does not currently ship Microsoft-signed bootloaders. On machines with Secure Boot enabled, PXE boot fails with a signature-verification error.

**Fix**: disable Secure Boot in the target's firmware, or enrol Bootimus's iPXE EFI binary into the firmware's MOK keystore.

### Forgotten Admin Password

```bash
# Prints a fresh random password to the logs, then continues starting
./bootimus serve --reset-admin-password

# Via Docker
docker exec bootimus bootimus serve --reset-admin-password
docker logs bootimus | grep "New Password"
```

### Database Connection Failed

```bash
# Check SQLite database
ls -la data/bootimus.db

# For PostgreSQL, test connection
psql -h localhost -U bootimus -d bootimus
```

## Licence

Licensed under the Apache Licence, Version 2.0. See [LICENSE](LICENSE) for details.

Copyright 2025-2026 Bootimus Contributors

## Contributing

Contributions welcome! Please open an issue or pull request.

## Links

- **GitHub**: https://github.com/garybowers/bootimus
- **Docker Hub**: https://hub.docker.com/r/garybowers/bootimus
- **Documentation**: https://github.com/garybowers/bootimus/tree/main/docs
