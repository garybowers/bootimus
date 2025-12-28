# Bootimus - Modern PXE/HTTP Boot Server

**A production-ready, self-contained PXE and HTTP boot server** written in Go with embedded iPXE bootloaders, SQLite/PostgreSQL support, and a full-featured web admin interface. Deploy in seconds with a single binary or Docker container.

**Why Bootimus over iVentoy?**
- ✅ **Self-contained**: Single binary with embedded bootloaders and web UI (no external dependencies)
- ✅ **Modern**: Built with Go, RESTful API, proper HTTP Basic Auth, SQLite/PostgreSQL support
- ✅ **Lightweight**: Runs anywhere - Docker, systemd, bare metal (amd64/arm64)
- ✅ **Database-backed**: Track boot logs, client statistics, and granular MAC-based access control
- ✅ **Production-ready**: Proper logging, metrics, API-first design, multi-arch Docker images
- ✅ **Open Source**: Apache 2.0 license, actively maintained

## Features

### Core Functionality
- **TFTP Server**: Traditional PXE boot support (port 69)
- **HTTP Server**: iPXE and HTTP boot (port 8080)
- **Embedded iPXE Bootloaders**: No external files needed - bootloaders baked into the binary
- **Dynamic Boot Menus**: Generated on-the-fly based on client MAC address permissions
- **ISO Upload**: Web-based drag-and-drop upload (up to 10GB)
- **Automatic Scanning**: Auto-detect ISOs in data directory

### Database & Storage
- **SQLite by Default**: Zero-configuration embedded database (just works)
- **PostgreSQL Optional**: Scale to enterprise with PostgreSQL backend
- **MAC Address Access Control**: Fine-grained per-client ISO permissions
- **Boot Logging**: Track every boot attempt with statistics
- **Filesystem Fallback**: Run without database for simple deployments

### Admin Interface
- **Web Admin Panel**: Full-featured UI on separate port (8081)
- **HTTP Basic Auth**: Auto-generated password on first run
- **User Management**: Multi-user support with role-based access
- **RESTful API**: Complete REST API for automation
- **Client Management**: Add/edit/delete clients, assign images
- **Image Management**: Upload, download from URL, scan, enable/disable ISOs
- **Boot Logs**: Real-time boot attempt tracking with live streaming

### Deployment
- **Single Binary**: Fully self-contained with embedded assets
- **Docker Ready**: Multi-arch images (amd64/arm64) on Docker Hub
- **Systemd Support**: Production systemd service files
- **Zero Config**: Sensible defaults, works out of the box

## Quick Start

### Docker (Recommended)

```bash
# Create data directory
mkdir -p data

# Run with SQLite (no database container needed!)
docker run -d \
  --name bootimus \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -p 8081:8081/tcp \
  -v $(pwd)/data:/data \
  garybowers/bootimus:latest

# Check logs for admin password
docker logs bootimus | grep "Admin password"

# Access admin interface
open http://localhost:8081
```

**Directory Structure**: Bootimus automatically creates subdirectories:
- `/data/isos/` - ISO image files and extracted boot files (in subdirectories per ISO)
- `/data/bootloaders/` - Custom bootloader files (optional)

### Standalone Binary

```bash
# Download binary (or build from source)
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64
chmod +x bootimus-amd64

# Create data directory
mkdir -p data

# Run (SQLite mode - no database required!)
./bootimus-amd64 serve

# Admin panel: http://localhost:8081
# Admin password shown in startup logs
```

### Docker Compose with PostgreSQL

```bash
# Clone repo
git clone https://github.com/garybowers/bootimus
cd bootimus

# Start with PostgreSQL
docker-compose up -d

# View logs
docker-compose logs -f bootimus
```

## Configuration

Bootimus uses sensible defaults and requires minimal configuration.

### Configuration Precedence
1. Command-line flags
2. Environment variables (prefixed with `BOOTIMUS_`)
3. Configuration file (`bootimus.yaml`)

