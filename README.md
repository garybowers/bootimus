# Bootimus - Modern PXE/HTTP Boot Server

**A production-ready, self-contained PXE and HTTP boot server** written in Go with embedded iPXE bootloaders, SQLite/PostgreSQL support, and a full-featured web admin interface. Deploy in seconds with a single binary or Docker container.

## There Be Dragons!

This is an early-stage work-in-progress project - there may be bugs. Please raise an issue for any unexpected behaviour you encounter.

### AI Disclosure

I've used Claude CLI to help with some parts of this project - mostly making the web UI pretty, as I'm NOT a frontend developer. I also used it to generate the docs, but I review them manually - no automatically-generated AI code goes into the project without review from myself.

## Features

- **Single binary, zero config**: Everything bundled - bootloaders, web UI, database. Just run it
- **50+ distro support**: Automatic kernel/initrd extraction with a generic fallback scanner for unknown ISOs
- **Built-in diagnostic tools**: GParted, Clonezilla, Memtest86+, SystemRescue, ShredOS, Netboot.xyz - one-click download and enable from the admin UI
- **Custom tools**: Add your own PXE-bootable tools with configurable boot methods (kernel, chain, memdisk)
- **Per-client access control**: Assign specific images per MAC address, toggle public image visibility per client
- **Client auto-discovery**: Clients are automatically detected when they PXE boot, like DHCP leases - promote to static when ready
- **Next boot action**: Set a one-time boot image for a client with optional Wake-on-LAN - auto-clears after use
- **Hardware inventory**: Automatic collection of CPU, memory, manufacturer, serial number, and NIC info from PXE clients
- **JWT authentication**: Secure token-based auth with a dedicated login page, replacing browser basic auth dialogs
- **LDAP/Active Directory**: Optional LDAP backend with group-based admin access - local accounts always work as fallback
- **Swappable bootloaders**: Ship with embedded iPXE, or bring your own custom bootloader sets
- **Secure Boot**: Microsoft-signed shim bootloader for UEFI Secure Boot environments
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
- **[Thin OS Boot Method](docs/thinos.md)** - Universal ISO boot via memdisk
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
| **HDT** | Hardware inventory and diagnostics |

Download URLs are shown in the UI and can be overridden to point at local mirrors or newer versions.

### Custom Tools

You can add your own PXE-bootable tools via the **"+ Add Custom Tool"** button in the Tools section. Custom tools support:

- **Boot methods**: Kernel/initrd, chain (EFI), or memdisk
- **Archive types**: ZIP, single binary, or ISO
- **Boot parameters**: With `{{HTTP_URL}}` placeholder for server URL substitution
- **Download from URL**: Specify any HTTP/HTTPS URL for the tool files

## Bootloader Management

Bootimus ships with embedded iPXE bootloaders for UEFI (x86_64, ARM64), Legacy BIOS, and Secure Boot. You can also use custom bootloader sets:

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

Configure your DHCP server to point clients to Bootimus. Example for ISC DHCP:

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

### Permission Denied on Port 69

```bash
# Run as root
sudo ./bootimus serve

# Or use Docker with NET_BIND_SERVICE
docker run --cap-add NET_BIND_SERVICE ...

# Or use non-privileged port
./bootimus serve --tftp-port 6969
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

### Database Connection Failed

```bash
# Check SQLite database
ls -la data/bootimus.db

# For PostgreSQL, test connection
psql -h localhost -U bootimus -d bootimus
```

## Licence

Licensed under the Apache Licence, Version 2.0. See [LICENSE](LICENSE) for details.

Copyright 2025 Bootimus Contributors

## Contributing

Contributions welcome! Please open an issue or pull request.

## Links

- **GitHub**: https://github.com/garybowers/bootimus
- **Docker Hub**: https://hub.docker.com/r/garybowers/bootimus
- **Documentation**: https://github.com/garybowers/bootimus/tree/main/docs
