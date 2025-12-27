# Bootimus Admin Interface Guide

## Overview

The Bootimus admin interface provides a full-featured web UI and REST API for managing your PXE boot server. All management tasks can be performed through either the web interface or the API.

## Accessing the Admin Panel

### Web Interface

```
http://your-server:8081/
```

**Requirements:**
- Admin interface runs on separate port (default 8081, configurable via `--admin-port`)
- Works with or without database (`--db-disable` mode supported)
- HTTP Basic Authentication required (username: `admin`, password: auto-generated on first run)

### First Run

On first startup, Bootimus generates a random admin password and displays it in the server logs:

```
╔════════════════════════════════════════════════════════════════╗
║                    ADMIN PASSWORD GENERATED                    ║
╠════════════════════════════════════════════════════════════════╣
║  Password: AbCdEfGh1234567890-_XyZ123456    ║
╠════════════════════════════════════════════════════════════════╣
║  Saved to: /path/to/.admin_password                            ║
║  This password will NOT be shown again!                        ║
║  Save it now or retrieve it from the file above.               ║
╚════════════════════════════════════════════════════════════════╝
```

**Important**: Save this password - it's only displayed once!

### Quick Start

1. Start Bootimus:
   ```bash
   docker-compose up -d
   # OR
   ./bootimus serve
   ```

2. Copy the admin password from the server logs (displayed on first run)

3. Open your browser to `http://localhost:8081/`

4. Log in with username `admin` and the generated password

5. You'll see the admin dashboard with real-time statistics

## Features

### Dashboard

Real-time statistics showing:
- Total clients registered
- Active (enabled) clients
- Total ISO images
- Enabled images
- Total boot attempts

### Client Management

**Add a Client:**
1. Click "Add Client" button
2. Enter MAC address (format: `00:11:22:33:44:55`)
3. Optionally add name and description
4. Check "Enabled" to allow booting
5. Click "Create Client"

**Edit a Client:**
1. Click "Edit" on any client row
2. Modify name, description, or enabled status
3. Select which ISOs this client can access (multi-select)
4. Click "Update Client"

**Delete a Client:**
- Click "Delete" on any client row
- Confirm the deletion

### Image Management

**Upload an ISO:**
1. Click "Upload ISO" button
2. Drag and drop an ISO file or click to browse
3. Optionally add a description
4. Check "Public" to make it available to all clients
5. Click "Upload"

**Scan for ISOs:**
1. Add ISO files to your data directory manually
2. Click "Scan for ISOs" button
3. Bootimus will detect and register new ISOs

**Enable/Disable an Image:**
- Click "Enable" or "Disable" button on any image
- Disabled images won't appear in boot menus

**Make Public/Private:**
- Click "Make Public" to allow all clients to access
- Click "Make Private" to restrict to assigned clients only

**Delete an Image:**
- Click "Delete" on any image row
- Image is removed from database
- ISO file remains on disk (delete manually if needed)

### Boot Logs

View recent boot attempts showing:
- Timestamp
- Client MAC address
- Image name
- IP address
- Success/failure status
- Error messages (if any)

Logs auto-refresh every 30 seconds.

## REST API

All admin functions are available via REST API for automation and integration.

### Authentication

The API uses HTTP Basic Authentication:
- **Username**: `admin`
- **Password**: Auto-generated on first run (see server logs or `.admin_password` file)

All API requests must include authentication:
```bash
curl -u admin:your-password http://localhost:8081/api/stats
```

**Additional security recommendations**:
- Keep admin port (8081) isolated from boot network
- Use firewall rules to restrict access to admin port
- Consider SSH tunnel or VPN for remote access

### API Endpoints

#### Stats

```bash
GET /api/stats
```

Response:
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

**List all clients:**
```bash
curl -u admin:your-password http://localhost:8081/api/clients
```

**Get single client:**
```bash
curl -u admin:your-password http://localhost:8081/api/clients?id=1
```

**Create client:**
```bash
curl -u admin:your-password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Machine 1",
    "description": "Test machine in lab",
    "enabled": true
  }'
```

**Update client:**
```bash
curl -u admin:your-password -X PUT "http://localhost:8081/api/clients?id=1" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Name",
    "enabled": false
  }'
```

**Delete client:**
```bash
curl -u admin:your-password -X DELETE "http://localhost:8081/api/clients?id=1"
```