### Example Configuration File

```yaml
# bootimus.yaml
tftp_port: 69
http_port: 8080
admin_port: 8081
data_dir: ./data          # Base data directory (creates subdirs: isos/, bootloaders/)
server_addr: ""           # Auto-detected if not specified

# SQLite mode (default - no configuration needed!)
db:
  disable: false          # false = use SQLite, true = no database

# PostgreSQL mode (optional)
# db:
#   host: localhost
#   port: 5432
#   user: bootimus
#   password: bootimus
#   name: bootimus
#   sslmode: disable
#   disable: false
```

### Environment Variables

```bash
# Server settings
export BOOTIMUS_TFTP_PORT=69
export BOOTIMUS_HTTP_PORT=8080
export BOOTIMUS_ADMIN_PORT=8081
export BOOTIMUS_DATA_DIR=/var/lib/bootimus/data

# Database settings
export BOOTIMUS_DB_DISABLE=false        # Use SQLite
# export BOOTIMUS_DB_HOST=postgres      # Use PostgreSQL
# export BOOTIMUS_DB_PASSWORD=secret

./bootimus serve
```

## Database Modes

### SQLite Mode (Default)

SQLite is **enabled by default** - no configuration required!

```bash
# Run with SQLite (default)
./bootimus serve

# Database automatically created at: <data_dir>/bootimus.db
# Perfect for: Single-server deployments, testing, small networks
```

### PostgreSQL Mode

For enterprise deployments with high concurrency:

```yaml
# bootimus.yaml
db:
  host: postgres.example.com
  port: 5432
  user: bootimus
  password: secretpassword
  name: bootimus
  sslmode: require
  disable: false
```

### No Database Mode

Filesystem-only mode (all ISOs public to all clients):

```bash
./bootimus serve --db-disable
```

## Architecture

```
bootimus/
├── cmd/                    # Cobra CLI commands
│   ├── root.go            # Root command and configuration
│   └── serve.go           # Server start command
├── internal/
│   ├── models/            # Database models (GORM)
│   ├── database/          # PostgreSQL database layer
│   ├── storage/           # SQLite storage layer
│   ├── server/            # TFTP/HTTP server
│   ├── admin/             # Admin API handlers
│   ├── auth/              # HTTP Basic Auth
│   └── bootloaders/       # Embedded iPXE bootloaders
├── web/static/            # Embedded web UI
├── main.go                # Entry point
├── Dockerfile             # Docker build
└── docker-compose.yml     # Docker Compose config
```

## Database Schema

### Tables

**clients** - Network boot clients
- `id` - Primary key
- `mac_address` - Unique MAC address (normalized)
- `name` - Client name/description
- `description` - Additional details
- `enabled` - Whether client can boot
- `last_boot` - Last boot timestamp
- `boot_count` - Number of boots
- `allowed_images` - JSON array of allowed image filenames (SQLite)

**images** - ISO images
- `id` - Primary key
- `name` - Display name
- `filename` - ISO filename (unique)
- `description` - Image description
- `size` - File size in bytes
- `enabled` - Whether image is available
- `public` - If true, available to all clients
- `boot_count` - Usage statistics
- `last_booted` - Last boot timestamp

**client_images** - Many-to-many relationship (PostgreSQL only)
- Links clients to their allowed images

**boot_logs** - Boot attempt logs
- `mac_address` - Client MAC
- `image_name` - Image requested
- `success` - Boot success/failure
- `error_msg` - Error details (if failed)
- `ip_address` - Client IP
- `created_at` - Timestamp

## How It Works

### Boot Flow

1. **Client PXE Boot**:
   - Client sends DHCP request
   - DHCP server responds with boot server IP and bootloader filename
   - Client downloads iPXE bootloader via TFTP from Bootimus

2. **iPXE Chainloading**:
   - iPXE bootloader starts
   - Requests dynamic menu: `GET /menu.ipxe?mac=<MAC>`
   - Bootimus generates menu based on MAC address permissions

