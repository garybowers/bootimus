#  DHCP Configuration Guide

Complete guide for configuring various DHCP servers to work with Bootimus for PXE network booting.

##  Table of Contents

- [Built-in proxyDHCP (standalone mode)](#built-in-proxydhcp-standalone-mode)
- [Overview](#overview)
- [ISC DHCP Server](#isc-dhcp-server)
- [Dnsmasq](#dnsmasq)
- [MikroTik RouterOS](#mikrotik-routeros)
- [Ubiquiti EdgeRouter](#ubiquiti-edgerouter)
- [pfSense](#pfsense)
- [OPNsense](#opnsense)
- [Windows Server DHCP](#windows-server-dhcp)
- [Troubleshooting](#troubleshooting)
- [PiHole](#pi-hole-dnsmasq)

## Built-in proxyDHCP (standalone mode)

Bootimus includes a built-in proxyDHCP responder (RFC 4578). When enabled, Bootimus answers the PXE-specific DHCP options itself — **your existing DHCP server needs no PXE configuration at all**. It keeps assigning IPs as usual; Bootimus only responds to PXE clients with `next-server`, bootfile, and the PXE vendor class, and never offers an IP address of its own.

This is the simplest way to run Bootimus in any environment where you don't own the main DHCP server, or where you don't want to touch it.

### How it works

1. Client broadcasts `DHCPDISCOVER`.
2. The LAN's existing DHCP server replies with an IP lease (no PXE info needed).
3. Bootimus replies on the same broadcast with PXE boot info only — no IP.
4. The client's PXE ROM merges both replies: IP from the main DHCP, bootfile from Bootimus.

Because Bootimus never offers an IP, there is no conflict with the existing DHCP server and no lease pool to coordinate.

### Enable

```bash
# CLI flag
bootimus serve --proxy-dhcp

# Environment variable
BOOTIMUS_PROXY_DHCP_ENABLED=true

# YAML config
proxy_dhcp:
  enabled: true
  bootfile_bios: undionly.kpxe           # legacy BIOS PXE
  bootfile_uefi: bootimus.efi            # UEFI x64
  bootfile_arm64: bootimus-arm64.efi     # UEFI ARM64
```

Off by default so existing installs that already run dnsmasq/ISC-DHCP/etc. aren't surprised.

### Requirements and caveats

- **Binds UDP/67 and UDP/4011.** UDP/67 requires `CAP_NET_BIND_SERVICE` or root; UDP/4011 is the PXE boot-server discovery port that some UEFI ROMs (notably AMI/Supermicro) follow up on after the initial offer. In Docker, the default image already runs as root so no extra capability needed; add `--cap-add NET_BIND_SERVICE` only for rootless setups.
- **Same broadcast domain.** proxyDHCP relies on seeing client DHCP broadcasts. It works on a flat LAN or a single VLAN. If your network uses a DHCP relay (`ip helper-address`) to forward DHCP across VLANs, the relay forwards to your main DHCP but not to Bootimus — add Bootimus as an additional relay target, or keep targets on the same VLAN as Bootimus.
- **Docker networking.** Use `macvlan`, `ipvlan`, or `network_mode: host` so the container is a first-class participant on the broadcast domain. A default bridge network will not work — broadcasts get trapped inside `docker0`.
- **Two proxyDHCP servers on the same LAN is legal, but debugging is a nightmare.** If you enable Bootimus's built-in proxyDHCP, disable any existing dnsmasq proxyDHCP advertising PXE on the same network.

### docker-compose example

```yaml
services:
  bootimus:
    image: garybowers/bootimus:latest
    environment:
      BOOTIMUS_PROXY_DHCP_ENABLED: "true"
      BOOTIMUS_SERVER_ADDR: 10.76.42.41    # the container's macvlan IP
    ports:
      - "67:67/udp"                         # proxyDHCP
      - "4011:4011/udp"                     # PXE boot-server discovery
      - "69:69/udp"                         # TFTP
      - "8080:8080/tcp"
      - "8081:8081/tcp"
    networks:
      lan:
        ipv4_address: 10.76.42.41
networks:
  lan:
    driver: macvlan
    driver_opts:
      parent: eth0
    ipam:
      config:
        - subnet: 10.76.42.0/24
```

### Verifying

Container logs should show:

```
proxyDHCP: listening on UDP/67 + UDP/4011, advertising next-server=10.76.42.41 (BIOS=undionly.kpxe, UEFI=bootimus.efi, ARM64=bootimus-arm64.efi)
```

And per-client lines when a PXE boot happens:

```
proxyDHCP: DISCOVER -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
proxyDHCP: REQUEST  -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
TFTP: Client requesting file: bootimus.efi
```

The admin UI's **Server Information** panel also shows current proxyDHCP state.

---

## Overview

To enable PXE network booting, your DHCP server must be configured to:

1. **Provide IP addresses** to clients (standard DHCP)
2. **Point to boot server** (`next-server` or DHCP option 66)
3. **Specify bootloader filename** (DHCP option 67)
4. **Detect iPXE** and chain to HTTP menu (optional but recommended)

**Replace `192.168.1.10` with your Bootimus server IP address in all examples below.**

### Bootloader Filenames

| Client Type | DHCP Filename | Notes |
|-------------|---------------|-------|
| UEFI (x86_64) | `bootimus.efi` (or `ipxe.efi`) | Custom-built iPXE with embedded script |
| UEFI (ARM64) | `bootimus-arm64.efi` (or `ipxe-arm64.efi`) | Custom-built iPXE with embedded script |
| Legacy BIOS | `undionly.kpxe` | Standard PXE bootloader |

> **Secure Boot:** Bootimus does not currently ship Microsoft-signed Secure Boot binaries. If your target machines have Secure Boot enabled, either disable it in firmware, or enrol Bootimus's iPXE build into the firmware's Secure Boot keystore (MOK).

### Boot Flow

```
Client → DHCP Request
      ← DHCP Offer (IP, next-server, bootloader filename)
Client → TFTP Request for bootloader (bootimus.efi or undionly.kpxe)
      ← Bootloader downloaded
Client → HTTP Request for menu.ipxe
      ← Boot menu displayed
Client → Boot selected ISO
```

## ISC DHCP Server

ISC DHCP is the standard DHCP server on most Linux distributions.

### Configuration

Edit `/etc/dhcp/dhcpd.conf`:

```conf
# Basic DHCP configuration
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    option routers 192.168.1.1;
    option domain-name-servers 8.8.8.8, 8.8.4.4;

    # PXE boot server
    next-server 192.168.1.10;  # Bootimus server IP

    # Detect if client is already running iPXE
    if exists user-class and option user-class = "iPXE" {
        # Client has iPXE, chain to HTTP menu
        filename "http://192.168.1.10:8080/menu.ipxe";
    }
    # UEFI systems (x86_64)
    elsif option arch = 00:07 {
        filename "ipxe.efi";
    }
    # UEFI systems (alternative)
    elsif option arch = 00:09 {
        filename "ipxe.efi";
    }
    # Legacy BIOS
    else {
        filename "undionly.kpxe";
    }
}
```

### Advanced: Per-Client Boot Configuration

```conf
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    next-server 192.168.1.10;

    # Specific client configuration
    host lab-server-1 {
        hardware ethernet 00:11:22:33:44:55;
        fixed-address 192.168.1.50;
        filename "ipxe.efi";
    }

    # Default configuration for other clients
    if exists user-class and option user-class = "iPXE" {
        filename "http://192.168.1.10:8080/menu.ipxe";
    }
    elsif option arch = 00:07 or option arch = 00:09 {
        filename "ipxe.efi";
    }
    else {
        filename "undionly.kpxe";
    }
}
```

### Restart Service

```bash
# Test configuration
sudo dhcpd -t -cf /etc/dhcp/dhcpd.conf

# Restart DHCP service
sudo systemctl restart isc-dhcp-server

# Check status
sudo systemctl status isc-dhcp-server

# View logs
sudo journalctl -u isc-dhcp-server -f
```

## Dnsmasq

Dnsmasq is a lightweight DHCP and DNS server, popular on embedded systems and routers.

### Configuration

Edit `/etc/dnsmasq.conf`:

```conf
# DHCP range
dhcp-range=192.168.1.100,192.168.1.200,12h

# Default gateway
dhcp-option=3,192.168.1.1

# DNS servers
dhcp-option=6,8.8.8.8,8.8.4.4

# PXE boot configuration
dhcp-boot=tag:!ipxe,undionly.kpxe,192.168.1.10
dhcp-boot=tag:ipxe,http://192.168.1.10:8080/menu.ipxe

# UEFI support
dhcp-match=set:efi-x86_64,option:client-arch,7
dhcp-match=set:efi-x86_64,option:client-arch,9
dhcp-boot=tag:efi-x86_64,tag:!ipxe,ipxe.efi,192.168.1.10

# Legacy BIOS support
dhcp-match=set:bios,option:client-arch,0
dhcp-boot=tag:bios,tag:!ipxe,undionly.kpxe,192.168.1.10

# Enable TFTP server (optional, if using dnsmasq as TFTP server)
# enable-tftp
# tftp-root=/var/lib/tftpboot
```

### Minimal Configuration

If you just want basic PXE without iPXE detection:

```conf
dhcp-range=192.168.1.100,192.168.1.200,12h
dhcp-option=3,192.168.1.1
dhcp-option=6,8.8.8.8

# TFTP server and boot file
dhcp-boot=undionly.kpxe,192.168.1.10
```

### Restart Service

```bash
# Test configuration
sudo dnsmasq --test

# Restart service
sudo systemctl restart dnsmasq

# Check status
sudo systemctl status dnsmasq

# View logs
sudo journalctl -u dnsmasq -f
```

## MikroTik RouterOS

MikroTik routers are popular for network booting due to their flexibility and performance.

### Via Web Interface (WebFig)

#### 1. Define DHCP Options
* Navigate to **IP** > **DHCP Server** > **Options**.
* Click **Add New** for each of the following (ensure the **Value** includes **single quotes**):
    * **Option 66 (Server)**: Name: `tftp-server` | Code: `66` | Value: `'<BOOT_SERVER_IP>'`.
    * **Option 67 (BIOS)**: Name: `boot-bios` | Code: `67` | Value: `'undionly.kpxe'`.
    * **Option 67 (UEFI)**: Name: `boot-uefi` | Code: `67` | Value: `'ipxe.efi'`.

#### 2. Create Option Sets
* Navigate to **IP** > **DHCP Server** > **Option Sets**.
* **BIOS Set**: Click **Add New**, Name: `set-bios`, then add `tftp-server` and `boot-bios`.
* **UEFI Set**: Click **Add New**, Name: `set-uefi`, then add `tftp-server` and `boot-uefi`.

#### 3. Configure Option Matcher (Detection Logic)
* Navigate to **IP** > **DHCP Server** > **Option Matcher**.
* **BIOS Entry**: Name: `match-bios` | Code: `93` | Value: `0x0000` | Option Set: `set-bios` | Server: `<DHCP_SERVER_NAME>`.
* **UEFI Entry**: Name: `match-uefi-7` | Code: `93` | Value: `0x0007` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.
* **UEFI Alt Entry**: Name: `match-uefi-9` | Code: `93` | Value: `0x0009` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.

#### 4. DHCP Network Configuration
* Navigate to **IP** > **DHCP Server** > **Networks**.
* Open the entry for your subnet (e.g., `192.168.88.0/24`).
* **Next Server**: Enter your `<BOOT_SERVER_IP>`.
* **Boot File Name**: **LEAVE EMPTY** (The Option Matcher dynamically injects the filename).

---

### Via Command Line (CLI)



Replace the placeholders `<BOOT_SERVER_IP>`, `<DHCP_SERVER_NAME>`, and `<YOUR_SUBNET>` with your specific details before running.

```routeros
# 1. Define DHCP Options
/ip dhcp-server option
add code=66 name=tftp-server value="'<BOOT_SERVER_IP>'"
add code=67 name=boot-bios value="'undionly.kpxe'"
add code=67 name=boot-uefi value="'ipxe.efi'"

# 2. Create Option Sets
/ip dhcp-server option sets
add name=set-bios options=tftp-server,boot-bios
add name=set-uefi options=tftp-server,boot-uefi

# 3. Create Option Matchers for Architecture Detection
/ip dhcp-server option-matcher
add code=93 name=match-bios option-set=set-bios server=<DHCP_SERVER_NAME> value=0x0000
add code=93 name=match-uefi-7 option-set=set-uefi server=<DHCP_SERVER_NAME> value=0x0007
add code=93 name=match-uefi-9 option-set=set-uefi server=<DHCP_SERVER_NAME> value=0x0009

# 4. Apply to DHCP Network
/ip dhcp-server network
set [find address="<YOUR_SUBNET>"] boot-file-name="" next-server=<BOOT_SERVER_IP>

## Ubiquiti EdgeRouter

Ubiquiti EdgeRouters use EdgeOS (based on Vyatta/VyOS).

### Via Web UI

1. Navigate to **Services > DHCP Server**
2. Select your DHCP server (e.g., `LAN`)
3. Under **Actions**, click **Edit**
4. Scroll to **PXE Settings**:
   - **Boot File**: `undionly.kpxe` (BIOS) or `ipxe.efi` (UEFI)
   - **Boot Server**: `192.168.1.10`
5. Click **Save**

### Via CLI

```bash
configure

# Set TFTP server for network boot
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-server 192.168.1.10
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-name undionly.kpxe

# Advanced: UEFI support
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "option arch code 93 = unsigned integer 16;"
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "if option arch = 00:07 { filename &quot;ipxe.efi&quot;; } else { filename &quot;undionly.kpxe&quot;; }"

commit
save
exit
```

**Note**: Replace `LAN` with your actual shared network name if different.

### Verify Configuration

```bash
show service dhcp-server
show service dhcp-server leases
```

## pfSense

pfSense is a popular open-source firewall and router distribution.

### Configuration Steps

1. Navigate to **Services > DHCP Server**
2. Select the interface (e.g., **LAN**)
3. Scroll down to **Network Booting** section
4. Configure:
   - **Enable Network Booting**:  Check
   - **Next Server**: `192.168.1.10`
   - **Default BIOS Filename**: `undionly.kpxe`
   - **UEFI 64-bit Filename**: `ipxe.efi`
5. Click **Save**

### Advanced: Custom Options

For iPXE detection, add custom DHCP options:

1. Navigate to **Services > DHCP Server**
2. Select interface
3. Scroll to **Additional BOOTP/DHCP Options**
4. Add options:

```
# Option 60 (Class Identifier)
60 text "PXEClient"

# Option 66 (TFTP Server)
66 text "192.168.1.10"

# Option 67 (Bootfile Name)
67 text "undionly.kpxe"
```

### Static DHCP Mappings

For specific clients:

1. **Services > DHCP Server > LAN**
2. Scroll to **DHCP Static Mappings**
3. Click **Add**
4. Configure:
   - **MAC Address**: `00:11:22:33:44:55`
   - **IP Address**: `192.168.1.50`
   - **Filename**: `ipxe.efi`
   - **Root Path**: Leave empty
5. Click **Save**

## OPNsense

OPNsense is a pfSense fork with a modern interface.

### Configuration Steps

1. Navigate to **Services > DHCPv4 > [Interface]**
2. Scroll to **Network Booting**
3. Configure:
   - **Enable Network Booting**:  Check
   - **Next Server**: `192.168.1.10`
   - **Default BIOS Filename**: `undionly.kpxe`
   - **UEFI 64-bit Filename**: `ipxe.efi`
4. Click **Save**
5. Click **Apply Changes**

### Advanced Configuration

1. Navigate to **Services > DHCPv4 > [Interface]**
2. Click **Additional Options** tab
3. Add custom options similar to pfSense

## Windows Server DHCP

Windows Server DHCP service.

### Configuration Steps

1. Open **DHCP Manager** (`dhcpmgmt.msc`)
2. Expand your DHCP server
3. Expand **IPv4**
4. Right-click **Scope** → **Scope Options**
5. Configure:
   - **066 Boot Server Host Name**: `192.168.1.10`
   - **067 Bootfile Name**: `undionly.kpxe`

### Advanced: UEFI and BIOS Detection

1. Right-click **Scope** → **Set Predefined Options**
2. Click **Add**
3. Create option code 60 (Vendor Class):
   - **Code**: 60
   - **Name**: Vendor Class
   - **Data Type**: String
4. Create policies for UEFI/BIOS:
   - Right-click **Policies** → **New Policy**
   - **Condition**: Vendor Class equals "PXEClient:Arch:00007" (UEFI)
   - **Options**: Set bootfile to `ipxe.efi`
   - Repeat for BIOS (Arch:00000) with `undionly.kpxe`

### PowerShell Configuration

```powershell
# Set DHCP scope options
Set-DhcpServerv4OptionValue -ScopeId 192.168.1.0 -OptionId 66 -Value "192.168.1.10"
Set-DhcpServerv4OptionValue -ScopeId 192.168.1.0 -OptionId 67 -Value "undionly.kpxe"

# Create policy for UEFI
Add-DhcpServerv4Policy -Name "UEFI" -Condition OR -VendorClass EQ "PXEClient:Arch:00007"
Set-DhcpServerv4OptionValue -PolicyName "UEFI" -OptionId 67 -Value "ipxe.efi"

# Create policy for BIOS
Add-DhcpServerv4Policy -Name "BIOS" -Condition OR -VendorClass EQ "PXEClient:Arch:00000"
Set-DhcpServerv4OptionValue -PolicyName "BIOS" -OptionId 67 -Value "undionly.kpxe"
```

## Troubleshooting

### Client Not Receiving DHCP Offer

```bash
# Check DHCP server logs
sudo journalctl -u isc-dhcp-server -f   # ISC DHCP
sudo journalctl -u dnsmasq -f           # Dnsmasq

# Verify DHCP server is running
sudo systemctl status isc-dhcp-server
sudo systemctl status dnsmasq

# Check network connectivity
ping 192.168.1.10

# Capture DHCP traffic
sudo tcpdump -i eth0 port 67 or port 68
```

### Client Not Downloading Bootloader

```bash
# Verify Bootimus TFTP server is running
sudo netstat -ulnp | grep :69

# Test TFTP manually
tftp 192.168.1.10
> get undionly.kpxe
> quit

# Check Bootimus logs
docker logs bootimus | grep TFTP
```

### iPXE Loads But No Menu

```bash
# Verify HTTP server is running
curl http://192.168.1.10:8080/menu.ipxe

# Check if ISOs are available
curl -u admin:password http://192.168.1.10:8081/api/images

# Verify client has network access to HTTP port
telnet 192.168.1.10 8080

# Check Bootimus logs
docker logs bootimus -f
```

### Wrong Bootloader (UEFI vs BIOS)

```bash
# Check client firmware mode in DHCP logs
sudo journalctl -u isc-dhcp-server | grep -i "arch"

# UEFI clients send option 93 with value 00:07 or 00:09
# BIOS clients send option 93 with value 00:00

# Verify DHCP configuration handles architecture detection
```

### Client Boots But Shows "No Bootable Device"

Possible causes:
1. **DHCP option 67 incorrect**: Should be `undionly.kpxe` or `ipxe.efi`
2. **Next-server not set**: DHCP option 66 must point to Bootimus
3. **TFTP port blocked**: Firewall blocking port 69
4. **iPXE chainloading failed**: HTTP port 8080 not accessible

**Solution**:
```bash
# Verify all ports are accessible
sudo ufw allow 69/udp    # TFTP
sudo ufw allow 8080/tcp  # HTTP boot
sudo ufw allow 8081/tcp  # Admin (optional)

# Check Bootimus is listening
sudo netstat -tulpn | grep -E '69|8080|8081'
```

### DHCP Server Conflicts

If you have multiple DHCP servers on the network:

```bash
# Find all DHCP servers
sudo nmap --script broadcast-dhcp-discover

# Disable conflicting DHCP servers
# Or configure DHCP relay/helper if needed
```

## Next Steps

-  Configure [Image Management](images.md) to add ISOs
-  Set up [Admin Console](admin.md) for management
-  Configure [Client Management](clients.md) for access control
-  Review [Security Guide](security.md) for hardening


## Pi-hole (dnsmasq)

Pi-hole uses `dnsmasq` for its DHCP engine, which allows for granular architecture detection using configuration files.

### Via Web Interface

#### 1. Enable DHCP
* Navigate to **Settings** > **DHCP**.
* Check **DHCP server enabled**.
* Define your **IP range**, **Gateway**, and **Lease duration**.
* *Note: Detailed PXE options are not available in the Web UI and must be configured via CLI.*

---

### Via Command Line (CLI)



To support BIOS and UEFI simultaneously, you must create a custom configuration file in the `dnsmasq.d` directory.

#### 1. Create the Config File
* Open a terminal on your Pi-hole and create the file:
    `sudo nano /etc/dnsmasq.d/07-pxe.conf`

#### 2. Define the Logic
* Paste the following block into the file, replacing `<BOOT_SERVER_IP>` with your actual server IP:

```bash
# 1. Identify client architecture (Option 93)
dhcp-match=set:bios,option:client-arch,0
dhcp-match=set:efi-x64,option:client-arch,7
dhcp-match=set:efi-x64-alt,option:client-arch,9

# 2. Set the TFTP Server IP (Option 66)
dhcp-option=option:server-ip,<BOOT_SERVER_IP>

# 3. Assign filenames based on architecture (Option 67)
dhcp-boot=tag:bios,undionly.kpxe,,<BOOT_SERVER_IP>
dhcp-boot=tag:efi-x64,ipxe.efi,,<BOOT_SERVER_IP>
dhcp-boot=tag:efi-x64-alt,ipxe.efi,,<BOOT_SERVER_IP>

# 4. Optional: PXE Menu/Service definitions
pxe-service=tag:bios,x86PC,"Network Boot BIOS",undionly.kpxe,<BOOT_SERVER_IP>
pxe-service=tag:efi-x64,x86-64_EFI,"Network Boot UEFI",ipxe.efi,<BOOT_SERVER_IP>
pxe-service=tag:efi-x64-alt,x86-64_EFI,"Network Boot UEFI (Alt)",ipxe.efi,<BOOT_SERVER_IP>