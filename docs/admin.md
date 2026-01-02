#  Admin Console Guide

Complete guide for using the Bootimus admin interface and REST API.

##  Table of Contents

- [Accessing the Admin Panel](#accessing-the-admin-panel)
- [Dashboard](#dashboard)
- [Client Management](#client-management)
- [Image Management](#image-management)
- [Boot Logs](#boot-logs)
- [REST API](#rest-api)
- [Automation Examples](#automation-examples)
- [Security Best Practises](#security-best-practises)

## Accessing the Admin Panel

### Web Interface

```
http://your-server:8081/
```

**Requirements**:
- Admin interface runs on separate port (default 8081)
- Works with SQLite or PostgreSQL
- HTTP Basic Authentication required

### First-Time Login

On first startup, Bootimus generates a random admin password:

```
╔════════════════════════════════════════════════════════════════╗
║                    ADMIN PASSWORD GENERATED                    ║
╠════════════════════════════════════════════════════════════════╣
║  Password: AbCdEfGh1234567890-_XyZ123456                       ║
╠════════════════════════════════════════════════════════════════╣
║  Saved to: /path/to/.admin_password                            ║
║  This password will NOT be shown again!                        ║
║  Save it now or retrieve it from the file above.               ║
╚════════════════════════════════════════════════════════════════╝
```

**Login credentials**:
- **Username**: `admin`
- **Password**: Check server logs or `.admin_password` file

### Quick Start

1. Start Bootimus:
   ```bash
   docker-compose up -d
   # OR
   ./bootimus serve
   ```

2. Copy the admin password from server logs

3. Open browser to `http://localhost:8081/`

4. Log in with username `admin` and generated password

## Dashboard

The dashboard provides real-time statistics:

-  **Total clients** - All registered clients
-  **Active clients** - Enabled clients that can boot
-  **Total images** - All ISO images
-  **Enabled images** - Images available in boot menu
-  **Total boots** - Number of boot attempts

All statistics update in real-time via WebSocket/SSE.

## Client Management

### Add a Client

1. Click **"Add Client"** button
2. Enter MAC address (format: `00:11:22:33:44:55`)
3. Optionally add name and description
4. Check **"Enabled"** to allow booting
5. Click **"Create Client"**

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Machine 1",
    "description": "Test workstation",
    "enabled": true
  }'
```

### Edit a Client

1. Click **"Edit"** on any client row
2. Modify name, description, or enabled status
3. Select which ISOs this client can access (multi-select)
4. Click **"Update Client"**

**Via API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Name",
    "enabled": false
  }'
```

### Delete a Client

Click **"Delete"** on any client row and confirm deletion.

**Via API**:
```bash
curl -u admin:password -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

### Assign Images to Client

**Via Web Interface**:
1. Click **"Edit"** on client
2. Select images from multi-select dropdown
3. Click **"Update Client"**

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "image_filenames": ["ubuntu-24.04.iso", "debian-12.iso"]
  }'
```

## Image Management

### Upload an ISO

**Via Web Interface**:
1. Click **"Upload ISO"** button
2. Drag and drop ISO file or click to browse
3. Optionally add description
4. Check **"Public"** to make available to all clients
5. Click **"Upload"**

**Upload limit**: 10GB per file

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Download from URL

Download ISOs directly to server:

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

# Monitor progress
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Extract Kernel/Initrd

Extract boot files for faster booting and reduced bandwidth:

**Via Web Interface**:
1. Find image in **Images** tab
2. Click **"Extract"** button
3. Wait for extraction to complete

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'
```

**Benefits**:
-  Faster boot (download 100MB instead of 6GB)
-  Reduced bandwidth (critical for multiple clients)
-  Better compatibility (some ISOs don't support sanboot)

See [Image Management Guide](images.md) for detailed extraction information.

### Download Netboot Files

For Debian/Ubuntu installer ISOs that require netboot:

**Via Web Interface**:
1. Find image with **"Netboot Required"** badge
2. Click **"Download Netboot"** button
3. Wait for download and extraction

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

**What are netboot files?**
- Official minimal boot files from Debian/Ubuntu
- ~30-50MB download (instead of full ISO)
- Installer downloads packages from internet during installation
- Always get latest packages

See [Netboot Support](images.md#netboot-support) for details.

### Scan for ISOs

Scan data directory for manually added ISOs:

**Via Web Interface**:
1. Manually copy ISO files to `/data/isos/` directory
2. Click **"Scan for ISOs"** button
3. Bootimus detects and registers new ISOs

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### Enable/Disable Image

**Via Web Interface**:
- Click **"Enable"** or **"Disable"** button on any image
- Disabled images won't appear in boot menus

**Via API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

### Make Public/Private

**Via Web Interface**:
- Click **"Make Public"** to allow all clients to access
- Click **"Make Private"** to restrict to assigned clients only

**Via API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": true}'
```

### Delete Image

**Via Web Interface**:
- Click **"Delete"** on any image row
- Confirm deletion
- Image removed from database
- ISO file remains on disk (delete manually if needed)

**Via API**:
```bash
# Delete from database only
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso"

# Delete from database and filesystem
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso&delete_file=true"
```

## Boot Logs

View recent boot attempts with live streaming:

**Information shown**:
-  Timestamp
-  Client MAC address
-  Image name
-  IP address
- / Success/failure status
-  Error messages (if any)

**Auto-refresh**: Logs update in real-time via SSE (Server-Sent Events)

**Via API**:
```bash
# Get last 100 logs (default)
curl -u admin:password http://localhost:8081/api/logs

# Get last 10 logs
curl -u admin:password http://localhost:8081/api/logs?limit=10

# Get last 500 logs (max 1000)
curl -u admin:password http://localhost:8081/api/logs?limit=500
```

## REST API

All admin functions available via REST API for automation.

### Authentication

HTTP Basic Authentication required for all endpoints:
- **Username**: `admin`
- **Password**: Auto-generated on first run

```bash
curl -u admin:your-password http://localhost:8081/api/stats
```

### API Endpoints

#### Stats

```bash
GET /api/stats
```

**Response**:
```json
{
  "success": true,
  "data": {
    "total_clients": 10,
    "active_clients": 8,
    "total_images": 5,
    "enabled_images": 4,
    "total_boots": 127
  }
}
```

#### Clients

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/clients` | List all clients |
| `GET` | `/api/clients?mac=<MAC>` | Get client by MAC |
| `POST` | `/api/clients` | Create client |
| `PUT` | `/api/clients?mac=<MAC>` | Update client |
| `DELETE` | `/api/clients?mac=<MAC>` | Delete client |
| `POST` | `/api/clients/assign` | Assign images to client |

#### Images

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/images` | List all images |
| `GET` | `/api/images?filename=<name>` | Get image |
| `PUT` | `/api/images?filename=<name>` | Update image |
| `DELETE` | `/api/images?filename=<name>` | Delete image |
| `POST` | `/api/images/upload` | Upload ISO |
| `POST` | `/api/images/download` | Download ISO from URL |
| `POST` | `/api/images/extract` | Extract kernel/initrd |
| `POST` | `/api/images/netboot/download` | Download netboot files |
| `POST` | `/api/scan` | Scan for new ISOs |

#### Downloads

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/downloads` | List active downloads |
| `GET` | `/api/downloads/progress?filename=<name>` | Get download progress |

#### Logs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/logs?limit=<N>` | Get boot logs |
| `GET` | `/api/logs/stream` | SSE stream of real-time logs |

## Automation Examples

### Bulk Add Clients

```bash
#!/bin/bash
# bulk-add-clients.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

CLIENTS=(
  "00:11:22:33:44:01:Server1"
  "00:11:22:33:44:02:Server2"
  "00:11:22:33:44:03:Workstation1"
)

for entry in "${CLIENTS[@]}"; do
  IFS=':' read -r mac1 mac2 mac3 mac4 mac5 mac6 name <<< "$entry"
  mac="${mac1}:${mac2}:${mac3}:${mac4}:${mac5}:${mac6}"

  curl -u admin:$ADMIN_PASSWORD -X POST http://localhost:8081/api/clients \
    -H "Content-Type: application/json" \
    -d "{\"mac_address\":\"$mac\",\"name\":\"$name\",\"enabled\":true}"

  echo "Added $name ($mac)"
done
```

### Make All Images Public

```bash
#!/bin/bash
# make-all-public.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

images=$(curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/images | jq -r '.data[].filename')

for filename in $images; do
  curl -u admin:$ADMIN_PASSWORD -X PUT "http://localhost:8081/api/images?filename=$filename" \
    -H "Content-Type: application/json" \
    -d '{"public":true}'
  echo "Made $filename public"
done
```

### Monitor Boot Attempts

```bash
#!/bin/bash
# monitor-boots.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

while true; do
  clear
  echo "=== Recent Boot Attempts ==="
  curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/logs?limit=20 | \
    jq -r '.data[] | "\(.created_at) | \(.mac_address) | \(.image_name) | \(if .success then "" else "✗" end)"'
  sleep 5
done
```

### Export Statistics

```bash
#!/bin/bash
# export-stats.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

echo "Bootimus Usage Report - $(date)"
echo "================================"

stats=$(curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/stats | jq '.data')

echo "Total Clients: $(echo $stats | jq -r '.total_clients')"
echo "Active Clients: $(echo $stats | jq -r '.active_clients')"
echo "Total Images: $(echo $stats | jq -r '.total_images')"
echo "Total Boots: $(echo $stats | jq -r '.total_boots')"

echo -e "\nTop Clients by Boot Count:"
curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/clients | \
  jq -r '.data | sort_by(.boot_count) | reverse | .[:5] | .[] | "\(.boot_count) boots - \(.name // .mac_address)"'
```

## Security Best Practises

### Network Isolation

Keep admin port separate from boot network:

```bash
# Allow boot traffic (TFTP/HTTP) on one interface
# Allow admin traffic on different interface or localhost only
```

### Firewall Rules

```bash
# Allow admin access only from specific IP range
sudo ufw allow from 192.168.1.0/24 to any port 8081

# Or block admin port from external access entirely
sudo ufw deny 8081
```

### SSH Tunnel

Access admin interface securely via SSH tunnel:

```bash
# Create SSH tunnel
ssh -L 8081:localhost:8081 user@bootimus-server

# Access admin panel
open http://localhost:8081/
```

### VPN Access

- Place Bootimus admin port on VPN network only
- Require VPN connection for admin access
- Keep boot ports (69, 8080) on separate network segment

### Password Management

-  Store admin password securely (password manager)
-  Rotate password periodically by deleting `.admin_password` and restarting
- 🛡 Consider additional authentication layer (nginx with client certs)

## Troubleshooting

### Admin Interface Not Loading

```bash
# Check service is running
docker ps | grep bootimus

# Check logs
docker logs bootimus

# Verify port is accessible
curl -u admin:password http://localhost:8081/api/stats

# Check firewall
sudo ufw status | grep 8081
```

### Cannot Upload Large ISOs

```bash
# Check available disk space
df -h /opt/bootimus/data

# Upload limit is 10GB by default
# For larger ISOs, use download from URL or manual copy + scan
```

### Changes Not Reflecting

- Hard refresh browser (Ctrl+F5 or Cmd+Shift+R)
- Check browser console for errors (F12)
- Verify API responses with curl
- Check server logs for detailed errors

### API Returns Errors

```bash
# Check request format (JSON content-type for POST/PUT)
curl -v -u admin:password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","name":"Test"}'

# Verify resource exists for update/delete
curl -u admin:password http://localhost:8081/api/images | jq

# Check server logs
docker logs bootimus | tail -50
```

## Next Steps

-  Read [Image Management Guide](images.md) for ISO handling
-  See [Deployment Guide](deployment.md) for production setup
-  Configure [DHCP Server](dhcp.md) for PXE booting
-  Set up [Client Management](clients.md) for access control