3. **Menu Display**:
   - iPXE displays ASCII menu with available ISOs
   - User selects an ISO
   - iPXE boots ISO via HTTP using `sanboot`

4. **Logging**:
   - Bootimus logs boot attempt (client, image, success/failure)
   - Updates statistics (boot counts, last boot time)

### Access Control

**SQLite/PostgreSQL Mode**:
- Public images: Available to all clients
- Private images: Assigned per MAC address
- Disabled clients: Cannot boot any images
- Boot attempts logged with success/failure

**No Database Mode**:
- All ISOs available to all clients
- No logging or statistics

## DHCP Configuration

Configure your DHCP server to point network boot clients to Bootimus. Replace `192.168.1.10` with your Bootimus server IP address.

### ISC DHCP Server (dhcpd.conf)

```
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    next-server 192.168.1.10;  # Bootimus server IP

    # Chain to HTTP after iPXE loads
    if exists user-class and option user-class = "iPXE" {
        filename "http://192.168.1.10:8080/menu.ipxe?mac=${net0/mac}";
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

### Dnsmasq

```
dhcp-range=192.168.1.100,192.168.1.200,12h
dhcp-boot=tag:!ipxe,undionly.kpxe,192.168.1.10
dhcp-boot=tag:ipxe,http://192.168.1.10:8080/menu.ipxe

# UEFI support
dhcp-match=set:efi-x86_64,option:client-arch,7
dhcp-boot=tag:efi-x86_64,tag:!ipxe,ipxe.efi,192.168.1.10
```

### MikroTik RouterOS

Configure via CLI or WebFig:

```
# Via CLI
/ip dhcp-server network
set [find] next-server=192.168.1.10 boot-file-name=undionly.kpxe

# For UEFI support, use DHCP options
/ip dhcp-server option
add code=60 name=pxe-client value="'PXEClient'"
add code=66 name=tftp-server value="'192.168.1.10'"
add code=67 name=bootfile-bios value="'undionly.kpxe'"
add code=67 name=bootfile-uefi value="'ipxe.efi'"

/ip dhcp-server network
set [find] dhcp-option=pxe-client,tftp-server next-server=192.168.1.10
```

**Via WebFig:**
1. Navigate to **IP > DHCP Server > Networks**
2. Double-click your network
3. Set **Next Server**: `192.168.1.10`
4. Set **Boot File Name**: `undionly.kpxe` (BIOS) or `ipxe.efi` (UEFI)
5. Click **OK**

**Note**: MikroTik doesn't natively support iPXE detection. Clients will TFTP boot iPXE, then chain to HTTP automatically.

### Ubiquiti EdgeRouter

Configure via CLI:

```bash
configure

# Set TFTP server for network boot
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-server 192.168.1.10
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-name undionly.kpxe

# For UEFI support (optional)
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "option arch code 93 = unsigned integer 16;"
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "if option arch = 00:07 { filename &quot;ipxe.efi&quot;; } else { filename &quot;undionly.kpxe&quot;; }"

commit
save
```

**Via Web UI:**
1. Navigate to **Services > DHCP Server**
2. Select your DHCP server (e.g., `LAN`)
3. Under **Actions**, click **Edit**
4. Scroll to **PXE Settings**:
   - **Boot File**: `undionly.kpxe` (BIOS) or `ipxe.efi` (UEFI)
   - **Boot Server**: `192.168.1.10`
5. Click **Save**

**Note**: Replace `LAN` with your actual shared network name if different.

## Admin Interface

Access the web admin panel at: `http://your-server:8081/`

### First-Time Login

On first run, Bootimus generates a random admin password:

```
Admin password: a3f9c8e2d1b0
```

- **Username**: `admin`
- **Password**: Check server logs or `.admin_password` file
- Password stored as SHA-256 hash for security

### Dashboard Features

