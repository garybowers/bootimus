#  镜像管理指南

管理 ISO 镜像、提取引导文件,以及处理 Debian/Ubuntu netboot 等特殊场景的完整指南。

##  目录

- [添加镜像](#添加镜像)
- [Kernel 提取](#kernel-提取)
- [Netboot 支持](#netboot-支持)
- [Ubuntu Desktop 优化](#ubuntu-desktop-优化)
- [已支持的发行版](#已支持的发行版)
- [故障排查](#故障排查)

## 添加镜像

### 通过 Web 界面上传

1. 进入管理面板:`http://your-server:8081`
2. 点击 **"Upload ISO"** 按钮
3. 拖放 ISO 文件或点击浏览
4. 可选地添加描述
5. 勾选 **"Public"** 让所有客户端可访问
6. 点击 **"Upload"**

**上传限制**:每个文件 10GB

### 通过 API 上传

```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### 从 URL 下载

直接把 ISO 下载到服务器,无需本地上传:

**通过 Web 界面**:
1. 点击 **"Download from URL"** 按钮
2. 输入 ISO 下载 URL
3. 添加描述
4. 点击 **"Download"**

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'
```

**监控进度**:
```bash
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### 用文件夹组织

放在子目录中的 ISO 会在引导菜单中自动分组:

```
/data/isos/
├── ubuntu-24.04.iso              # ungrouped
├── linux/                        # "linux" group
│   ├── debian-12.iso
│   └── servers/                  # "servers" subgroup under "linux"
│       └── truenas-scale.iso
└── windows/                      # "windows" group
    └── win11.iso
```

分组在启动时和扫描时自动创建。也可以通过管理 UI 的 Groups 标签手动管理。

### 扫描已有 ISO

如果你手动把 ISO 复制到数据目录(包括子目录):

1. 把 ISO 文件复制到 `/data/isos/` 目录(或子目录)
2. 在管理面板点击 **"Scan for ISOs"** 按钮
3. Bootimus 检测并注册新 ISO,并根据文件夹创建分组

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Kernel 提取

大多数现代 ISO 都支持通过 iPXE 的 `sanboot` 命令直接 HTTP 引导,即下载并引导整个 ISO。但提取 kernel 和 initrd 带来显著好处:

###  Kernel 提取的好处

- **更快的引导时间**:只下载 kernel/initrd(~100MB),而不是整个 ISO(1-10GB)
- **更少的带宽**:对多客户端网络至关重要
- **更好的兼容性**:某些 ISO 不能正确支持 `sanboot`
- **网络安装**:为 Debian/Ubuntu 安装器使用 netboot 文件
- **UEFI Secure Boot**:提取会检测 ISO 的已签名 shim,让 Secure Boot 客户端能够校验 kernel — 参见 [Secure Boot 指南](secure-boot.md)

### 如何提取

**通过 Web 界面**:
1. 进入 **Images** 标签页
2. 找到你的 ISO 镜像
3. 点击 **"Extract"** 按钮
4. 等待提取完成

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04-live-server-amd64.iso"}'
```

### 手动提取

如果内置提取器不支持你的 ISO,你可以手动提取引导文件,bootimus 会自动检测到它们。

1. 创建一个与 ISO 同名(去掉 `.iso` 扩展名)的目录:
   ```bash
   mkdir -p data/isos/my-custom-distro/
   ```

2. 把 kernel 和 initrd 放进该目录,使用以下精确名称:
   ```
   data/isos/
   ├── my-custom-distro.iso
   └── my-custom-distro/
       ├── vmlinuz          # kernel
       └── initrd           # initrd/initramfs
   ```

3. 在管理面板点击 **"Scan for ISOs"**(或重启 bootimus)。镜像会被自动识别为已提取并设为 kernel 引导方法。

这对子目录中的 ISO 也有效:
```
data/isos/linux/my-custom-distro.iso
data/isos/linux/my-custom-distro/vmlinuz
data/isos/linux/my-custom-distro/initrd
```

### 提取了什么

Bootimus 自动检测发行版并提取:

- **Kernel**:`vmlinuz`(或 `linux`、`bzImage`)
- **Initrd**:`initrd`、`initrd.gz`、`initrd.lz`
- **Squashfs**(Ubuntu/Debian live):`filesystem.squashfs`
- **发行版元数据**:OS 类型、引导参数

**提取文件位置**:
```
/data/isos/
├── ubuntu-24.04.iso                    # Original ISO
└── ubuntu-24.04/                       # Extracted directory
    ├── vmlinuz                         # Kernel
    ├── initrd                          # Initrd
    └── casper/
        └── filesystem.squashfs         # Squashfs filesystem
```

### 自动选择引导方法

提取后,Bootimus 自动选择最优的引导方法:

| 发行版 | 引导方法 | 下载量 |
|--------------|-------------|-----------|
| Ubuntu Desktop(已提取) | `fetch=` | ~2.8GB(仅 squashfs) |
| Ubuntu Desktop(未提取) | `url=` | ~18GB(ISO × 3) |
| Ubuntu Server(netboot) | Netboot | ~50MB(netboot 文件) |
| Debian Installer(netboot) | Netboot | ~30MB(netboot 文件) |
| Arch Linux | HTTP boot | ~100MB(kernel/initrd) |
| Fedora/RHEL | HTTP boot | ~150MB(kernel/initrd + stage2) |

## Netboot 支持

某些安装器 ISO(Debian、Ubuntu Server)不包含完整的操作系统 — 它们设计成在安装期间下载软件包。对它们,Bootimus 支持下载官方 netboot 文件。

###  检测 Netboot 需求

当你提取 Debian 或 Ubuntu Server 安装器 ISO 时,Bootimus 会检测到它需要 netboot:

**指标**:
- ISO 包含 `/install/` 目录(而不是 `/casper/`)
- 安装器类型(而非 live/desktop)
- ISO 体积小(< 1GB)

**管理面板显示**:
-  "Netboot Required" 标记
- 📥 "Download Netboot" 按钮

### 下载 Netboot 文件

**通过 Web 界面**:
1. 进入 **Images** 标签页
2. 找到带 "Netboot Required" 标记的安装器 ISO
3. 点击 **"Download Netboot"** 按钮
4. 等待下载和提取

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

### Netboot 文件是什么?

Netboot 文件是发行版提供的官方、最小化引导文件:

**Debian netboot**:
- 来源:`http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`
- 大小:~30MB
- 包含:`vmlinuz`、`initrd.gz`、安装器文件

**Ubuntu netboot**:
- 来源:`http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`
- 大小:~50MB
- 包含:`vmlinuz`、`initrd.gz`、安装器文件

### Netboot 如何工作

1. **客户端引导**:下载 netboot kernel/initrd(~50MB)
2. **安装器启动**:netboot initrd 启动网络安装器
3. **软件包下载**:安装器从 Ubuntu/Debian 镜像站点下载软件包
4. **安装**:直接从互联网仓库安装操作系统

**好处**:
-  始终获取最新软件包(而不是过期的 ISO 软件包)
-  到 PXE 服务器的带宽最小(无需下载 ISO)
-  存储需求更小
-  官方签名的引导文件

### Debian Installer Netboot

**支持的 ISO**:
- `debian-*-netinst.iso` — 网络安装器
- 带 `/install/` 目录的小型 Debian 安装器 ISO

**检测**:
```
ISO structure:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**Netboot URL**:`http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`

**引导参数**:`priority=critical ip=dhcp`

### Ubuntu Server Netboot

**支持的 ISO**:
- `ubuntu-*-live-server-*.iso` — 带 `/install/` 目录的 live server 安装器
- 较旧的 Ubuntu server 安装器

**检测**:
```
ISO structure:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**Netboot URL**:`http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`

**引导参数**:`ip=dhcp`

###  重要:Ubuntu Desktop vs Server

有**两种** Ubuntu ISO,引导方法不同:

| 类型 | ISO 名称模式 | 目录 | 引导方法 | Netboot? |
|------|------------------|-----------|-------------|----------|
| **Desktop/Live** | `ubuntu-*-desktop-*.iso` | `/casper/` | `fetch=` 或 `url=` |  否 |
| **Server Installer** | `ubuntu-*-live-server-*.iso`(带 `/install/`) | `/install/` | Netboot |  是 |

**Ubuntu Desktop** (`/casper/`):
- 包含完整的 live OS
- 使用 casper 引导,搭配 `fetch=` 或 `url=`
- 提取 kernel 以使用 `fetch=`(只下载 squashfs)
- 不支持 netboot

**Ubuntu Server Installer** (`/install/`):
- 最小化网络安装器
- 需要 netboot 文件
- 安装期间下载软件包
- 效率高得多

## Ubuntu Desktop 优化

Ubuntu Desktop ISO 使用 casper live 引导系统。不做优化的话,它们会下载整个 ISO **三次**(6GB ISO 约 18GB)。

###  问题:ISO 三次下载

**默认行为**(未提取):
```
Boot parameter: url=http://server/ubuntu.iso

Result:
- Download 1: Kernel verifies ISO (6GB)
- Download 2: Initrd verifies ISO (6GB)
- Download 3: Casper mounts ISO (6GB)
Total: ~18GB downloaded
```

###  方案 1:提取并使用 `fetch=` 参数

**提取后**:
```
Boot parameter: fetch=http://server/ubuntu/casper/filesystem.squashfs

Result:
- Download: Only squashfs (~2.8GB)
Total: ~2.8GB downloaded
```

**如何启用**:
1. 从 ISO 提取 kernel/initrd
2. Bootimus 自动使用 `fetch=` 参数
3. 仅下载 squashfs(不下载整个 ISO)

**节省**:减少 85%(18GB → 2.8GB)

###  方案 2:改用 Ubuntu Server Netboot

对于服务器部署,使用 Ubuntu Server 安装器搭配 netboot:

**Netboot 方法**:
```
1. Upload ubuntu-server.iso
2. Extract kernel/initrd
3. Download netboot files
4. Boot with netboot (~50MB download)
5. Install from Ubuntu repositories
```

**节省**:减少 99%(18GB → 50MB)

### 引导参数参考

**Ubuntu Desktop (casper)**:
```bash
# Default (no extraction) - downloads ISO 3 times
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp url=http://server/ubuntu.iso

# Optimised (with extraction) - downloads squashfs once
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp fetch=http://server/ubuntu/casper/filesystem.squashfs
```

**Ubuntu Server (netboot)**:
```bash
# Netboot - minimal download
ip=dhcp
```

## 已支持的发行版

### 完全测试

| 发行版 | Kernel 提取 | Netboot | 备注 |
|--------------|-------------------|---------|-------|
| **Arch Linux** |  是 |  N/A | `/arch/boot/x86_64/vmlinuz-linux` |
| **Fedora Workstation** |  是 |  N/A | `/isolinux/vmlinuz` |
| **Rocky Linux** |  是 |  N/A | `/isolinux/vmlinuz` |
| **Debian(installer)** |  是 |  是 | `/install/vmlinuz` + netboot |
| **Debian Live** |  是 |  否 | `/live/vmlinuz` |
| **Ubuntu Desktop** |  是 |  否 | `/casper/vmlinuz` + fetch 优化 |
| **Ubuntu Server** |  是 |  是 | `/install/vmlinuz` + netboot |
| **Pop!_OS** |  是 |  否 | `/casper/vmlinuz` |
| **TrueNAS SCALE** |  是 |  否 | `/vmlinuz` + `/initrd.img`(根目录) |
| **Proxmox VE** |  是 |  否 | `/boot/linux26` + `/boot/initrd.img` |
| **openSUSE** |  是 |  N/A | `/boot/x86_64/loader/linux` |
| **NixOS** |  N/A |  N/A | Sanboot |

### 检测模式

Bootimus 通过扫描特定文件模式来检测发行版:

**Arch Linux**:
```
/arch/boot/x86_64/vmlinuz-linux
/arch/boot/x86_64/initramfs-linux.img
```

**Fedora/RHEL/Rocky**:
```
/isolinux/vmlinuz
/isolinux/initrd.img
```

**Ubuntu Desktop (casper)**:
```
/casper/vmlinuz or /casper/vmlinuz.efi
/casper/initrd or /casper/initrd.gz or /casper/initrd.lz
/casper/filesystem.squashfs
```

**Ubuntu Server Installer**:
```
/install/vmlinuz or /install.amd/vmlinuz
/install/initrd.gz or /install.amd/initrd.gz
```

**Debian Installer**:
```
/install/vmlinuz or /install.amd/vmlinuz
/install/initrd.gz or /install.amd/initrd.gz
```

**TrueNAS SCALE**:
```
/vmlinuz
/initrd.img
/live/filesystem.squashfs
```

**Proxmox VE**:
```
/boot/linux26
/boot/initrd.img
```

## 故障排查

### 提取失败

**症状**:管理面板出现 "Extraction failed" 错误

**常见原因**:
1. **ISO 损坏**:重新下载 ISO
2. **不支持的 ISO**:检查该发行版是否被支持
3. **磁盘空间**:确保有足够空间用于提取
4. **权限**:检查数据目录的文件权限

**调试**:
```bash
# Check extraction logs
docker logs bootimus | grep -i extract

# Verify ISO integrity
sha256sum ubuntu.iso

# Check disk space
df -h /data/isos/

# Test manual mount
sudo mount -o loop ubuntu.iso /mnt
ls /mnt/casper/
sudo umount /mnt
```

### Netboot 下载失败

**症状**:"Netboot download failed" 错误

**常见原因**:
1. **网络连通性**:无法访问 Debian/Ubuntu 镜像站点
2. **URL 已变更**:镜像 URL 可能已被更新
3. **Tarball 提取失败**:下载损坏

**解决方案**:
```bash
# Test mirror connectivity
curl -I http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz

# Check server logs
docker logs bootimus | grep -i netboot

# Manually verify netboot URL
wget http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz
tar -tzf netboot.tar.gz | grep vmlinuz
```

### 引导菜单显示错误的镜像类型

**症状**:镜像显示 "[kernel]" 标记但不以 kernel 方法引导

**原因**:数据库和文件系统不同步

**解决方案**:
```bash
# Re-extract kernel/initrd
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'

# Or re-scan ISOs
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### 客户端多次下载 ISO

**症状**:Ubuntu Desktop ISO 被下载 3 次

**原因**:在未提取的情况下使用了 `url=` 参数

**解决方案**:
1. 从 ISO 提取 kernel/initrd
2. Bootimus 会自动使用 `fetch=` 参数
3. 仅下载 squashfs(不下载整个 ISO)

**验证**:
```bash
# Check if extracted
ls -la /data/isos/ubuntu-24.04/casper/filesystem.squashfs

# Check server logs during boot
docker logs -f bootimus
# Look for: "fetch=..." instead of "url=..."
```

### 显示需要 Netboot 但没有下载按钮

**症状**:镜像显示 "Netboot Required" 但没有下载按钮

**原因**:未配置 Netboot URL 或检测失败

**解决方案**:
```bash
# Check image details
curl -u admin:password http://localhost:8081/api/images | jq '.data[] | select(.filename=="debian-13.2.0-amd64-netinst.iso")'

# Verify netboot_required and netboot_url fields
# If netboot_url is empty, the ISO detection may have failed

# Try re-extracting
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

## 下一步

-  查看 [管理控制台指南](admin.md) 管理镜像
-  阅读 [部署指南](deployment.md) 进行存储配置
-  配置 [DHCP 服务器](dhcp.md) 实现 PXE 引导
-  设置 [客户端管理](clients.md) 进行访问控制
