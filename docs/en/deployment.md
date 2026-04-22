#  Deployment Guide

Complete guide for deploying Bootimus in various environments with networking and storage configurations.

##  Table of Contents

- [Quick Start](#quick-start)
- [Docker Deployment](#docker-deployment)
- [Binary Deployment](#binary-deployment)
- [Networking Configuration](#networking-configuration)
- [Storage Configuration](#storage-configuration)
- [Database Options](#database-options)
- [Production Deployment](#production-deployment)

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

# Create data directory
mkdir -p data

# Run (SQLite mode - no database required)
./bootimus-amd64 serve

# Admin panel: http://localhost:8081
# Admin password shown in startup logs
```

## Docker Deployment

### Docker Compose with PostgreSQL

```bash
# Clone repository
git clone https://github.com/garybowers/bootimus
cd bootimus

# Start with PostgreSQL
docker-compose up -d

# View logs
docker-compose logs -f bootimus
```

The Docker Compose stack includes:
- **Bootimus server**: Main PXE/HTTP boot server
- **PostgreSQL**: Database for client/image management
- **Health checks**: Automatic service monitoring
- **Persistent storage**: Data volumes for ISOs and database

### Directory Structure

Bootimus automatically creates subdirectories:
- `/data/isos/` - ISO image files and extracted boot files (in subdirectories per ISO)
- `/data/bootloaders/` - Custom bootloader files (optional)
- `/data/bootimus.db` - SQLite database (if using SQLite mode)

## Networking Configuration

### Default Internal Bridge Network

By default, containers use an internal bridge network with port forwarding:

```yaml
networks:
  bootimus_net:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
          gateway: 172.20.0.1
```

- **Bootimus server**: `172.20.0.3`
- **PostgreSQL**: `172.20.0.2`
- **Access from host**: Via port forwarding (e.g., `localhost:8081`)

### Bridged Network with Static IP on LAN

For production PXE environments, you may want the container directly on your LAN with a static IP.

#### Step 1: Find Your Network Interface

```bash
ip addr show  # Linux
# Look for your primary interface (e.g., eth0, ens33, enp0s3)
```

#### Step 2: Edit docker-compose.yml

Uncomment the `host_bridge` network sections:

```yaml
services:
  bootimus:
    networks:
      # Comment out internal bridge
      # bootimus_net:
      #   ipv4_address: 172.20.0.3
      # Enable host bridge
      host_bridge:
        ipv4_address: 192.168.1.100  # Your desired static IP
    environment:
      BOOTIMUS_SERVER_ADDR: 192.168.1.100  # Set static server address

networks:
  # Uncomment and configure for your LAN
  host_bridge:
    driver: macvlan
    driver_opts:
      parent: eth0  # Your network interface
    ipam:
      config:
        - subnet: 192.168.1.0/24      # Your LAN subnet
          gateway: 192.168.1.1         # Your LAN gateway
          ip_range: 192.168.1.100/32   # Container static IP
```

#### Step 3: Configure Network Details

Update these values for your network:
- `parent`: Your host's network interface (e.g., `eth0`, `ens33`)
- `subnet`: Your LAN subnet (e.g., `192.168.1.0/24`)
- `gateway`: Your router's IP (e.g., `192.168.1.1`)
- `ip_range`: The static IP for Bootimus (e.g., `192.168.1.100/32`)
- `BOOTIMUS_SERVER_ADDR`: Same as the static IP

#### Step 4: Start Container

```bash
docker-compose down
docker-compose up -d
```

#### Step 5: Verify Connectivity

```bash
# From another machine on the LAN
curl http://192.168.1.100:8081

# Ping the container
ping 192.168.1.100
```

###  Important Notes for Macvlan Networking

- **Macvlan networking**: Container appears as a separate device on your LAN
- **Host cannot reach container**: The host machine cannot directly communicate with macvlan containers. Use a separate VM/container for admin access, or create a macvlan interface on the host.
- **DHCP conflicts**: Ensure the static IP is outside your DHCP range or reserved in your DHCP server
- **Firewall rules**: Container bypasses host firewall - configure container firewall separately if needed

### Accessing Macvlan Containers from Host

If you need to access the macvlan container from the host machine:

```bash
# Create a macvlan interface on the host
sudo ip link add macvlan0 link eth0 type macvlan mode bridge
sudo ip addr add 192.168.1.101/32 dev macvlan0
sudo ip link set macvlan0 up
sudo ip route add 192.168.1.100/32 dev macvlan0

# Now you can access the container from the host
curl http://192.168.1.100:8081
```

## Binary Deployment

### System Requirements

- **OS**: Linux (amd64, arm64, armv7)
- **Privileges**: Root required for port 69 (TFTP), or use non-privileged ports
- **Disk**: 10GB+ for ISO storage
- **Memory**: 512MB minimum, 2GB+ recommended

### Installation

```bash
# Download binary for your architecture
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64

# Make executable
chmod +x bootimus-amd64

# Move to system location
sudo mv bootimus-amd64 /usr/local/bin/bootimus

# Create data directory
sudo mkdir -p /opt/bootimus/data

# Create systemd service
sudo nano /etc/systemd/system/bootimus.service
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
ExecStart=/usr/local/bin/bootimus serve --data-dir /opt/bootimus/data
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable bootimus
sudo systemctl start bootimus

# Check status
sudo systemctl status bootimus

# View logs
sudo journalctl -u bootimus -f
```

## Storage Configuration

### Data Directory Structure

```
/opt/bootimus/data/
├── isos/                           # ISO files
│   ├── ubuntu-24.04.iso           # ISO file
│   ├── ubuntu-24.04/              # Extracted boot files
│   │   ├── vmlinuz
│   │   ├── initrd
│   │   └── casper/
│   │       └── filesystem.squashfs
│   └── debian-12.iso
├── bootloaders/                    # Custom bootloaders (optional)
├── bootimus.db                     # SQLite database (if using SQLite)
└── .admin_password                 # Generated admin password
```

### Disk Space Requirements

- **ISOs**: 1-10GB per ISO
- **Extracted files**: 100MB-3GB per ISO
- **Database**: < 100MB
- **Recommended**: 50GB+ for multiple ISOs

### Storage Best Practises

1. **Use SSD**: Faster boot times for clients
2. **Regular backups**: Backup database and ISOs
3. **Monitor disk space**: Set up alerts for low disk space
4. **Clean old ISOs**: Remove unused ISOs to free space

## Database Options

### SQLite Mode (Default)

SQLite is **enabled by default** - no configuration required!

```bash
# Run with SQLite (default)
./bootimus serve

# Database automatically created at: <data_dir>/bootimus.db
```

**Benefits**:
-  Zero configuration
-  Single file database
-  Perfect for single-server deployments
-  Easy backups (just copy the file)

**Limitations**:
-  Lower concurrency than PostgreSQL
-  Single-server only (no clustering)

### PostgreSQL Mode

For enterprise deployments with high concurrency:

#### Configuration File Method

```yaml
# bootimus.yaml
db:
  host: postgres.example.com
  port: 5432
  user: bootimus
  password: secretpassword
  name: bootimus
  sslmode: require
```

#### Environment Variable Method

```bash
export BOOTIMUS_DB_HOST=postgres.example.com
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secretpassword
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=require

./bootimus serve
```

**Benefits**:
-  High concurrency
-  Multi-server deployments
-  Advanced replication
-  Better performance at scale

**Requirements**:
- PostgreSQL 12+ server
- Network connectivity to database
- Additional infrastructure

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

### Configuration Options

Bootimus uses sensible defaults and requires minimal configuration.

#### Configuration Precedence

1. Command-line flags (highest priority)
2. Environment variables (prefixed with `BOOTIMUS_`)
3. Configuration file (`bootimus.yaml`)

#### Example Configuration File

```yaml
# bootimus.yaml
tftp_port: 69
http_port: 8080
admin_port: 8081
data_dir: ./data          # Base data directory
server_addr: ""           # Auto-detected if not specified

# Database configuration (optional)
# If no db.host is specified, SQLite is used automatically
db:
  host: localhost       # Leave empty for SQLite
  port: 5432
  user: bootimus
  password: bootimus
  name: bootimus
  sslmode: disable
```

#### Environment Variables

```bash
# Server settings
export BOOTIMUS_TFTP_PORT=69
export BOOTIMUS_HTTP_PORT=8080
export BOOTIMUS_ADMIN_PORT=8081
export BOOTIMUS_DATA_DIR=/var/lib/bootimus/data
export BOOTIMUS_SERVER_ADDR=192.168.1.100

# Database settings (PostgreSQL only)
export BOOTIMUS_DB_HOST=postgres      # Empty = SQLite
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secret
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=disable

./bootimus serve
```

## Troubleshooting

### Permission Denied on Port 69

```bash
# Run as root
sudo ./bootimus serve

# Or use Docker with NET_BIND_SERVICE capability
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

# Check PostgreSQL logs
docker logs bootimus-db
```

### Container Cannot Be Reached on LAN

```bash
# Verify macvlan configuration
docker network inspect bootimus_host_bridge

# Check IP address assignment
docker exec bootimus ip addr show

# Verify routing
ip route | grep 192.168.1.100

# Check firewall
sudo iptables -L -n | grep 192.168.1.100
```

### Out of Disk Space

```bash
# Check disk usage
df -h /opt/bootimus/data

# Find large files
du -sh /opt/bootimus/data/*

# Clean up old ISOs
rm /opt/bootimus/data/isos/old-image.iso

# Scan for new ISOs to update database
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Next Steps

-  Read [Image Management Guide](images.md) for ISO handling
-  See [Admin Console Guide](admin.md) for management
-  Configure [DHCP Server](dhcp.md) for PXE booting
-  Review [Security Guide](security.md) for hardening