- **System Statistics**: Total clients, active clients, images, boot counts
- **Client Management**: Add/edit/delete clients, assign images
- **Image Management**: Upload ISOs, scan directory, enable/disable
- **Boot Logs**: View recent boot attempts with success/failure status

### Admin API

All admin endpoints require HTTP Basic Auth (`admin:<password>`).

**Stats**
- `GET /api/stats` - System statistics

**Clients**
- `GET /api/clients` - List all clients
- `GET /api/clients?mac=<MAC>` - Get client by MAC
- `POST /api/clients` - Create client
- `PUT /api/clients?mac=<MAC>` - Update client
- `DELETE /api/clients?mac=<MAC>` - Delete client
- `POST /api/clients/assign` - Assign images to client

**Images**
- `GET /api/images` - List all images
- `GET /api/images?filename=<name>` - Get image
- `PUT /api/images?filename=<name>` - Update image
- `DELETE /api/images?filename=<name>` - Delete image
- `POST /api/images/upload` - Upload ISO (multipart/form-data)
- `POST /api/images/download` - Download ISO from URL
- `GET /api/downloads` - List active downloads
- `GET /api/downloads/progress?filename=<name>` - Get download progress
- `POST /api/scan` - Scan data directory for new ISOs

**Logs**
- `GET /api/logs?limit=<N>` - Get boot logs (default 100)

### API Examples

```bash
# Get admin password from logs
docker logs bootimus | grep "Admin password"

# Set password variable
ADMIN_PASS="a3f9c8e2d1b0"

# Get system stats
curl -u admin:$ADMIN_PASS http://localhost:8081/api/stats

# Create a client
curl -u admin:$ADMIN_PASS -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Machine 1",
    "description": "Test workstation",
    "enabled": true
  }'

# Upload an ISO
curl -u admin:$ADMIN_PASS -X POST http://localhost:8081/api/images/upload \
  -F "file=@ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"

# Download an ISO from URL
curl -u admin:$ADMIN_PASS -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'

# Check download progress
curl -u admin:$ADMIN_PASS http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso

# Assign images to client (SQLite mode)
curl -u admin:$ADMIN_PASS -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "image_filenames": ["ubuntu-24.04-live-server-amd64.iso", "debian-12.iso"]
  }'

# Get boot logs
curl -u admin:$ADMIN_PASS http://localhost:8081/api/logs?limit=10
```

## Production Deployment

### Docker with SQLite (Simplest)

```bash
docker run -d \
  --name bootimus \
  --restart unless-stopped \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -p 8081:8081/tcp \
  -v /opt/bootimus/data:/data \
  garybowers/bootimus:latest
```

### Docker Compose with PostgreSQL

```yaml
version: '3.8'

services:
  bootimus:
    image: garybowers/bootimus:latest
    container_name: bootimus
    restart: unless-stopped
    cap_add:
      - NET_BIND_SERVICE
    ports:
      - "69:69/udp"
      - "8080:8080/tcp"
      - "8081:8081/tcp"
    volumes:
      - ./data:/data
      - ./bootimus.yaml:/app/bootimus.yaml
    environment:
      - BOOTIMUS_DB_HOST=postgres
      - BOOTIMUS_DB_PASSWORD=secretpassword
    depends_on:
      - postgres

  postgres:
    image: postgres:17-alpine
    container_name: bootimus-db
    restart: unless-stopped
    environment:
      - POSTGRES_USER=bootimus
      - POSTGRES_PASSWORD=secretpassword
      - POSTGRES_DB=bootimus
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

### Systemd Service

```ini
[Unit]
Description=Bootimus PXE/HTTP Boot Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/bootimus
ExecStart=/opt/bootimus/bootimus serve
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
# Install
sudo cp bootimus.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable bootimus
sudo systemctl start bootimus

# Check status
sudo systemctl status bootimus

# View logs
sudo journalctl -u bootimus -f
```

## Building from Source

### Local Build

```bash
# Clone repository
git clone https://github.com/garybowers/bootimus
cd bootimus

# Install dependencies
go mod download

