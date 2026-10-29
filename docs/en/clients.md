#  Client Management Guide

Complete guide for managing network boot clients with MAC-based access control.

##  Table of Contents

- [Overview](#overview)
- [Client Detail Page](#client-detail-page)
- [Adding Clients](#adding-clients)
- [Client Permissions](#client-permissions)
- [Public vs Private Images](#public-vs-private-images)
- [Client Statistics](#client-statistics)
- [Bulk Operations](#bulk-operations)
- [Next Boot Action](#next-boot-action)
- [Previewing a Client's Boot Menu](#previewing-a-clients-boot-menu)
- [Booting from iSCSI or AoE](#booting-from-iscsi-or-aoe)
- [Deployment Profiles](#deployment-profiles)
- [Wake-on-LAN](#wake-on-lan)
- [BMC Power Control](#bmc-power-control)
- [Remote Console](#remote-console)
- [Hardware Inventory](#hardware-inventory)
- [Troubleshooting](#troubleshooting)

## Overview

Bootimus uses MAC address-based access control to manage which clients can boot and which ISOs they can access. This provides granular control over your network boot environment.

### Key Concepts

- **Client**: A network boot device identified by MAC address
- **Static client**: Manually created or promoted from discovered - a permanent registration
- **Discovered client**: Automatically created when an unknown device PXE boots (like a DHCP lease)
- **Enabled**: Client is allowed to boot (shows in boot menu)
- **Disabled**: Client cannot boot (blocked from accessing boot menu)
- **Assigned Images**: When a client has images assigned, it sees **only those images** (not the full public list)
- **Show Public Images**: When enabled alongside assigned images, client sees both assigned and public images
- **Next Boot Action**: A one-time boot image override that auto-clears after use
- **Bootloader Set**: An optional initial bootloader override for this client
- **Auto-install variables**: Named values available to auto-install files as `{{VAR.<name>}}`; group variables are defaults, client variables override them (see [Auto-Install](auto-install.md#variables))

### Client Auto-Discovery

When an unknown device PXE boots, Bootimus automatically creates a **discovered** client record with:
- MAC address from the PXE request
- Hardware inventory (CPU, memory, manufacturer, serial number, NIC)
- Enabled with public images visible by default

Discovered clients appear in the clients table with a "Discovered" badge. You can promote them to static clients using the **"Make Static"** button, which registers them as permanent entries. If a previously deleted client PXE boots again, it is automatically restored.

### Database Modes

**SQLite Mode**:
- Clients stored in SQLite database
- Image assignments stored in `allowed_images` JSON field
- Perfect for single-server deployments

**PostgreSQL Mode**:
- Clients stored in PostgreSQL database
- Image assignments use many-to-many relationship table
- Better performance for large deployments

## Client Detail Page

Clicking a row in the client list opens that client's detail page rather than the edit form. The page is linkable (`#clients/<mac>` in the address bar) and the browser's back button returns to the list.

The page shows:

- **Header**: name, MAC address, static or discovered, enabled state, enrolment state, the last unattended install state, any pending next-boot action and the client group.
- **Actions**: Edit (opens the usual edit form), Make Static, Wake, Next Boot, BMC power control with the live power state, Open Console, Preview Menu, Capture Disk, Deploy Image and Delete. Read-only users see the page without the buttons that change anything.
- **Overview**: last boot, boot count, bootloader set, allowed images, SAN target, auto-install file, BMC settings and timestamps.
- **Provisioning**: client group, pending next boot, install state and Kubernetes cluster membership, followed by the deployment profile in force (the client's own or the one inherited from its group, linked to the Deployment Profiles tab), the enrolment rule that enrolled the client with the time it happened (or "manually added" for static clients), and an effective settings table listing the image, auto-install file, SAN target, bootloader set, public image visibility, first-boot action and disk image together with where each value comes from: the client itself, its profile, the group's profile, the group or the default. A discovered client that no rule or person has placed yet gets a **Re-evaluate rules** button for administrators, which runs the enabled enrolment rules again.
- **Activity**: counts of boots, unattended installs, disk imaging jobs and inventory submissions.
- **Hardware**: the latest inventory with a history view that highlights values that changed between submissions.
- **Installs**: the unattended install records with their state and messages.
- **Timeline**: every recorded event for the client merged newest first: boot attempts, install progress, disk imaging jobs, proxyDHCP requests still held by the PXE Debug recorder and, for administrators, audit log entries about this client. **Load more** pages further back.

The timeline is also available from the API:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8081/api/clients/timeline?mac=aa:bb:cc:dd:ee:ff&limit=50"

# Next page using the cursor from the previous response
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8081/api/clients/timeline?mac=aa:bb:cc:dd:ee:ff&before=2026-10-25T09:00:00Z"
```

Each item has `time`, `kind` (`boot`, `install`, `disk`, `dhcp` or `audit`), `title`, `detail` and `status` (`ok`, `failed`, `pending` or `info`).

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
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients \
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

All formats are normalised to colon-separated lowercase.

## Client Permissions

### Select a Bootloader Set

The bootloader set used for the initial PXE download is resolved in this order:

1. Client bootloader set
2. Client-group bootloader set
3. Globally active bootloader set

Select a set in the client's edit dialog, or leave **Default (global setting)**
to inherit from its group and then the global setting.

Per-client and client-group selection requires Bootimus's built-in proxyDHCP
server, because it uses the PXE request's MAC address to advertise a
set-qualified boot filename. External DHCP servers continue to advertise their
statically configured filename and therefore use the globally active set.

### Assign Images to Client

**Via Web Interface**:
1. Click **"Edit"** on client row
2. Select images from multi-select dropdown
3. Click **"Update Client"**

**Via API**:
```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/assign-images \
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
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | jq
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
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
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
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/images?filename=windows.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": false}'
```

### Access Control Matrix

| Client State | What They See |
|--------------|---------------|
| **Enabled + Assigned** | Only their assigned images |
| **Enabled + No Assignments** | All public images |
| **Disabled** | All public images |
| **Not Registered** | All public images |

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
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients | jq '.data[] | {name, boot_count, last_boot}'

# Get top clients by boot count
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients | \
  jq '.data | sort_by(.boot_count) | reverse | .[0:10] | .[] | {name, boot_count}'
```

### Boot Logs

View detailed boot logs per client:

```bash
# Filter boot logs by MAC address
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/logs | \
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

  curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients \
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
  curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/assign-images \
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
macs=$(curl -H "Authorization: Bearer $TOKEN" -s http://localhost:8081/api/clients | \
  jq -r '.data[].mac_address')

for mac in $macs; do
  curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/clients?mac=$mac" \
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

curl -H "Authorization: Bearer $TOKEN" -s http://localhost:8081/api/clients | \
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

  curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients \
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

## Next Boot Action

Set a one-time boot image for a client. On the next PXE boot, the selected image will be pre-selected as the default menu item with a timeout. The action auto-clears after use - subsequent boots return to normal.

### Via Web Interface

1. Click **"Next Boot"** on a client row
2. Select an image from the dropdown
3. Click **"Set Next Boot"** to just set the image, or **"Set & Wake"** to also send a Wake-on-LAN packet

### Via API

```bash
# Set next boot image
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients/next-boot \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","image_filename":"ubuntu-24.04.iso"}'

# Clear next boot action
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients/next-boot \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","image_filename":""}'
```

### Behaviour

- The boot menu is displayed as normal but with the next boot image pre-selected as default
- If the global menu timeout is disabled (set to 0), a 10-second timeout is applied as an override
- If the client doesn't boot before the action is consumed, the next boot clears on the first PXE request
- Empty groups are hidden from the menu when a client has assigned images

## Previewing a Client's Boot Menu

Render the exact iPXE script a client will receive without waiting for it to PXE boot. The preview has no side effects: nothing is written to the boot log, the client's next-boot action is not consumed, and cluster enrolment state is left alone.

### Via Web Interface

- Click **"Preview Menu"** on a client row, or
- Open **Boot Menu** → **Preview as Client**, enter a MAC address (or leave `unknown` for an unregistered client) and click **"Preview Menu"**

The modal shows whether the MAC is a registered client, whether it would receive the normal menu or a cluster install boot, any pending next-boot image, and the full script with **Copy** and **Download** buttons.

### Via API

```bash
# Preview for a specific client
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8081/api/clients/menu-preview?mac=00:11:22:33:44:55"

# Preview the anonymous menu an unregistered client gets
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients/menu-preview
```

The response data contains `mac`, `known_client`, `client_name`, `next_boot_image`, `kind` (`menu` or `install`) and `script`. Read-only users can call this endpoint.

## Booting from iSCSI or AoE

Clients can boot an operating system that lives on a SAN disk rather than a local one. Bootimus hands the disk to the firmware with iPXE's `sanboot` command, so anything that can boot from a SAN with iPXE works here: iSCSI and ATA-over-Ethernet targets, and raw disk images served over HTTP. Bootimus does not host the disks; point a target at a storage server you already run.

### Defining a target

**Clients** → **SAN Targets** → **Add SAN Target**:

| Field | Meaning |
|-------|---------|
| Name | Shown in the boot menu as "Boot from SAN: *name*" |
| SAN URI | The iPXE SAN URI, for example `iscsi:10.0.0.5::::iqn.2024-01.lan:disk0`, `aoe:e0.0` or `http://10.0.0.5/disk.img` |
| Initiator IQN | Optional. `{{MAC}}` is replaced with the client's MAC address without colons, so one target can serve many clients with distinct initiator names |
| CHAP | Optional username and password, plus reverse (mutual) CHAP credentials if the target demands them |

The iSCSI URI format is `iscsi:<server>:<protocol>:<port>:<lun>:<iqn>`. Leave the protocol, port and LUN empty for the defaults, as in the example above.

### Assigning a target

Open a client (or a client group) and pick the target under **SAN Target**. A client without its own target inherits the group's. The client's menu gains a "Boot from SAN" entry; tick **Boot from SAN by default** to make it the entry the menu falls through to when the timeout fires, so a rebooted machine comes straight back up from the SAN. A pending next-boot image still takes priority, and the usual images remain selectable.

For a one-off SAN boot choose the target in **Next Boot**; it is listed after the images and clears after one boot like any other next-boot action.

### Example targets

A Linux target with `tgt`:

```bash
sudo tgtadm --lld iscsi --op new --mode target --tid 1 -T iqn.2024-01.lan:disk0
sudo tgtadm --lld iscsi --op new --mode logicalunit --tid 1 --lun 1 -b /srv/iscsi/disk0.img
sudo tgtadm --lld iscsi --op bind --mode target --tid 1 -I ALL
```

Target URI: `iscsi:10.0.0.5::::iqn.2024-01.lan:disk0`

With LIO (`targetcli`) create a fileio or block backstore, an iqn, a LUN and a portal, and either disable authentication for the TPG or set `userid`/`password` (and `mutual_userid`/`mutual_password`) to match the CHAP fields here.

**Windows** installs and boots from iSCSI through iPXE's SAN hook: boot the Windows installer from the menu while the client has a target assigned, and the installer sees the iSCSI disk as a local drive. After installation set **Boot from SAN by default** so the machine returns to it on every boot. Windows needs the network driver for the NIC it booted from; most modern Intel and Realtek NICs are covered by the installer.

### Caveats

- CHAP credentials are placed in the iPXE script, which is fetched over plain HTTP by the booting client. Treat the boot network as trusted, as you already do for auto-install answer files.
- iPXE's `sanboot` requires a bootloader on the SAN disk; it does not install anything.
- Read-only users see the targets but not the CHAP passwords.

## Deployment Profiles

When several clients should get the same image, auto-install file, SAN target or bootloader set, put those settings in a deployment profile and assign the profile to the clients or to their group instead of filling in each client. A client's own settings always win over its profile, and a client's profile wins over its group's. The client editor shows the **Effective settings** that result. See the [Deployment Profiles guide](deployment-profiles.md) for precedence, first-boot actions and examples, and its [auto-enrolment rules](deployment-profiles.md#auto-enrolment-rules) section for placing newly discovered machines into a group and profile automatically.

## Wake-on-LAN

Send a WOL magic packet to wake a client remotely. Combine with **Next Boot** to wake a machine and have it boot into a specific image.

### Waking a group

**Wake All (WOL)** wakes enabled members using the group's saved broadcast address, or the server default if the field is blank. Individual **Wake** buttons use the server default, so check the group override if individual wakes work but group wakes do not.

Bootimus sends the first packet before reporting that the group wake has started. If that send fails, the error appears in the UI and the remaining members are not attempted. Otherwise, the remaining packets are sent in the background with the configured stagger delay. Check the server's `ClientGroup wake` log entries for later failures. A successful send does not confirm that a machine has woken.

### Via API

```bash
# Wake a client
curl -H "Authorization: Bearer $TOKEN" -X POST "http://localhost:8081/api/clients/wake?mac=00:11:22:33:44:55"

# Wake with custom broadcast address
curl -H "Authorization: Bearer $TOKEN" -X POST "http://localhost:8081/api/clients/wake?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{"broadcast_addr":"192.168.1.255"}'
```

## BMC Power Control

Clients with a baseboard management controller can be powered on, off and restarted from the client edit dialog, from the client group bulk actions and from scheduled tasks. Two protocols are supported:

| Protocol | Typical hardware | Default port | Notes |
|---|---|---|---|
| **Redfish** (default) | Servers with iDRAC, iLO, XClarity, OpenBMC or any other DMTF Redfish BMC | 443 | HTTPS with basic authentication |
| **Intel AMT** | Business desktops and mini PCs with Intel vPro, such as the Dell OptiPlex Micro range | 16992 (16993 with TLS) | HTTP digest authentication; AMT must be provisioned in the firmware setup (MEBx) first |

The BMC host is always set per client, because every BMC has its own address. The protocol, port, username, password and *Skip TLS verify* flag can be set per client or inherited from the client group, so a fleet of identical machines only needs the credentials entered once.

For Intel AMT the port chooses the transport: leave it blank or set 16992 for plain HTTP, or set 16993 for TLS. Tick *Skip TLS verify* when the machine uses its self-signed certificate.

### Power actions

The same action names work for both protocols:

| Action | Redfish | Intel AMT |
|---|---|---|
| `On` | On | Power On |
| `ForceOff` | ForceOff | Power Off (Soft) |
| `ForceRestart` | ForceRestart | Master Bus Reset |
| `GracefulShutdown` | GracefulShutdown | Power Off (Soft, Graceful) |
| `GracefulRestart` | GracefulRestart | Master Bus Reset (Graceful) |

On Intel AMT the graceful actions are only offered while the machine is on and the [Intel Local Manageability Service](https://github.com/intel/lms) is running in the operating system. Bootimus checks the states the machine currently advertises and reports a clear error rather than falling back to a hard power-off.

### Via API

```bash
# Set the protocol and credentials on a client
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/clients/update?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{"bmc_protocol":"amt","ipmi_host":"192.168.1.50","ipmi_username":"admin","ipmi_password":"secret"}'

# Query the power state
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/power/status?mac=00:11:22:33:44:55"

# Send a power action
curl -H "Authorization: Bearer $TOKEN" -X POST "http://localhost:8081/api/clients/power?mac=00:11:22:33:44:55&action=On"
```

`bmc_protocol` accepts `redfish` or `amt`. Leave it empty on a client to inherit the group's protocol; an empty protocol everywhere means Redfish.

## Remote Console

Clients with a BMC host configured get an **Open Console** button in the client edit dialog and a **Console** button in the client list. It opens an interactive serial console in the browser, streamed through Bootimus over a WebSocket, so a machine can be watched through POST, firmware setup and the installer without a monitor attached. The console is for administrators only, one session per client at a time, and closes after 30 minutes without activity.

| Protocol | Transport | Requirements |
|---|---|---|
| **Intel AMT** | Serial-over-LAN through the AMT redirection service, implemented natively by Bootimus on port 16994 (16995 when the client uses the TLS management port 16993) | SOL and the redirection listener enabled in MEBx under *Remote Control*, or in the AMT web UI under *Remote Control* → *Redirection*; the same user and password as power control |
| **Redfish** | IPMI Serial-over-LAN by running `ipmitool sol activate` on the Bootimus host | `ipmitool` on `PATH` (the Docker image includes it), UDP port 623 reachable on the BMC, and SOL enabled for the BMC user. The Redfish port is not used for the console |

The serial console only shows what the firmware or operating system writes to the serial port. On most servers that means enabling *Console Redirection* in the BIOS or UEFI setup, and on Linux adding `console=ttyS0,115200` (or `ttyS1`, depending on the board) to the kernel command line. Intel AMT machines redirect COM1 at 115200 baud once SOL is enabled. If the screen stays blank, press Enter; if nothing appears during boot, console redirection is not switched on in the firmware.

Keystrokes are sent as typed, so the usual terminal shortcuts work. Closing the dialog or clicking **Disconnect** ends the session and, for IPMI, deactivates the SOL payload on the BMC so the next connection starts cleanly.

### Via API

The endpoint is a WebSocket at `/api/clients/console?mac=...` on the admin port. Browsers cannot send an `Authorization` header on a WebSocket, so the JWT is passed either as the second `Sec-WebSocket-Protocol` entry after `bearer`, or as a `token` query parameter. Binary frames carry console output and keystrokes; text frames from Bootimus are JSON status messages such as `{"type":"connected","message":"..."}`.

```bash
# websocat example: interactive console for a client
websocat --binary "ws://localhost:8081/api/clients/console?mac=00:11:22:33:44:55&token=$TOKEN"
```

## Hardware Inventory

Bootimus collects hardware information from PXE clients during boot, including:
- CPU, memory, platform, and architecture
- Manufacturer, product name, and serial number
- UUID and NIC chip info
- IP address

View inventory from the **Edit & Assign Images** modal for any client, or use the API:

```bash
# Latest inventory
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/inventory?mac=00:11:22:33:44:55"

# Inventory history
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/inventory/history?mac=00:11:22:33:44:55&limit=10"
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
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | jq

# Enable client
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{"enabled":true}'

# Check available images
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images | jq '.data[] | {filename, enabled, public}'

# Make images public
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public":true,"enabled":true}'
```

### Boot from Local Disk Returns to the Menu or Stops

**Symptoms**: Choosing **Boot from Local Disk** (or letting the menu time out on it) shows "Network setup failed", returns to the menu, or leaves the machine at a firmware prompt instead of starting the operating system on the disk.

**How it works**: iPXE cannot start a local operating system directly on UEFI, so Bootimus hands control back to the firmware. On BIOS clients the menu first tries `sanboot --drive 0x80`, which starts the first hard disk directly. On UEFI clients iPXE exits with a failure status so the firmware moves on to the next entry in its boot order. Clients that chain the Bootimus bootloaders released before this behaviour was introduced need the updated `bootimus.efi`, `ipxe.efi` or `undionly.kpxe`; Bootimus serves them automatically over TFTP and HTTP.

**Checklist**:
1. Put the network boot entry first and the local disk second in the firmware boot order. Firmware only continues to the disk when another entry follows PXE.
2. On UEFI, make sure the disk has a boot entry (for example `EFI/BOOT/BOOTX64.EFI` or a registered Windows Boot Manager entry). Some firmware stops at a setup screen when iPXE returns and nothing else is bootable.
3. If you serve your own iPXE build rather than the bundled bootloaders, add the same handling to the script that chains `menu.ipxe`: after the chain returns, test `isset ${localboot}` and `exit 1`.

### MAC Address Not Detected

**Symptoms**: Boot logs show "unknown" MAC address

**Possible causes**:
1. iPXE cannot detect MAC address from network interface
2. Client using multiple network interfaces

**Solution**:
```bash
# Check boot logs for actual IP address
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/logs | jq '.data[] | {mac_address, ip_address}'

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
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | jq

# Verify image assignments
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | \
  jq '.data.allowed_images'

# Re-assign images
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/assign-images \
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
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients | \
  jq '.data[] | select(.mac_address=="00:11:22:33:44:55")'

# Update existing client instead
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{"name":"Updated Name","enabled":true}'

# Or delete and re-create
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

## Next Steps

-  Configure [Image Management](images.md) to add ISOs
-  Use [Admin Console](admin.md) for management
-  Set up [DHCP Configuration](dhcp.md) for network booting
-  View [Boot Logs](admin.md#boot-logs) for monitoring
