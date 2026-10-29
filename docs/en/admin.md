#  Admin Console Guide

Complete guide for using the Bootimus admin interface and REST API.

For optional global and per-image boot passwords, see [boot menu passwords](boot-passwords.md).

##  Table of Contents

- [Accessing the Admin Panel](#accessing-the-admin-panel)
- [Dashboard](#dashboard)
- [Client Management](#client-management)
- [Image Management](#image-management)
- [Boot Logs](#boot-logs)
- [Housekeeping and Retention](#housekeeping-and-retention)
- [Audit Log](#audit-log)
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
- JWT token-based authentication (with optional LDAP/AD backend and OIDC single sign-on)

### First-Time Login

On first startup, Bootimus generates a random admin password:

```
╔════════════════════════════════════════════════════════════════╗
║                    ADMIN PASSWORD GENERATED                    ║
╠════════════════════════════════════════════════════════════════╣
║  Username: admin                                               ║
║  Password: AbCdEfGh1234567890-_XyZ123456                       ║
╠════════════════════════════════════════════════════════════════╣
║  This password will NOT be shown again!                        ║
║  Save it now or reset it using --reset-admin-password flag     ║
╚════════════════════════════════════════════════════════════════╝
```

Navigate to `http://your-server:8081` and you'll see a dedicated login page. Enter the admin credentials to access the panel.

**Login credentials**:
- **Username**: `admin`
- **Password**: Check server startup logs

If LDAP is configured, a dropdown will appear on the login page allowing you to choose between local and LDAP authentication. If OIDC is configured, a **Sign in with …** button appears for single sign-on. Users who aren't administrators get read-only access: they can browse the panel but can't change anything. See [Authentication Guide](authentication.md) for details.

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
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients \
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
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
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
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

### Assign Images to Client

**Via Web Interface**:
1. Click **"Edit"** on client
2. Select images from multi-select dropdown
3. Click **"Update Client"**

**Via API**:
```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/assign-images \
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
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/upload \
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
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "filename": "ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'
# NB The `filename` parameter is optional. When not given, its derived from the `url` parameter.

# Monitor progress
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Extract Kernel/Initrd

Extract boot files for faster booting and reduced bandwidth:

**Via Web Interface**:
1. Find image in **Images** tab
2. Click **"Extract"** button
3. Wait for extraction to complete

**Via API**:
```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/extract \
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
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/netboot/download \
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
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/scan
```

### Enable/Disable Image

**Via Web Interface**:
- Click **"Enable"** or **"Disable"** button on any image
- Disabled images won't appear in boot menus

**Via API**:
```bash
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

### Make Public/Private

**Via Web Interface**:
- Click **"Make Public"** to allow all clients to access
- Click **"Make Private"** to restrict to assigned clients only

**Via API**:
```bash
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
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
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso"

# Delete from database and filesystem
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso&delete_file=true"
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
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/logs

# Get last 10 logs
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/logs?limit=10

# Get last 500 logs (max 1000)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/logs?limit=500
```

## Housekeeping and Retention

Boot logs and hardware inventory submissions accumulate on every PXE boot, and the audit log grows with every admin change. Housekeeping runs once a day (and a few minutes after start-up) and prunes records older than the retention periods set under **Settings** → **Housekeeping**. A period of `0` keeps those records forever.

| Setting | Default | What it removes |
|---------|---------|-----------------|
| Boot log retention | 90 days | Boot attempt entries older than the period |
| Inventory history retention | 90 days | Older inventory submissions; the most recent inventory for every client is always kept |
| Remove discovered clients not seen for | 0 (off) | Auto-discovered clients whose last boot is older than the period. Static clients and cluster nodes are never touched |
| Audit log retention | 365 days | Admin action audit entries older than the period |
| Check the catalogue daily | on | Not a pruning task: flags installed images and tools that have a newer release in the catalogue (see the [Image Management guide](images.md#checksums-and-update-notifications)) |

The card also shows the result of the last run per task and how much space each top-level entry of the data directory uses, so it is easy to spot which ISO group or cache is taking the room.

**Via API**:
```bash
# Current settings, last report and data directory usage
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/housekeeping

# Keep boot logs for 30 days, inventory history for a year, never prune clients
curl -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"boot_log_days":30,"inventory_history_days":365,"discovered_client_days":0,"audit_log_days":365}' \
  http://localhost:8081/api/housekeeping

# Run housekeeping now
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/housekeeping/run

# Compare installed images and tools with the catalogue now
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/catalogue/check
```

## Audit Log

Every change made through the admin API is recorded: the user, the HTTP method and path, the query string, the status the server answered with, and the caller's IP address. Reads (`GET` requests) and the login endpoint itself are not recorded. Query values that look like passwords or tokens are stored as `[redacted]`.

The **Audit Log** tab is visible to administrators only. It can be filtered by user, by path and by a start time, and pages through older entries with **Load more**.

The same entries can be forwarded as `admin.action` webhook events by ticking that event under **Settings → Webhook**. The payload carries the user, method, path, query, status and a short summary in its `metadata` map.

**Via API**:
```bash
# Latest 100 entries
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/audit

# Everything alice did to clients since a point in time
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8081/api/audit?username=alice&path=/api/clients&since=2026-10-01T00:00:00Z&limit=500"
```

The response contains `entries` (newest first) and `total`, so `offset` can be used to page.

## REST API

All admin functions available via REST API for automation.

### OpenAPI Document

The server describes its own API as an OpenAPI 3.0 document at `GET /api/openapi.json` (no authentication needed). The same document is committed as [docs/openapi.json](../openapi.json) and can be regenerated with `bootimus openapi` or `make openapi`. Each operation carries two extensions: `x-bootimus-public` marks endpoints that need no token, and `x-bootimus-readonly` marks endpoints that read-only users may call. The **API Reference** tab in the admin UI is rendered from this document, and so is kept in step with the server automatically.

```bash
# Save the document from a running server
curl -s http://localhost:8081/api/openapi.json > bootimus-openapi.json

# Or print it from the binary without a server
bootimus openapi > bootimus-openapi.json
```

Import it into Postman, Insomnia, Swagger UI or an OpenAPI client generator to work with the API from your own tooling.

### Authentication

All endpoints except `/api/login` and `/api/auth-info` need a bearer token, sent as `Authorization: Bearer <token>`. That is either a 24-hour JWT from `POST /api/login` (see [Authentication](authentication.md#obtain-a-token)) or a long-lived [API token](authentication.md#api-tokens) created under **Users** → **API Tokens**, which is the better choice for scripts. Log in with:
- **Username**: `admin`
- **Password**: Auto-generated on first run

```bash
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.data.token')

curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/stats
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
| `POST` | `/api/assign-images` | Assign images to client |
| `GET` | `/api/clients/menu-preview?mac=<MAC>` | Render the iPXE script a client would receive (no side effects; omit `mac` for the anonymous menu) |
| `GET` | `/api/clients/timeline?mac=<MAC>&limit=50&before=<cursor>` | Merged activity timeline for one client (boots, installs, disk imaging jobs, proxyDHCP requests and, for administrators, audit entries), newest first. Pass `next_before` back unchanged as the URL-encoded `before` value to page through equal timestamps without skipping events. RFC 3339 timestamps remain accepted as exclusive time bounds. |

#### Images

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/images` | List all images |
| `GET` | `/api/images?filename=<name>` | Get image |
| `PUT` | `/api/images?filename=<name>` | Update image |
| `DELETE` | `/api/images?filename=<name>` | Delete image |
| `POST` | `/api/images/upload` | Upload ISO |
| `POST` | `/api/images/download` | Download ISO from URL, verifying against `sha256` or `checksum_url` when given |
| `POST` | `/api/images/verify?filename=<name>` | Hash an image against a checksum in the background |
| `GET` | `/api/images/verify?filename=<name>` | Verification progress and result |
| `POST` | `/api/catalogue/check` | Flag images and tools with newer catalogue releases |
| `POST` | `/api/images/extract` | Extract kernel/initrd |
| `POST` | `/api/images/netboot/download` | Download netboot files |
| `PUT` | `/api/images/winpe?filename=<name>` | Choose the WIM file and image index a Windows or WinPE image boots. Body: `{wim_path, wim_index}` |
| `POST` | `/api/scan` | Scan for new ISOs |

#### Disk Images

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/disk-images` | Readiness status and the list of captured disk images |
| `PUT` | `/api/disk-images?id=<id>` | Update an image's description |
| `DELETE` | `/api/disk-images?id=<id>` | Delete an image and its files |
| `POST` | `/api/disk-images/capture` | Queue a capture: `{mac_address, name, description, disk, wake}` |
| `POST` | `/api/disk-images/deploy` | Queue a deploy: `{mac_address` or `client_group_id, image_id, disk, wake}` |
| `GET` | `/api/disk-images/jobs?mac=<MAC>&limit=<n>` | Recent capture and deploy jobs |
| `DELETE` | `/api/disk-images/jobs?id=<id>` | Cancel a pending or running job |

See the [Disk Imaging guide](disk-imaging.md) for the workflow.

#### Deployment Profiles

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/deployment-profiles` | List profiles with the number of clients and groups using each |
| `POST` | `/api/deployment-profiles` | Create: `{name, description, image_filename, auto_install_file, san_target_id, boot_from_san, bootloader_set, disk_image_id, first_boot_action, show_public_images}` |
| `PUT` | `/api/deployment-profiles?id=<id>` | Update a profile |
| `DELETE` | `/api/deployment-profiles?id=<id>` | Delete a profile and clear it from clients and groups |
| `GET` | `/api/clients/effective?mac=<MAC>` | The settings in force for a client and the source of each value |

Clients and client groups take `profile_id` in their update bodies. See the [Deployment Profiles guide](deployment-profiles.md).

#### Enrolment Rules

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/enrolment-rules` | List rules in priority order with match counts |
| `POST` | `/api/enrolment-rules` | Create: `{name, priority, enabled, mac_prefix, manufacturer_contains, product_contains, serial_pattern, uuid_prefix, firmware, client_group_id, profile_id, make_static, name_pattern}` |
| `PUT` | `/api/enrolment-rules?id=<id>` | Update a rule |
| `DELETE` | `/api/enrolment-rules?id=<id>` | Delete a rule; clients it enrolled keep their settings |
| `POST` | `/api/enrolment-rules/test` | Dry run a rule body against existing clients |
| `POST` | `/api/enrolment-rules/apply` | Run all enabled rules over unmanaged discovered clients |

See [Auto-enrolment rules](deployment-profiles.md#auto-enrolment-rules).

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
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r '.data.token')

CLIENTS=(
  "00:11:22:33:44:01:Server1"
  "00:11:22:33:44:02:Server2"
  "00:11:22:33:44:03:Workstation1"
)

for entry in "${CLIENTS[@]}"; do
  IFS=':' read -r mac1 mac2 mac3 mac4 mac5 mac6 name <<< "$entry"
  mac="${mac1}:${mac2}:${mac3}:${mac4}:${mac5}:${mac6}"

  curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients \
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
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r '.data.token')

images=$(curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images | jq -r '.data[].filename')

for filename in $images; do
  curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/images?filename=$filename" \
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
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r '.data.token')

while true; do
  clear
  echo "=== Recent Boot Attempts ==="
  curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/logs?limit=20 | \
    jq -r '.data[] | "\(.created_at) | \(.mac_address) | \(.image_name) | \(if .success then "" else "✗" end)"'
  sleep 5
done
```

### Export Statistics

```bash
#!/bin/bash
# export-stats.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r '.data.token')

echo "Bootimus Usage Report - $(date)"
echo "================================"

stats=$(curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/stats | jq '.data')

echo "Total Clients: $(echo $stats | jq -r '.total_clients')"
echo "Active Clients: $(echo $stats | jq -r '.active_clients')"
echo "Total Images: $(echo $stats | jq -r '.total_images')"
echo "Total Boots: $(echo $stats | jq -r '.total_boots')"

echo -e "\nTop Clients by Boot Count:"
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients | \
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
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/stats

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
curl -v -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","name":"Test"}'

# Verify resource exists for update/delete
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images | jq

# Check server logs
docker logs bootimus | tail -50
```

## Next Steps

-  Read [Image Management Guide](images.md) for ISO handling
-  See [Deployment Guide](deployment.md) for production setup
-  Configure [DHCP Server](dhcp.md) for PXE booting
-  Set up [Client Management](clients.md) for access control
