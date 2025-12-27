# Bootimus - PXE/HTTP Boot Server with MAC Address Access Control

A production-ready PXE and HTTP boot server written in Go with PostgreSQL-backed MAC address and image access control. Bootimus serves boot files over TFTP (for traditional PXE) and HTTP (for HTTP boot/iPXE) with stateful management of client permissions.

## Features

- **TFTP Server**: Serves files for traditional PXE boot (default port 69)
- **HTTP Server**: Serves files for HTTP boot and iPXE (default port 8080)
- **Admin Server**: Separate admin interface on port 8081 (isolated from boot network)
- **Web Admin Interface**: Full-featured admin panel with embedded static UI
- **HTTP Basic Authentication**: Auto-generated password on first run for admin security
- **RESTful API**: Complete REST API for automation and integration
- **Dynamic Boot Menus**: Programmatically generated iPXE menus based on client MAC address
- **MAC Address Access Control**: PostgreSQL-backed permissions for fine-grained ISO access
- **ISO Upload**: Web-based drag-and-drop ISO upload (up to 10GB)
- **Stateful Tracking**: Logs boot attempts, tracks usage statistics
- **Cobra CLI**: Professional command-line interface with Viper configuration
- **Docker Support**: Production-ready Docker and docker-compose deployment
- **Single Binary**: Fully self-contained executable with embedded UI and menu generation
- **Filesystem Fallback**: Can run without database for simple deployments

## Architecture

```
bootimus/
├── cmd/                    # Cobra CLI commands
│   ├── root.go            # Root command and configuration
│   ├── serve.go           # Server start command
│   └── migrate.go         # Database migration command
├── internal/
│   ├── models/            # Database models (GORM)
│   │   └── models.go
│   ├── database/          # Database layer
│   │   └── database.go
│   └── server/            # Server implementation
│       └── server.go
├── main.go                # Entry point
├── Dockerfile             # Docker build
├── docker-compose.yml     # Docker Compose config
└── bootimus.example.yaml  # Example configuration
```

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Start the server with PostgreSQL
docker-compose up -d

# View logs
docker-compose logs -f bootimus

# Stop
docker-compose down
```

### Standalone Binary

```bash
# Build
go build -o bootimus

# Run with database
./bootimus serve

# Run without database (filesystem-only mode)
./bootimus serve --db-disable
```

## Configuration

Bootimus can be configured via:
1. Command-line flags
2. Environment variables (prefixed with `BOOTIMUS_`)
3. Configuration file (`bootimus.yaml`)

### Configuration File

```bash
# Copy example config
cp bootimus.example.yaml bootimus.yaml

# Edit configuration
nano bootimus.yaml
```

Example `bootimus.yaml`:

```yaml
tftp_port: 69
http_port: 8080
boot_dir: ./boot
data_dir: ./data
server_addr: ""  # Auto-detected

db:
  host: localhost
  port: 5432
  user: bootimus
  password: bootimus
  name: bootimus
  sslmode: disable
  disable: false
```

### Environment Variables

```bash
export BOOTIMUS_TFTP_PORT=69
export BOOTIMUS_HTTP_PORT=8080
export BOOTIMUS_DB_HOST=localhost
export BOOTIMUS_DB_PASSWORD=secretpassword
./bootimus serve
```

## Database Schema

### Tables

**clients** - Network boot clients
- `id` - Primary key
- `mac_address` - Unique MAC address
- `name` - Client name/description
- `enabled` - Whether client can boot
- `last_boot` - Last boot timestamp
- `boot_count` - Number of boots

**images** - ISO images
- `id` - Primary key
- `name` - Display name
- `filename` - ISO filename
- `enabled` - Whether image is available
- `public` - If true, available to all clients
- `boot_count` - Usage statistics

**client_images** - Many-to-many relationship
- Links clients to their allowed images

**boot_logs** - Boot attempt logs
- Tracks every boot attempt with client, image, success/failure

### Running Migrations

```bash
./bootimus migrate
```

## Commands

### serve

Start the PXE/HTTP boot server:

```bash
./bootimus serve [flags]

Flags:
  --tftp-port int        TFTP server port (default 69)
  --http-port int        HTTP server port (default 8080)
  --boot-dir string      Directory containing boot files (default "./boot")
  --data-dir string      Directory containing ISO images (default "./data")
  --server-addr string   Server IP address (auto-detected if not specified)
  --db-host string       PostgreSQL host (default "localhost")
  --db-port int          PostgreSQL port (default 5432)
  --db-user string       PostgreSQL user (default "bootimus")
  --db-password string   PostgreSQL password
  --db-name string       PostgreSQL database name (default "bootimus")
  --db-disable           Disable database (filesystem-only mode)
