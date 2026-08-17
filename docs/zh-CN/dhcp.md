#  DHCP 配置指南

配置各种 DHCP 服务器与 Bootimus 协同进行 PXE 网络引导的完整指南。

##  目录

- [内建 proxyDHCP(独立模式)](#内建-proxydhcp独立模式)
- [概述](#概述)
- [ISC DHCP Server](#isc-dhcp-server)
- [Dnsmasq](#dnsmasq)
- [MikroTik RouterOS](#mikrotik-routeros)
- [Ubiquiti EdgeRouter](#ubiquiti-edgerouter)
- [pfSense](#pfsense)
- [OPNsense](#opnsense)
- [Windows Server DHCP](#windows-server-dhcp)
- [故障排查](#故障排查)
- [PiHole](#pi-hole-dnsmasq)

## 内建 proxyDHCP(独立模式)

Bootimus 内置了一个 proxyDHCP 响应器(RFC 4578)。启用后,Bootimus 自己回应 PXE 专属的 DHCP 选项 — **你现有的 DHCP 服务器根本不需要任何 PXE 配置**。它照常分配 IP;Bootimus 只对 PXE 客户端回应 `next-server`、bootfile 和 PXE 厂商类,从不分配自己的 IP 地址。

这是在任何你不掌控主 DHCP 服务器、或你不想动它的环境里运行 Bootimus 的最简方式。

### 工作原理

1. 客户端广播 `DHCPDISCOVER`。
2. 局域网现有的 DHCP 服务器回复一个 IP 租约(无需 PXE 信息)。
3. Bootimus 在同一广播上仅回复 PXE 引导信息 — 不分配 IP。
4. 客户端的 PXE ROM 合并两个回复:IP 来自主 DHCP,bootfile 来自 Bootimus。

由于 Bootimus 从不分配 IP,因此和现有 DHCP 服务器没有任何冲突,也没有租约池要协调。

### 启用

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

默认关闭,以免让已经跑着 dnsmasq/ISC-DHCP/等的现有安装感到意外。

### 要求和注意事项

- **绑定 UDP/67 和 UDP/4011。** UDP/67 需要 `CAP_NET_BIND_SERVICE` 或 root 权限;UDP/4011 是 PXE boot-server 发现端口,某些 UEFI ROM(尤其是 AMI/Supermicro)会在最初的 offer 之后从这个端口继续。Docker 中,默认镜像已经以 root 运行,所以无需额外 capability;仅在 rootless 设置中才需要 `--cap-add NET_BIND_SERVICE`。
- **同一广播域。** proxyDHCP 依赖于看到客户端 DHCP 广播。它在扁平局域网或单个 VLAN 上工作。如果你的网络使用 DHCP 中继(`ip helper-address`)跨 VLAN 转发 DHCP,中继会转发到你的主 DHCP 但不转发到 Bootimus — 把 Bootimus 加为额外的中继目标,或把目标保持在与 Bootimus 同一个 VLAN。
- **Docker 网络。** 使用 `macvlan`、`ipvlan` 或 `network_mode: host`,这样容器才是广播域里的一等参与者。默认桥接网络不行 — 广播会被困在 `docker0` 里。
- **同一局域网上的两个 proxyDHCP 服务器是合法的,但调试是噩梦。** 如果你启用了 Bootimus 内置的 proxyDHCP,请在同一网络上禁用任何现有的、为 PXE 通告的 dnsmasq proxyDHCP。

### docker-compose 示例

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

### 验证

容器日志应该显示:

```
proxyDHCP: listening on UDP/67 + UDP/4011, advertising next-server=10.76.42.41 (BIOS=undionly.kpxe, UEFI=bootimus.efi, ARM64=bootimus-arm64.efi)
```

每次 PXE 引导发生时,会有针对每个客户端的行:

```
proxyDHCP: DISCOVER -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
proxyDHCP: REQUEST  -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
TFTP: Client requesting file: bootimus.efi
```

管理 UI 的 **Server Information** 面板也会显示当前的 proxyDHCP 状态。

---

## 概述

要启用 PXE 网络引导,你的 DHCP 服务器必须配置成:

1. **分配 IP 地址** 给客户端(标准 DHCP)
2. **指向引导服务器**(`next-server` 或 DHCP option 66)
3. **指定 bootloader 文件名**(DHCP option 67)
4. **检测 iPXE** 并链式跳转到 HTTP 菜单(可选但推荐)

**在以下所有示例中,把 `192.168.1.10` 换成你的 Bootimus 服务器 IP。**

### Bootloader 文件名

| 客户端类型 | DHCP 文件名 | 备注 |
|-------------|---------------|-------|
| UEFI (x86_64) | `bootimus.efi`(或 `ipxe.efi`) | 自定义构建的、内嵌脚本的 iPXE |
| UEFI (ARM64) | `bootimus-arm64.efi`(或 `ipxe-arm64.efi`) | 自定义构建的、内嵌脚本的 iPXE |
| Legacy BIOS | `undionly.kpxe` | 标准 PXE bootloader |

> **Secure Boot:** 对启用了 Secure Boot 的客户端,请激活内置的 `secureboot-official` bootloader 集,并改为通告 `ipxe-shimx64.efi`(x86_64)或 `ipxe-shimaa64.efi`(ARM64)作为 UEFI bootfile。该集处于激活状态时,内建 proxyDHCP 会自动完成这些。参见 [Secure Boot 指南](secure-boot.md)。

### 引导流程

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

ISC DHCP 是大多数 Linux 发行版上的标准 DHCP 服务器。

### 配置

编辑 `/etc/dhcp/dhcpd.conf`:

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

### 高级:按客户端引导配置

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

### 重启服务

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

Dnsmasq 是一个轻量级的 DHCP 和 DNS 服务器,在嵌入式系统和路由器上很流行。

### 配置

编辑 `/etc/dnsmasq.conf`:

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

### 最小化配置

如果你只想要不带 iPXE 检测的基本 PXE:

```conf
dhcp-range=192.168.1.100,192.168.1.200,12h
dhcp-option=3,192.168.1.1
dhcp-option=6,8.8.8.8

# TFTP server and boot file
dhcp-boot=undionly.kpxe,192.168.1.10
```

### 重启服务

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

MikroTik 路由器因其灵活性和性能,在网络引导中很受欢迎。

### 通过 Web 界面 (WebFig)

#### 1. 定义 DHCP 选项
* 进入 **IP** > **DHCP Server** > **Options**。
* 为以下每一项点击 **Add New**(确保 **Value** 包含**单引号**):
    * **Option 66 (Server)**:Name:`tftp-server` | Code:`66` | Value:`'<BOOT_SERVER_IP>'`。
    * **Option 67 (BIOS)**:Name:`boot-bios` | Code:`67` | Value:`'undionly.kpxe'`。
    * **Option 67 (UEFI)**:Name:`boot-uefi` | Code:`67` | Value:`'ipxe.efi'`。

#### 2. 创建 Option Sets
* 进入 **IP** > **DHCP Server** > **Option Sets**。
* **BIOS Set**:点击 **Add New**,Name:`set-bios`,然后加入 `tftp-server` 和 `boot-bios`。
* **UEFI Set**:点击 **Add New**,Name:`set-uefi`,然后加入 `tftp-server` 和 `boot-uefi`。

#### 3. 配置 Option Matcher(检测逻辑)
* 进入 **IP** > **DHCP Server** > **Option Matcher**。
* **BIOS 条目**:Name:`match-bios` | Code:`93` | Value:`0x0000` | Option Set:`set-bios` | Server:`<DHCP_SERVER_NAME>`。
* **UEFI 条目**:Name:`match-uefi-7` | Code:`93` | Value:`0x0007` | Option Set:`set-uefi` | Server:`<DHCP_SERVER_NAME>`。
* **UEFI Alt 条目**:Name:`match-uefi-9` | Code:`93` | Value:`0x0009` | Option Set:`set-uefi` | Server:`<DHCP_SERVER_NAME>`。

#### 4. DHCP 网络配置
* 进入 **IP** > **DHCP Server** > **Networks**。
* 打开你子网的条目(例如 `192.168.88.0/24`)。
* **Next Server**:输入你的 `<BOOT_SERVER_IP>`。
* **Boot File Name**:**留空**(Option Matcher 会动态注入文件名)。

---

### 通过命令行 (CLI)



在运行前,把占位符 `<BOOT_SERVER_IP>`、`<DHCP_SERVER_NAME>` 和 `<YOUR_SUBNET>` 替换为你的具体信息。

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
```

## Ubiquiti EdgeRouter

Ubiquiti EdgeRouter 使用基于 Vyatta/VyOS 的 EdgeOS。

### 通过 Web UI

1. 进入 **Services > DHCP Server**
2. 选择你的 DHCP 服务器(例如 `LAN`)
3. 在 **Actions** 下点击 **Edit**
4. 滚动到 **PXE Settings**:
   - **Boot File**:`undionly.kpxe`(BIOS)或 `ipxe.efi`(UEFI)
   - **Boot Server**:`192.168.1.10`
5. 点击 **Save**

### 通过 CLI

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

**注意**:如果不同,请把 `LAN` 替换为你实际的共享网络名。

### 验证配置

```bash
show service dhcp-server
show service dhcp-server leases
```

## pfSense

pfSense 是一个流行的开源防火墙和路由器发行版。

### 配置步骤

1. 进入 **Services > DHCP Server**
2. 选择接口(例如 **LAN**)
3. 滚动到 **Network Booting** 部分
4. 配置:
   - **Enable Network Booting**: 勾选
   - **Next Server**:`192.168.1.10`
   - **Default BIOS Filename**:`undionly.kpxe`
   - **UEFI 64-bit Filename**:`ipxe.efi`
5. 点击 **Save**

### 高级:自定义选项

为了 iPXE 检测,添加自定义 DHCP 选项:

1. 进入 **Services > DHCP Server**
2. 选择接口
3. 滚动到 **Additional BOOTP/DHCP Options**
4. 添加选项:

```
# Option 60 (Class Identifier)
60 text "PXEClient"

# Option 66 (TFTP Server)
66 text "192.168.1.10"

# Option 67 (Bootfile Name)
67 text "undionly.kpxe"
```

### 静态 DHCP 映射

针对特定客户端:

1. **Services > DHCP Server > LAN**
2. 滚动到 **DHCP Static Mappings**
3. 点击 **Add**
4. 配置:
   - **MAC Address**:`00:11:22:33:44:55`
   - **IP Address**:`192.168.1.50`
   - **Filename**:`ipxe.efi`
   - **Root Path**:留空
5. 点击 **Save**

## OPNsense

OPNsense 是 pfSense 的分支,具有现代化的界面。

### 配置步骤

1. 进入 **Services > DHCPv4 > [Interface]**
2. 滚动到 **Network Booting**
3. 配置:
   - **Enable Network Booting**: 勾选
   - **Next Server**:`192.168.1.10`
   - **Default BIOS Filename**:`undionly.kpxe`
   - **UEFI 64-bit Filename**:`ipxe.efi`
4. 点击 **Save**
5. 点击 **Apply Changes**

### 高级配置

1. 进入 **Services > DHCPv4 > [Interface]**
2. 点击 **Additional Options** 标签
3. 添加与 pfSense 类似的自定义选项

## Windows Server DHCP

Windows Server DHCP 服务。

### 配置步骤

1. 打开 **DHCP Manager**(`dhcpmgmt.msc`)
2. 展开你的 DHCP 服务器
3. 展开 **IPv4**
4. 右键点击 **Scope** → **Scope Options**
5. 配置:
   - **066 Boot Server Host Name**:`192.168.1.10`
   - **067 Bootfile Name**:`undionly.kpxe`

### 高级:UEFI 与 BIOS 检测

1. 右键点击 **Scope** → **Set Predefined Options**
2. 点击 **Add**
3. 创建选项码 60(厂商类):
   - **Code**:60
   - **Name**:Vendor Class
   - **Data Type**:String
4. 为 UEFI/BIOS 创建策略:
   - 右键点击 **Policies** → **New Policy**
   - **Condition**:Vendor Class 等于 "PXEClient:Arch:00007"(UEFI)
   - **Options**:把 bootfile 设为 `ipxe.efi`
   - 为 BIOS(Arch:00000)重复,用 `undionly.kpxe`

### PowerShell 配置

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

## 故障排查

### 客户端未收到 DHCP Offer

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

### 客户端未下载 Bootloader

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

### iPXE 加载但没有菜单

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

### Bootloader 错误(UEFI vs BIOS)

```bash
# Check client firmware mode in DHCP logs
sudo journalctl -u isc-dhcp-server | grep -i "arch"

# UEFI clients send option 93 with value 00:07 or 00:09
# BIOS clients send option 93 with value 00:00

# Verify DHCP configuration handles architecture detection
```

### 客户端引导但显示 "No Bootable Device"

可能原因:
1. **DHCP option 67 不正确**:应为 `undionly.kpxe` 或 `ipxe.efi`
2. **未设置 Next-server**:DHCP option 66 必须指向 Bootimus
3. **TFTP 端口被阻止**:防火墙阻挡了端口 69
4. **iPXE 链式加载失败**:HTTP 端口 8080 不可达

**解决方案**:
```bash
# Verify all ports are accessible
sudo ufw allow 69/udp    # TFTP
sudo ufw allow 8080/tcp  # HTTP boot
sudo ufw allow 8081/tcp  # Admin (optional)

# Check Bootimus is listening
sudo netstat -tulpn | grep -E '69|8080|8081'
```

### DHCP 服务器冲突

如果你的网络上有多个 DHCP 服务器:

```bash
# Find all DHCP servers
sudo nmap --script broadcast-dhcp-discover

# Disable conflicting DHCP servers
# Or configure DHCP relay/helper if needed
```

## 下一步

-  配置 [镜像管理](images.md) 添加 ISO
-  设置 [管理控制台](admin.md) 进行管理
-  配置 [客户端管理](clients.md) 进行访问控制


## Pi-hole (dnsmasq)

Pi-hole 使用 `dnsmasq` 作为其 DHCP 引擎,这允许通过配置文件进行细粒度的架构检测。

### 通过 Web 界面

#### 1. 启用 DHCP
* 进入 **Settings** > **DHCP**。
* 勾选 **DHCP server enabled**。
* 定义你的 **IP range**、**Gateway** 和 **Lease duration**。
* *注意:详细的 PXE 选项无法在 Web UI 中配置,必须通过 CLI 完成。*

---

### 通过命令行 (CLI)



要同时支持 BIOS 和 UEFI,你必须在 `dnsmasq.d` 目录下创建一个自定义配置文件。

#### 1. 创建配置文件
* 在你的 Pi-hole 上打开终端并创建文件:
    `sudo nano /etc/dnsmasq.d/07-pxe.conf`

#### 2. 定义逻辑
* 将以下内容粘贴到文件中,把 `<BOOT_SERVER_IP>` 替换为你实际的服务器 IP:

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

```