# Build
make build DOCKER_USER=youruser

# Run
./bootimus serve
```

### Multi-Architecture Build

```bash
# Build for AMD64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bootimus-amd64

# Build for ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bootimus-arm64

# Build Docker images
make build DOCKER_USER=youruser
make docker DOCKER_USER=youruser
```

### GitHub Actions

The included GitHub Actions workflow automatically:
- Builds AMD64 and ARM64 binaries
- Creates multi-arch Docker images
- Pushes to Docker Hub
- Creates GitHub releases

Trigger on git tags:
```bash
git tag v1.0.0
git push origin v1.0.0
```

## Troubleshooting

### Permission Denied on Port 69

```bash
# Run as root
sudo ./bootimus serve

# Or use Docker with CAP_NET_BIND_SERVICE
docker run --cap-add NET_BIND_SERVICE ...

# Or use non-privileged port
./bootimus serve --tftp-port 6969
```

### Database Connection Failed

```bash
# Check SQLite database
ls -la data/bootimus.db

# For PostgreSQL, test connection
psql -h localhost -U bootimus -d bootimus

# Run in no-database mode
./bootimus serve --db-disable
```

### No ISOs in Menu

```bash
# Check data directory
ls -la data/*.iso

# Scan for ISOs via API
curl -u admin:password -X POST http://localhost:8081/api/scan

# Check client permissions (SQLite)
sqlite3 data/bootimus.db "SELECT * FROM clients;"
sqlite3 data/bootimus.db "SELECT * FROM images;"

# Enable public access to images
curl -u admin:password -X PUT http://localhost:8081/api/images?filename=ubuntu.iso \
  -H "Content-Type: application/json" \
  -d '{"public": true, "enabled": true}'
```

### Check Boot Logs

```bash
# Via API
curl -u admin:password http://localhost:8081/api/logs?limit=10

# Via SQLite
sqlite3 data/bootimus.db "SELECT * FROM boot_logs ORDER BY created_at DESC LIMIT 10;"

# Via PostgreSQL
psql -h localhost -U bootimus -d bootimus -c "SELECT * FROM boot_logs ORDER BY created_at DESC LIMIT 10;"
```

## Comparison: Bootimus vs iVentoy

| Feature | Bootimus | iVentoy |
|---------|----------|---------|
| **Language** | Go | C |
| **Single Binary** | ✅ Yes | ❌ No |
| **Embedded Bootloaders** | ✅ Yes | ❌ No |
| **Database** | SQLite + PostgreSQL | File-based |
| **Web UI** | ✅ Modern REST API | Basic HTML |
| **Authentication** | HTTP Basic Auth | None |
| **Boot Logging** | ✅ Full tracking | Limited |
| **MAC-based ACL** | ✅ Granular | ❌ No |
| **ISO Upload** | ✅ Web upload | Manual copy |
| **Docker Support** | ✅ Multi-arch | Limited |
| **API-First** | ✅ RESTful API | ❌ No |
| **Multi-tenancy** | ✅ Client isolation | ❌ No |
| **License** | Apache 2.0 | GPL |

## Security Considerations

- **Read-only TFTP**: TFTP server is read-only (no write operations)
- **Path Sanitization**: All file paths sanitized to prevent directory traversal
- **MAC Address Verification**: ISOs served only to authorized clients
- **Admin Authentication**: HTTP Basic Auth with SHA-256 password hashing
- **Separate Admin Port**: Admin interface isolated from boot network (port 8081)
- **Database Security**: SQLite file permissions, PostgreSQL SSL support
- **Audit Logs**: All boot attempts logged with client/image/success tracking
- **Firewall**: Limit TFTP/HTTP ports to local network only

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

Copyright 2025 Bootimus Contributors

## Contributing

Contributions welcome! Please open an issue or pull request.

## Links

- **GitHub**: https://github.com/garybowers/bootimus
- **Docker Hub**: https://hub.docker.com/r/garybowers/bootimus