```

### migrate

Run database migrations:

```bash
./bootimus migrate
```

## Directory Structure

### Boot Directory (default: `./boot`)

Place iPXE bootloaders here:

```
boot/
├── ipxe.efi           # iPXE UEFI bootloader
└── undionly.kpxe      # iPXE for legacy BIOS PXE
```

Download iPXE bootloaders:

```bash
mkdir -p boot
wget http://boot.ipxe.org/ipxe.efi -O boot/ipxe.efi
wget http://boot.ipxe.org/undionly.kpxe -O boot/undionly.kpxe
```

### Data Directory (default: `./data`)

Place ISO images here:

```
data/
├── ubuntu-24.04-live-server-amd64.iso
├── debian-12.0.0-amd64-netinst.iso
└── your-custom-installer.iso
```

The server will:
1. Automatically scan for `.iso` files
2. Sync them with the database
3. Generate dynamic menus based on MAC address permissions

## MAC Address Access Control

### Database Mode (Default)

When database is enabled, Bootimus provides fine-grained access control:

#### Public Images

```sql
-- Make an image available to all clients
UPDATE images SET public = true WHERE name = 'ubuntu-24.04-live-server-amd64';
```

#### Client-Specific Access

```sql
-- Add a client
INSERT INTO clients (mac_address, name, enabled)
VALUES ('00:11:22:33:44:55', 'Lab Machine 1', true);

-- Grant access to specific images
INSERT INTO client_images (client_id, image_id)
SELECT
  (SELECT id FROM clients WHERE mac_address = '00:11:22:33:44:55'),
  id
FROM images WHERE name IN ('debian-12.0.0-amd64-netinst', 'custom-tool');
```

### Filesystem Mode

When database is disabled (`--db-disable`), all ISOs are available to all clients.

## How It Works

1. **Startup**: Bootimus scans the data directory for `.iso` files and syncs with database
2. **PXE Boot**:
   - Client sends DHCP request
   - DHCP server responds with boot server address and iPXE bootloader filename
   - Client downloads iPXE bootloader via TFTP
   - iPXE requests `/menu.ipxe?mac=<MAC>` via HTTP
3. **Menu Generation**:
   - Bootimus queries database for images accessible to the MAC address
   - Generates iPXE menu on-the-fly (no external files)
   - Menu includes only allowed images
4. **Boot**:
   - User selects an ISO from the ASCII menu
   - iPXE boots the ISO via HTTP using `sanboot`
   - Bootimus logs the boot attempt and updates statistics

## DHCP Configuration

### ISC DHCP Server (dhcpd.conf)

```
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    next-server 192.168.1.10;  # IP of bootimus server

    if exists user-class and option user-class = "iPXE" {
        filename "http://192.168.1.10:8080/menu.ipxe?mac=${net0/mac}";
    } elsif option arch = 00:07 or option arch = 00:09 {
        filename "ipxe.efi";
    } else {
        filename "undionly.kpxe";
    }
}
```

### Dnsmasq

```
dhcp-range=192.168.1.100,192.168.1.200,12h
dhcp-boot=tag:!ipxe,undionly.kpxe,192.168.1.10
dhcp-boot=tag:ipxe,http://192.168.1.10:8080/menu.ipxe

