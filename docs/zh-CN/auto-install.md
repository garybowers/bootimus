# 无人值守安装指南

跨 Windows、Ubuntu、Debian 和 Red Hat 系列发行版运行无人值守安装。配置文件丢进去一次,挂到镜像上,之后每次 PXE 引导都能在不按一次键的情况下完成安装。

## 目录

- [概述](#概述)
- [支持的格式](#支持的格式)
- [文件库](#文件库)
- [挂载文件](#挂载文件)
- [解析顺序](#解析顺序)
- [占位符](#占位符)
- [示例](#示例)
- [Windows 注意事项](#windows-注意事项)
- [REST API](#rest-api)
- [故障排查](#故障排查)

## 概述

无人值守安装配置存储在 `data/autoinstall/` 下按发行版分类的文件库中。你可以:

- **在 UI 里管理文件** — 在 **Auto-Install** 标签页中创建、编辑、上传、下载、删除配置,无需触碰文件系统。
- **为镜像设置默认值** — 引导该镜像的每个客户端都得到同一份配置。
- **按客户端覆盖** 或 **按客户端组覆盖** — 当某台机器需要不同的配置时(不同的 hostname、磁盘布局、角色等)。
- **模板化** — 使用类似 `{{HOSTNAME}}` 和 `{{IP}}` 的占位符,在分发时根据正在引导的客户端身份解析。

Bootimus 通过 HTTP 在无人值守安装端点上提供脚本;对 Windows 而言,还会把 `AutoUnattend.xml` 暂存到 SMB 安装共享上,这样 `setup.exe /unattend:` 就能自动取用。

## 支持的格式

| 发行版家族 | 格式 | 文件扩展名 | 识别为 |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| 其他任何 | 原始格式 | 任意 | `generic` |

扩展名同时决定了 UI 内的标签和文件被服务时的 `Content-Type` 头。

## 文件库

所有无人值守安装文件都位于 `data/autoinstall/<distro>/<filename>` 下:

```
data/autoinstall/
├── windows/
│   ├── kiosk.xml
│   └── server-2022.xml
├── ubuntu/
│   ├── default.yaml
│   └── lab-bench.yaml
├── debian/
│   └── server.cfg
└── rocky/
    └── workstation.ks
```

`<distro>` 段必须匹配一个已知的发行版 profile ID(详见 [发行版 Profile](distro-profiles.md))。该目录在首次启动时自动创建。

### 通过 UI 添加文件

**Auto-Install** 标签 → **New File** 打开编辑器,内含发行版选择器、文件名输入框,以及对语法友好的文本区。**Upload File** 接受任意本地文件并放入所选的发行版文件夹。

### 手动添加文件

直接丢进去就行:

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

它们会立即出现在 UI 中 — 无需重启,无需扫描。

## 挂载文件

无人值守安装文件在你将其挂到某处之前不会产生任何效果。有三处可以挂载:

### 镜像(默认值)

**Images** 标签 → 打开镜像的 **Properties** → **Auto-Install** 部分 → 选择一个文件。引导该镜像的每个客户端都会得到这份配置,除非有更具体的覆盖。

### 客户端(按机器覆盖)

**Clients** 标签 → 打开一个客户端 → **Auto-Install File** 下拉框。当某台特定机器需要不同配置时使用(例如,一台构建服务器 vs. 桌面机群的其余机器)。

### 客户端组(按机群覆盖)

**Groups** 标签 → 打开一个组 → **Auto-Install File**。适用于该组内的每个客户端。适合"实验室 3 的所有工作站"这类场景。

## 解析顺序

当客户端请求其无人值守安装文件时,Bootimus 会沿以下层级查找:

```
1. 按客户端覆盖              (Client.AutoInstallFile)
2. 按组覆盖                  (ClientGroup.AutoInstallFile, if client is in a group)
3. 镜像默认值                (Image.AutoInstallFile)
4. 内联旧版脚本              (Image.AutoInstallScript — pre-0.1.58 setups)
5. → 404 (no auto-install configured)
```

第一个非空匹配胜出。端点会记录它服务来源的日志:

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## 占位符

这些占位符会在分发时按客户端替换:

| 占位符 | 替换为 |
|-------|---------------|
| `{{MAC}}` | 客户端 MAC 地址(小写,冒号分隔) |
| `{{CLIENT_NAME}}` | Clients 表中的友好名称 |
| `{{HOSTNAME}}` | 同 `{{CLIENT_NAME}}`(在配置中更清晰的别名) |
| `{{IP}}` | 发起请求的客户端 IP |
| `{{SERVER_ADDR}}` | Bootimus 服务器地址 |
| `{{IMAGE_NAME}}` | 正在引导的镜像的显示名 |
| `{{IMAGE_FILENAME}}` | 正在引导的镜像的 ISO 文件名 |

占位符是纯字符串替换 — 不做转义。请按目标格式(XML、YAML 等)适当地引用它们。

## 示例

### Ubuntu Server (cloud-init)

`data/autoinstall/ubuntu/default.yaml`:

```yaml
#cloud-config
autoinstall:
  version: 1
  identity:
    hostname: {{HOSTNAME}}
    username: ubuntu
    password: "$6$rounds=4096$..."  # mkpasswd -m sha-512
  ssh:
    install-server: true
    allow-pw: false
    authorized-keys:
      - ssh-ed25519 AAAA...
  storage:
    layout:
      name: lvm
  late-commands:
    - curtin in-target -- systemctl enable --now serial-getty@ttyS0.service
```

启动参数(相关镜像默认已为 Ubuntu 设置好这些参数):

```
autoinstall ds=nocloud-net;s=http://{{SERVER_ADDR}}:8080/autoinstall/{{IMAGE_FILENAME}}/
```

### Debian (preseed)

`data/autoinstall/debian/server.cfg`:

```
d-i debian-installer/locale string en_GB.UTF-8
d-i keyboard-configuration/xkb-keymap select gb
d-i netcfg/get_hostname string {{HOSTNAME}}
d-i netcfg/get_domain string lan
d-i partman-auto/method string lvm
d-i partman-auto/choose_recipe select atomic
d-i passwd/root-login boolean false
d-i passwd/user-fullname string Admin
d-i passwd/username string admin
d-i passwd/user-password-crypted password $6$rounds=4096$...
d-i pkgsel/include string openssh-server
d-i grub-installer/bootdev string default
d-i finish-install/reboot_in_progress note
```

### Rocky / Fedora (kickstart)

`data/autoinstall/rocky/workstation.ks`:

```
text
lang en_GB.UTF-8
keyboard gb
timezone Europe/London --utc
network --bootproto=dhcp --hostname={{HOSTNAME}}
rootpw --lock
user --name=admin --groups=wheel --password=$6$rounds=4096$... --iscrypted
sshkey --username=admin "ssh-ed25519 AAAA..."
bootloader --location=mbr
clearpart --all --initlabel
autopart --type=lvm
%packages
@^minimal-environment
openssh-server
%end
```

### Windows 11 / Server (autounattend)

`data/autoinstall/windows/kiosk.xml`:标准 `<unattend>` 文档 — 参见 [微软的 autounattend 参考](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/)。占位符在任何文本节点内都生效:

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## Windows 注意事项

Windows 安装由 SMB 驱动。当某个镜像挂载了 autounattend 文件时,Bootimus 会:

1. 在打补丁 `boot.wim` 时把 `AutoUnattend.xml` 暂存到 SMB 安装共享。
2. 打补丁 `startnet.cmd`,让 WinPE 在引导时把它复制到 `X:\AutoUnattend.xml`(本地 RAM 盘)。
3. 以 `setup.exe /unattend:X:\AutoUnattend.xml` 启动 Setup。

如果镜像没有挂载 autounattend 文件,Setup 会和以前一样以交互模式运行。

**重启韧性。** WinPE 在安装中途会重启并从同一个客户端 IP 重新连接。内置的 Samba 配置设置了 `reset on zero vc = yes` 并禁用了 oplocks,这样第二次 `net use` 不会被陈旧的会话状态卡住。如果你用自己的 `data/smb/smb.conf` 替换了默认配置,请同步这些设置。

**撤销补丁。** 第一次打 SMB 补丁时,会在旁边保留一份原始的 `boot.wim`,命名为 `boot.wim.orig`。**Images** → **移除 SMB 补丁** 会恢复原始文件并删除该镜像的 SMB 共享——之后 ISO 的引导行为与刚上传时完全一致。由旧版 Bootimus 打补丁的镜像没有备份:请先重新提取镜像(会生成一份新的原始副本),再移除补丁。

## REST API

UI 里的每件事也都是一次 REST 调用。

```bash
# List all auto-install files
curl -u admin:pw http://localhost:8081/api/autoinstall-files

# Read a file
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/get?distro=ubuntu&filename=default.yaml"

# Create or overwrite a file
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/save \
  -H "Content-Type: application/json" \
  -d '{"distro":"ubuntu","filename":"default.yaml","content":"#cloud-config\n..."}'

# Upload a file
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/upload \
  -F "distro=windows" \
  -F "filename=kiosk.xml" \
  -F "file=@./kiosk.xml"

# Download
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/download?distro=ubuntu&filename=default.yaml" -o default.yaml

# Delete
curl -u admin:pw -X POST "http://localhost:8081/api/autoinstall-files/delete?distro=ubuntu&filename=default.yaml"
```

将文件挂载到镜像:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

将文件挂载到客户端:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

客户端在启动时访问的无人值守安装端点:

```
GET /autoinstall/<image-filename>/?mac=<mac>
```

`mac` 查询参数由引导菜单自动追加,以便按客户端覆盖能正确解析。

## 故障排查

### `/autoinstall/...` 返回 404

`no auto-install configuration for this image/client` — 解析链的任何层级都没有挂载文件。要么挂载到镜像、客户端或其所属组,要么检查 `auto_install_file` 是否真的指向 `data/autoinstall/` 下存在的某个文件。

### 占位符按字面输出

`{{HOSTNAME}}` 在已安装系统中显示为字面字符串,意味着文件在替换运行之前就被分发了 — 通常是因为客户端仅通过 IP 启动而请求未携带 `mac` 查询参数。确认引导菜单生成的 URL 形如 `/autoinstall/<iso>/?mac=<mac>`。

### 分发了错误的文件

解析是"最具体优先"。如果一个客户端有自己的覆盖而你并没意识到,这就是为什么镜像级默认没有被使用。检查服务器日志这一行:

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

`source:` 字段告诉你究竟是哪个槽位胜出。

### Windows Setup 交互式运行

- 镜像必须挂载有 autounattend 文件(镜像属性 → Auto-Install)。
- 挂载后需要重新打补丁 `boot.wim`:**Images** → **Patch SMB**(或下次启动时会自动重新打补丁)。
- 确认 SMB 共享在客户端可达(在 WinPE 中执行 `net view \\<server>`)。

### "AutoUnattend.xml not on share, running interactive setup"

由 `startnet.cmd` 在文件不在它预期位置时记录。要么暂存步骤失败了(检查打补丁时段附近的 Bootimus 服务器日志),要么 SMB 共享丢了该文件。从镜像属性重新执行 SMB 补丁。

### `net use fails after VM reboot`

在 0.1.58 通过在内置 Samba 配置中启用 `reset on zero vc = yes` 已修复。如果你维护着自定义 `smb.conf`,请加入:

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## 下一步

- 参见 [镜像管理](images.md) 了解如何将文件挂载到镜像。
- 参见 [客户端管理](clients.md) 了解按客户端覆盖。
- 参见 [发行版 Profile](distro-profiles.md) 了解映射到文件库子目录的底层 profile ID。
