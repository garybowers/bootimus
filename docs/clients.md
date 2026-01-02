#  Client Management Guide

Complete guide for managing network boot clients with MAC-based access control.

##  Table of Contents

- [Overview](#overview)
- [Adding Clients](#adding-clients)
- [Client Permissions](#client-permissions)
- [Public vs Private Images](#public-vs-private-images)
- [Client Statistics](#client-statistics)
- [Bulk Operations](#bulk-operations)
- [Troubleshooting](#troubleshooting)

## Overview

Bootimus uses MAC address-based access control to manage which clients can boot and which ISOs they can access. This provides granular control over your network boot environment.

### Key Concepts

- **Client**: A network boot device identified by MAC address
- **Enabled**: Client is allowed to boot (shows in boot menu)
- **Disabled**: Client cannot boot (blocked from accessing boot menu)
- **Public Images**: Available to all clients (enabled or not registered)
- **Private Images**: Only available to specifically assigned clients

### Database Modes

**SQLite Mode**:
- Clients stored in SQLite database
- Image assignments stored in `allowed_images` JSON field
- Perfect for single-server deployments

**PostgreSQL Mode**:
- Clients stored in PostgreSQL database
- Image assignments use many-to-many relationship table
- Better performance for large deployments

## Adding Clients

### Via Web Interface

1. Navigate to admin panel: `http://your-server:8081`
2. Click **"Clients"** tab
3. Click **"Add Client"** button
4. Fill in details:
   - **MAC Address**: `00:11:22:33:44:55` (required)
   - **Name**: Friendly name (e.g., "Lab Server 1")
   - **Description**: Additional details (optional)
   - **Enabled**: Check to allow booting
5. Click **"Create Client"**

### Via API

```bash
curl -u admin:password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Server 1",
    "description": "Dell PowerEdge R720",
    "enabled": true
  }'
```

### MAC Address Format

Bootimus accepts MAC addresses in these formats:
- `00:11:22:33:44:55` (colon-separated, preferred)
- `00-11-22-33-44-55` (dash-separated, auto-converted)
- `001122334455` (no separators, auto-converted)

All formats are normalized to colon-separated lowercase.

## Client Permissions

### Assign Images to Client

**Via Web Interface**:
1. Click **"Edit"** on client row
2. Select images from multi-select dropdown
3. Click **"Update Client"**

**Via API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "image_filenames": [
      "ubuntu-24.04-live-server-amd64.iso",
      "debian-13.2.0-amd64-netinst.iso",
      "archlinux-2025.12.01-x86_64.iso"
    ]
  }'
```

### View Client Permissions

**Via Web Interface**:
- Client's assigned images shown in edit modal

**Via API**:
```bash
# Get client details including assigned images
curl -u admin:password "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | jq
```

**Response**:
```json
{
  "success": true,
  "data": {
    "id": 1,
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Server 1",
    "description": "Dell PowerEdge R720",
    "enabled": true,
    "boot_count": 15,
    "last_boot": "2025-01-02T10:30:00Z",
    "allowed_images": [
      "ubuntu-24.04-live-server-amd64.iso",
      "debian-13.2.0-amd64-netinst.iso"
    ]
  }
}
```

## Public vs Private Images

### Public Images

Public images are available to **all clients**, even unregistered ones.

**Use cases**:
-  Rescue/recovery ISOs
-  Network diagnostic tools
-  Common deployment images
-  Open lab environments

**Make image public**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": true}'
```

### Private Images

Private images are **only available to assigned clients**.

**Use cases**:
-  Sensitive or licensed images
-  Client-specific deployments
-  Restricted environments
-  Beta/test images

**Make image private**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=windows.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": false}'
```

### Access Control Matrix

| Client State | Public Images | Private Images |
|--------------|---------------|----------------|
| **Enabled + Assigned** |  Yes |  Yes (assigned only) |
| **Enabled + Not Assigned** |  Yes |  No |
| **Disabled** |  Yes |  No |
| **Not Registered** |  Yes |  No |

## Client Statistics

Bootimus tracks boot statistics for each client:

- **Boot Count**: Total number of boot attempts
- **Last Boot**: Timestamp of most recent boot
- **Success Rate**: Percentage of successful boots

### View Statistics

**Via Web Interface**:
- Statistics shown in clients table

**Via API**:
```bash
# Get all clients with statistics
curl -u admin:password http://localhost:8081/api/clients | jq '.data[] | {name, boot_count, last_boot}'

# Get top clients by boot count
curl -u admin:password http://localhost:8081/api/clients | \
  jq '.data | sort_by(.boot_count) | reverse | .[0:10] | .[] | {name, boot_count}'
```

### Boot Logs

View detailed boot logs per client:

```bash
# Filter boot logs by MAC address
curl -u admin:password http://localhost:8081/api/logs | \
  jq '.data[] | select(.mac_address=="00:11:22:33:44:55")'
```

## Bulk Operations

### Bulk Add Clients

```bash
#!/bin/bash
# bulk-add-clients.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

# Format: MAC:NAME:DESCRIPTION
CLIENTS=(
  "00:11:22:33:44:01:Server-01:Production Web Server"
  "00:11:22:33:44:02:Server-02:Production Database Server"
  "00:11:22:33:44:03:Server-03:Production Cache Server"
  "00:11:22:33:44:10:Workstation-01:Developer Laptop"
  "00:11:22:33:44:11:Workstation-02:QA Testing Machine"
)

for entry in "${CLIENTS[@]}"; do
  IFS=':' read -r mac name description <<< "$entry"

  curl -u admin:$ADMIN_PASSWORD -X POST http://localhost:8081/api/clients \
    -H "Content-Type: application/json" \
    -d "{
      \"mac_address\":\"$mac\",
      \"name\":\"$name\",
      \"description\":\"$description\",
      \"enabled\":true
    }"

  echo "Added $name ($mac)"
  sleep 0.5
done
```

### Bulk Assign Images

```bash
#!/bin/bash
# bulk-assign-images.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

# Assign Ubuntu and Debian to all servers
SERVER_MACS=(
  "00:11:22:33:44:01"
  "00:11:22:33:44:02"
  "00:11:22:33:44:03"
)

IMAGES='["ubuntu-24.04-live-server-amd64.iso","debian-13.2.0-amd64-netinst.iso"]'

for mac in "${SERVER_MACS[@]}"; do
  curl -u admin:$ADMIN_PASSWORD -X POST http://localhost:8081/api/clients/assign \
    -H "Content-Type: application/json" \
    -d "{\"mac_address\":\"$mac\",\"image_filenames\":$IMAGES}"

  echo "Assigned images to $mac"
done
```

### Bulk Enable/Disable

```bash
#!/bin/bash
# bulk-enable.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

# Get all clients and enable them
macs=$(curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/clients | \
  jq -r '.data[].mac_address')

for mac in $macs; do
  curl -u admin:$ADMIN_PASSWORD -X PUT "http://localhost:8081/api/clients?mac=$mac" \
    -H "Content-Type: application/json" \
    -d '{"enabled":true}'
  echo "Enabled $mac"
done
```

### Export Client List

```bash
#!/bin/bash
# export-clients.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

echo "MAC Address,Name,Description,Enabled,Boot Count,Last Boot"

curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/clients | \
  jq -r '.data[] | [.mac_address, .name, .description, .enabled, .boot_count, .last_boot] | @csv'
```

### Import Clients from CSV

```bash
#!/bin/bash
# import-clients.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"
CSV_FILE="clients.csv"

# Skip header line and process CSV
tail -n +2 "$CSV_FILE" | while IFS=',' read -r mac name description enabled; do
  # Remove quotes from CSV values
  mac=$(echo $mac | tr -d '"')
  name=$(echo $name | tr -d '"')
  description=$(echo $description | tr -d '"')
  enabled=$(echo $enabled | tr -d '"')

  curl -u admin:$ADMIN_PASSWORD -X POST http://localhost:8081/api/clients \
    -H "Content-Type: application/json" \
    -d "{
      \"mac_address\":\"$mac\",
      \"name\":\"$name\",
      \"description\":\"$description\",
      \"enabled\":$enabled
    }"

  echo "Imported $name ($mac)"
done
```

## Troubleshooting

### Client Not Seeing Boot Menu

**Symptoms**: Client boots but shows empty menu or "No boot images available"

**Possible causes**:
1. Client is disabled
2. No public images available
3. No images assigned to client
4. All images are disabled

**Solution**:
```bash
# Check client status
curl -u admin:password "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | jq

# Enable client
curl -u admin:password -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{"enabled":true}'

# Check available images
curl -u admin:password http://localhost:8081/api/images | jq '.data[] | {filename, enabled, public}'

# Make images public
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public":true,"enabled":true}'
```

### MAC Address Not Detected

**Symptoms**: Boot logs show "unknown" MAC address

**Possible causes**:
1. iPXE cannot detect MAC address from network interface
2. Client using multiple network interfaces

**Solution**:
```bash
# Check boot logs for actual IP address
curl -u admin:password http://localhost:8081/api/logs | jq '.data[] | {mac_address, ip_address}'

# Register client by IP if MAC is unknown
# (Note: Less reliable, IP may change)
```

### Assigned Images Not Showing

**Symptoms**: Client can only see public images, not assigned ones

**Possible causes**:
1. Client not enabled
2. Images not enabled
3. Wrong MAC address format
4. Database sync issue

**Solution**:
```bash
# Verify client exists and is enabled
curl -u admin:password "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | jq

# Verify image assignments
curl -u admin:password "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | \
  jq '.data.allowed_images'

# Re-assign images
curl -u admin:password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address":"00:11:22:33:44:55",
    "image_filenames":["ubuntu.iso","debian.iso"]
  }'

# Check database directly (SQLite)
sqlite3 data/bootimus.db "SELECT * FROM clients WHERE mac_address='00:11:22:33:44:55';"
```

### Duplicate Client Error

**Symptoms**: "Client already exists" or UNIQUE constraint error

**Cause**: MAC address already registered

**Solution**:
```bash
# Find existing client
curl -u admin:password http://localhost:8081/api/clients | \
  jq '.data[] | select(.mac_address=="00:11:22:33:44:55")'

# Update existing client instead
curl -u admin:password -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{"name":"Updated Name","enabled":true}'

# Or delete and re-create
curl -u admin:password -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

## Next Steps

-  Configure [Image Management](images.md) to add ISOs
-  Use [Admin Console](admin.md) for management
-  Set up [DHCP Configuration](dhcp.md) for network booting
-  View [Boot Logs](admin.md#boot-logs) for monitoring