dhcp-match=set:efi-x86_64,option:client-arch,7
dhcp-boot=tag:efi-x86_64,tag:!ipxe,ipxe.efi,192.168.1.10
```

## Admin Interface

Bootimus includes a full-featured web-based admin panel for managing clients, images, and viewing boot logs.

### Accessing the Admin Panel

The admin interface runs on a separate port from the boot server for security:

```
http://your-server:8081/
```

**Security Features**:
- Runs on separate port (8081) - isolate from boot network
- HTTP Basic Authentication enabled by default
- Auto-generated password displayed in server logs on first run
- Password stored as SHA-256 hash in `.admin_password` file
- Works with or without database (`--db-disable` mode supported)

### Admin Features

#### Dashboard
- Real-time statistics (total clients, active clients, images, boot counts)
- Quick overview of system status

#### Client Management
- **Add clients**: Register new MAC addresses
- **Edit clients**: Update client information and enable/disable
- **Assign images**: Control which ISOs each client can access
- **View statistics**: See boot counts and last boot time per client
- **Delete clients**: Remove clients from the system

#### Image Management
- **Upload ISOs**: Drag-and-drop ISO file upload (up to 10GB)
- **Scan for ISOs**: Automatically detect new ISOs in the data directory
- **Enable/Disable images**: Control image availability
- **Public/Private**: Make images available to all clients or specific ones
- **View statistics**: Track image usage and boot counts
- **Delete images**: Remove images from database (optionally delete file)

#### Boot Logs
- View recent boot attempts (50 most recent by default)
- See MAC address, image name, success/failure status
- Track IP addresses and error messages
- Real-time log updates

### Screenshots

The admin interface features:
- Dark theme optimised for terminal users
- Responsive design for desktop and mobile
- Real-time statistics dashboard
- Easy drag-and-drop ISO uploads
- Filterable and sortable tables

## API Endpoints

### Boot Endpoints (Public)

- `GET /menu.ipxe?mac=<MAC>` - Dynamic iPXE menu for MAC address
- `GET /isos/<filename>?mac=<MAC>` - Serve ISO image
- `GET /api/isos?mac=<MAC>` - List available ISOs for MAC address
- `GET /health` - Health check endpoint
- `GET /<file>` - Serve boot files (iPXE bootloaders, etc.)

### Admin API Endpoints

All admin API endpoints require HTTP Basic Authentication (username: `admin`, password: auto-generated on first run).

**Stats**
- `GET /api/stats` - Get system statistics

**Clients**
- `GET /api/clients` - List all clients
- `GET /api/clients?id=<ID>` - Get single client
- `POST /api/clients` - Create new client
- `PUT /api/clients?id=<ID>` - Update client
- `DELETE /api/clients?id=<ID>` - Delete client
- `POST /api/clients/assign` - Assign images to client

**Images**
- `GET /api/images` - List all images
- `GET /api/images?id=<ID>` - Get single image
- `PUT /api/images?id=<ID>` - Update image
- `DELETE /api/images?id=<ID>` - Delete image
- `POST /api/images/upload` - Upload new ISO (multipart/form-data)
- `POST /api/scan` - Scan data directory for new ISOs

**Logs**
- `GET /api/logs?limit=<N>` - Get boot logs (default 100, max 1000)

#### API Examples

```bash
# Note: All admin API calls require HTTP Basic Auth
# Username: admin
# Password: (shown in server logs on first run, stored in .admin_password file)

# Get system stats
curl -u admin:your-password http://localhost:8081/api/stats

# List all clients
curl -u admin:your-password http://localhost:8081/api/clients

# Create a new client
curl -u admin:your-password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Machine 1",
    "enabled": true
  }'

# Assign images to a client
curl -u admin:your-password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": 1,
    "image_ids": [1, 2, 3]
  }'

# Upload an ISO
curl -u admin:your-password -X POST http://localhost:8081/api/images/upload \
  -F "file=@ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"

# Scan for new ISOs
curl -u admin:your-password -X POST http://localhost:8081/api/scan

# Get boot logs
curl -u admin:your-password http://localhost:8081/api/logs?limit=10
```

## Production Deployment

### Docker

```bash
# Build
docker build -t bootimus:latest .

# Run
docker run -d \
  --name bootimus \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -v $(pwd)/boot:/app/boot \
  -v $(pwd)/data:/app/data \
  -e BOOTIMUS_DB_HOST=postgres \
  -e BOOTIMUS_DB_PASSWORD=secretpassword \
  bootimus:latest
```

### Systemd Service

```ini
[Unit]
Description=Bootimus PXE/HTTP Boot Server
After=network.target postgresql.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/bootimus
ExecStart=/opt/bootimus/bootimus serve
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Security Considerations

- **Read-only TFTP**: TFTP server is read-only (no write operations)
- **Path Sanitisation**: All file paths are sanitised to prevent directory traversal
- **MAC Address Verification**: ISOs are served only to authorised MAC addresses (when database is enabled)
- **Database Security**: Use strong passwords and SSL for production PostgreSQL
- **Firewall**: Limit access to ports 69/UDP and 8080/TCP to your local network
- **Audit Logs**: All boot attempts are logged in the `boot_logs` table

## Troubleshooting

### Permission Denied on Port 69

```bash
# Run as root
sudo ./bootimus serve

# Or use a non-privileged port
./bootimus serve --tftp-port 6969

# Or use Docker with CAP_NET_BIND_SERVICE
docker run --cap-add NET_BIND_SERVICE ...
```

### Database Connection Failed

```bash
# Test connection
psql -h localhost -U bootimus -d bootimus

# Run migrations
./bootimus migrate

# Use filesystem-only mode
./bootimus serve --db-disable
```

### No ISOs in Menu

```bash
# Check data directory
ls -la data/

# Check database sync (if using database)
psql -h localhost -U bootimus -d bootimus -c "SELECT * FROM images;"

# Check client permissions
psql -h localhost -U bootimus -d bootimus -c "
  SELECT c.mac_address, i.name
  FROM clients c
  JOIN client_images ci ON c.id = ci.client_id
  JOIN images i ON ci.image_id = i.id;
"
```

## Development

```bash
# Install dependencies
go mod download

# Run locally
go run main.go serve --db-disable

# Build
go build -o bootimus

# Run tests (TODO)
go test ./...
```

## License

MIT License - feel free to use and modify as needed.
