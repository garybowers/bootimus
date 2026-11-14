#  镜像管理指南

管理 ISO 镜像、提取引导文件,以及处理 Debian/Ubuntu netboot 等特殊场景的完整指南。

##  目录

- [添加镜像](#添加镜像)
- [校验和与更新通知](#校验和与更新通知)
- [统一内核镜像 (UKI)](#统一内核镜像-uki)
- [OpenBSD 与 NetBSD](#openbsd-与-netbsd)
- [WinPE 与救援介质](#winpe-与救援介质)
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

### 校验和与更新通知

**下载时校验。** 通过 **Get Images**(内建目录)获取 ISO 时,Bootimus 会在其旁边寻找已发布的校验和文件(`SHA256SUMS`、`sha256sum.txt` 及类似文件),并在下载流式进行的同时计算哈希。不匹配会导致下载失败并删除文件,因此被截断或被篡改的镜像绝不会进入引导菜单。从普通 URL 下载时,也可以通过向 `/api/images/download` 传入 `sha256` 或 `checksum_url` 进行校验:

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{"url": "https://releases.ubuntu.com/24.04.4/ubuntu-24.04.4-live-server-amd64.iso",
       "checksum_url": "https://releases.ubuntu.com/24.04.4/SHA256SUMS"}'
```

已校验的镜像会在镜像列表中显示 **✓ Verified** 徽章,并在镜像属性面板中显示记录的 SHA-256。在没有已发布校验和的情况下下载的镜像,或手动上传的镜像,会标记为 **Unverified**;在属性面板中粘贴一个 SHA-256 或校验和文件的 URL,然后点击 **Verify**,即可在后台计算文件哈希(`POST /api/images/verify?filename=…`,用 `GET` 轮询同一 URL)。

**更新版本。** Bootimus 每天一次(以及在 **Settings → Housekeeping → Check now** 或 `POST /api/catalogue/check` 时)把已安装镜像与目录进行比较。从目录下载的镜像按其发行版和发布系列匹配,因此当 `24.04.4` 出现时,`ubuntu-24.04.3-desktop-amd64.iso` 会被标记;在此功能之前添加的镜像则按文件名形态匹配。被标记的镜像带有 **Update available** 徽章,属性面板中有 **Download newer version** 按钮,工具显示同样的徽章并带 **Download newer**,仪表盘会统计待处理的更新数量。允许远程配置更新时,检查使用在线目录;设置了 `--disable-remote-profiles` 时,则退回到二进制内置的目录。在 Housekeeping 下取消勾选 **Check the catalogue daily** 即可关闭。

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

## 统一内核镜像 (UKI)

统一内核镜像 (UKI) 是一个单独的 EFI 可执行文件,其中打包了 Linux 内核、它的 initrd 以及内核命令行。Bootimus 直接启动 UKI,无需提取。

**添加 UKI**:上传 `.efi` 文件、从 URL 下载,或复制到 `/data/isos/` 后扫描,方式与 ISO 完全相同。Bootimus 只登记属于 UKI 的 `.efi` 文件(带有 `.linux` 段的 EFI 可执行文件);其他 EFI 程序会被拒绝,ISO 提取目录中的文件也会被忽略。

**启动参数**:UKI 的 `.cmdline` 段中内嵌的命令行会在首次添加时复制到镜像的启动参数中,因此你可以在镜像属性中查看并修改它。Bootimus 使用以下方式启动该镜像:

```
kernel http://<server>:8080/isos/<image>.efi <boot parameters>
```

关闭 Secure Boot 时,这些参数会替换内嵌的命令行,所以内嵌命令行会被预先填入。开启 Secure Boot 时,大多数 UKI 内的 systemd-stub 会忽略这些参数,始终使用内嵌的命令行。

**仅限 UEFI**:UKI 是 EFI 程序,传统 BIOS 客户端无法启动它们。在 BIOS 客户端上选择 UKI 时,会显示需要 UEFI 的提示。

## OpenBSD 与 NetBSD

OpenBSD 和 NetBSD 不是从 Linux 风格的内核和 initrd 引导的。它们各自自带第二阶段网络加载程序,而该加载程序只会说 TFTP:它从 TFTP 服务器的根目录获取配置文件和安装内核。Bootimus 通过从安装 ISO 中提取这些文件并经由自己的 TFTP 服务器提供它们来支持两者,因此客户端必须能访问 TFTP 端口(PXE 本身已经需要这一点)。

**添加镜像**:OpenBSD 上传 `install77.iso`(或 `cd77.iso`),NetBSD 上传 `NetBSD-10.1-amd64.iso`,然后像其他 ISO 一样运行 **Extract Kernel/Initrd**。提取会把加载程序和安装内核复制到 ISO 旁边的 `bsd/` 文件夹中,并提取完整的 ISO 目录树,以便安装程序可以从 `http://<server>:8080/boot/<image>/iso/` 获取安装集。

| 发行版 | 从 ISO 中提取的加载程序 | 内核 |
|--------|--------------------------|------|
| OpenBSD | 来自 `/<version>/<arch>/` 的 `pxeboot`(BIOS)、`BOOTX64.EFI`、`BOOTIA32.EFI`、`BOOTAA64.EFI`(UEFI) | `bsd.rd` |
| NetBSD | 来自 `/<arch>/installation/misc/` 的 `pxeboot_ia32.bin`(BIOS),以及 ISO 自带时的 `bootx64.efi`(UEFI) | `netbsd-INSTALL.gz` |

**引导的工作方式**:菜单条目会链式加载 HTTP 端口上的一个小 iPXE 脚本,该脚本记录下这个客户端(按 IP 地址)选择了该镜像,然后从 `bsd/<image>/` 经 TFTP 链式加载对应的加载程序。在接下来的 15 分钟内,来自该 IP、针对加载程序在 TFTP 根目录寻找的那些文件(OpenBSD 的 `etc/boot.conf`、`etc/random.seed`、`bsd.rd`、`bsd`;NetBSD 的 `boot.cfg`、`EFI/NetBSD/boot.cfg`、`netbsd` 和 `netbsd-INSTALL.gz`)的请求都会由该镜像应答。配置文件即时生成,并把加载程序指向 `bsd/<image>/` 下的内核,因此显式路径同样可用。如果 ISO 缺少适用于该客户端固件的加载程序,客户端会看到相应提示并返回菜单。

**串口控制台及其他加载程序设置**:把额外的加载程序配置行放进镜像的 **Boot Parameters**,用分号分隔。对 OpenBSD,它们会追加到生成的 `boot.conf`,例如 `stty com0 115200;set tty com0`。对 NetBSD,它们会追加到生成的 `boot.cfg`,例如 `consdev=com0`。

**已知限制**:

- 此支持是新加入的,尚未在真实硬件上验证。请反馈哪些可用、哪些不可用。
- NetBSD 的 `pxeboot_ia32.bin` 加载后会自行进行一次 DHCP 交换,并读取 DHCP 的 `filename` 字段来决定使用 NFS 还是 TFTP。如果那里没有 `tftp:` 前缀,它默认使用 NFS,而 Bootimus 并不为此提供 NFS。如果加载程序报告无法打开 `boot.cfg` 或内核,请在其提示符下输入 `boot tftp:bsd/<image>/netbsd-INSTALL.gz`,或在你的 DHCP 服务器上给 NetBSD 客户端设置 `filename "tftp:boot.cfg";`。OpenBSD 的加载程序始终使用 TFTP,无需这一步。
- 按客户端的关联以 IP 地址为键,因此同一 NAT 地址后的两台客户端如果在 15 分钟内引导不同的 BSD 镜像,会收到相同的配置。
- 尚未接入 BSD 安装程序的自动安装。安装程序仍然可以通过 HTTP 从提取出的 ISO 目录树获取安装集。

## WinPE 与救援介质

Bootimus 通过 [wimboot](https://ipxe.org/wimboot) 引导 Windows 安装程序,同一路径现在也涵盖基于 WinPE 的救援介质,例如 Sergei Strelec 的 WinPE、Active@ Boot Disk 和 Hiren's BootCD PE。

**支持的布局**:任何带有 `BCD`(位于 `boot/` 或 `efi/microsoft/boot/` 下)、`boot/boot.sdi` 以及至少一个 `.wim` 文件的 ISO。安装介质还会有 `sources/install.wim` 或 `install.esd`;没有它的介质会被归类为 **WinPE / Rescue Media** 配置(`winpe`)而不是 **Windows**,因此不会对其应用自动安装或 SMB 处理。前四级目录中任何位置的 WIM 文件都能被找到,所以 Strelec 的 `SSTR/*.wim` 及类似布局无需重命名任何东西即可使用。

**选择 WIM**:提取后,镜像属性面板会列出找到的每个 WIM 及其大小。选择 wimboot 应加载的那个,如果一个 WIM 包含多个映像,再选择映像索引。只有索引大于一时才会把它加到 wimboot 行上,引导参数中显式的 `index=` 优先。同样的设置也可以通过 API 完成:

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"wim_path":"SSTR/strelec10x64Eng.wim","wim_index":1}' \
  "http://localhost:8081/api/images/winpe?filename=WinPE_Strelec.iso"
```

**引导方式**:只有所选的 WIM 会发送给客户端;wimboot 自行合成 BCD 和 `boot.sdi`,这避免了某些介质在使用 DVD 的 BCD 时出现的黑屏。引导参数仅限 wimboot 自己的选项(`rawbcd`、`rawwim`、`gui`、`linear`、`quiet`、`pause`、`index`、`initrdfile`),与 Windows 安装程序完全一致。

**限制**:

- 救援环境期望在自己的引导介质上找到的程序(例如 Strelec 的 `SSTR` 便携工具文件夹)不在 WIM 内,因此 PXE 引导后它们会缺失,除非该环境能自行通过网络获取。核心的 WinPE 桌面以及内置于 WIM 中的工具可以正常使用。
- 需要自带 BCD 条目的介质(自定义的 `bootmgr` 菜单、多 WIM 选择菜单)会直接引导进入所选的 WIM,而不显示那个菜单。
- 目前还不能单独上传裸 `.wim` 文件;请把它们放到带有 `BCD` 和 `boot.sdi` 的 ISO 上,或使用完整的救援 ISO。

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
| **TrueNAS SCALE** |  是 |  否 | 25.10.2.1：UEFI QEMU 安装及磁盘启动；下载完整 ISO |
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
/TrueNAS-SCALE.update
```

Bootimus 通过 `TrueNAS-SCALE.update` 文件识别 SCALE，并选择专用配置文件。照常提取内核/initrd；配置文件会下载完整 ISO，并将其挂载到 `/cdrom` 供安装程序使用。ISO 无需修改，安装程序须包含 systemd 254 或更高版本。

已使用 SCALE 25.10.2.1 在配备 8 GiB 内存的 UEFI QEMU 中验证：安装到新建虚拟磁盘后，从该磁盘启动。安装期间，完整 ISO 保留在内存中。其他版本、物理硬件和 Secure Boot 尚未测试；此配置文件不适用于基于 FreeBSD 的 TrueNAS CORE。

对于此前已提取的镜像，请重新提取以替换旧的 Debian 参数，然后检查自定义启动参数。

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