**Assign images to client:**
```bash
curl -u admin:your-password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": 1,
    "image_ids": [1, 2, 3]
  }'
```

#### Images

**List all images:**
```bash
curl -u admin:your-password http://localhost:8081/api/images
```

**Get single image:**
```bash
curl -u admin:your-password http://localhost:8081/api/images?id=1
```

**Update image:**
```bash
curl -u admin:your-password -X PUT "http://localhost:8081/api/images?id=1" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Ubuntu 24.04 LTS",
    "description": "Ubuntu Server",
    "enabled": true,
    "public": true
  }'
```

**Delete image:**
```bash
# Delete from database only
curl -u admin:your-password -X DELETE "http://localhost:8081/api/images?id=1"

# Delete from database and filesystem
curl -u admin:your-password -X DELETE "http://localhost:8081/api/images?id=1&delete_file=true"
```

**Upload ISO:**
```bash
curl -u admin:your-password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

**Scan for new ISOs:**
```bash
curl -u admin:your-password -X POST http://localhost:8081/api/scan
```

#### Boot Logs

**Get recent logs:**
```bash
# Get last 100 logs (default)
curl -u admin:your-password http://localhost:8081/api/logs

# Get last 10 logs
curl -u admin:your-password http://localhost:8081/api/logs?limit=10

# Get last 500 logs (max 1000)
curl -u admin:your-password http://localhost:8081/api/logs?limit=500
```

## Automation Examples

### Bulk Add Clients

```bash
#!/bin/bash
# bulk-add-clients.sh

# Set your admin password here or pass as environment variable
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

### Auto-assign Public Images

```bash
#!/bin/bash
# Make all images public

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

images=$(curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/images | jq -r '.data[].id')

for id in $images; do
  curl -u admin:$ADMIN_PASSWORD -X PUT "http://localhost:8081/api/images?id=$id" \
    -H "Content-Type: application/json" \
    -d '{"public":true}'
  echo "Made image $id public"
done
```

### Monitor Boot Attempts

```bash
#!/bin/bash
# monitor-boots.sh - Watch for new boot attempts

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

while true; do
  clear
  echo "=== Recent Boot Attempts ==="
  curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/logs?limit=20 | \
    jq -r '.data[] | "\(.created_at) | \(.mac_address) | \(.image_name) | \(if .success then "✓" else "✗" end)"'
  sleep 5
done
```

### Export Statistics

```bash
#!/bin/bash
# export-stats.sh - Generate usage report

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

## Security Best Practices

1. **Network Isolation:**
   ```bash
   # Keep admin port separate from boot network
   # Allow boot traffic (TFTP/HTTP) on one interface
   # Allow admin traffic on different interface or localhost only
   ```

2. **Firewall Rules:**
   ```bash
   # Allow admin access only from specific IP range
   sudo ufw allow from 192.168.1.0/24 to any port 8081

   # Or block admin port from external access entirely
   sudo ufw deny 8081
   ```

3. **SSH Tunnel:**
   ```bash
   # Access admin interface securely via SSH tunnel
   ssh -L 8081:localhost:8081 user@bootimus-server
   # Then access http://localhost:8081/
   ```

4. **VPN Access:**
   - Place Bootimus admin port on VPN network only
   - Require VPN connection for admin access
   - Keep boot ports (69, 8080) on separate network segment

5. **Password Management:**
   - Store admin password securely (password manager)
   - Rotate password periodically by deleting `.admin_password` file and restarting
   - Consider adding additional authentication layer (nginx with client certs)

## Troubleshooting

**Admin interface not loading:**
- Verify database is enabled (not using `--db-disable`)
- Check logs: `docker-compose logs -f bootimus`
- Ensure port 8080 is accessible

**Can't upload large ISOs:**
- Check available disk space in data directory
- Upload limit is 10GB by default

**Changes not reflecting:**
- Hard refresh browser (Ctrl+F5)
- Check browser console for errors
- Verify API responses with curl

**API returns errors:**
- Check request format (JSON content-type for POST/PUT)
- Verify IDs exist for update/delete operations
- Check server logs for detailed error messages

## Next Steps

- Set up authentication (nginx basic auth or OAuth proxy)
- Configure backups of PostgreSQL database
- Set up monitoring and alerting
- Document your network boot workflow
- Create automation scripts for common tasks
