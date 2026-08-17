# Bootimus USB 设备镜像

一个可烧录的、自包含的 Alpine Linux 镜像,开机即进入一台开箱即用的 Bootimus PXE 服务器。接到交换机、通电,同一广播域内的每台机器都能通过 PXE 引导 — 无需重新配置 DHCP,无需安装操作系统,无需任何配置。

## 里面有什么

- **Alpine Linux**(最小化,约 100 MB 基础系统)
- **bootimus**,默认启用 proxyDHCP
- **Samba** 以只读方式把 `/var/lib/bootimus/isos` 作为 `\\BOOTIMUS\isos` 共享出来,供安装过程中需要 SMB 访问的 Windows 安装程序使用
- **dnsmasq** 软件包已包含但默认禁用(bootimus 内建的 proxyDHCP 默认已覆盖这部分需求)
- **SSH 服务器** 用于远程管理

## 构建镜像

构建主机的要求:
- Docker(支持 `--privileged`)
- Go 1.24+ 用于交叉编译 bootimus
- 约 3 GB 可用磁盘空间

构建完全在一个特权 Alpine 容器内进行 — 不会加载主机内核模块,也不会在你的机器上安装任何工具。

```bash
make appliance
```

产出 `appliance/build/bootimus-appliance.img` — 一个纯磁盘镜像,可用 Etcher、Rufus 或 `dd` 烧录。

## 烧录到 U 盘

**小心确认目标设备** — `dd` 会直接覆盖,不会提示。

```bash
lsblk                                   # find your USB stick, e.g. /dev/sdb
sudo dd if=appliance/build/bootimus-appliance.img \
        of=/dev/sdX bs=4M conv=fsync status=progress
sync
```

在 macOS/Windows 上,[Etcher](https://etcher.balena.io) 或 [Rufus](https://rufus.ie) 可直接处理 `.img` 文件。

## 首次启动

1. 把 U 盘插到任意带以太网且接入有线网络的 PC。
2. 从 U 盘引导(一次性引导菜单或 BIOS 优先级设置)。
3. Alpine 启动,从局域网 DHCP 获取 IP,启动 bootimus + samba + proxyDHCP。
4. 控制台显示:

   ```
    ____              _   _
   | __ )  ___   ___ | |_(_)_ __ ___  _   _ ___
   |  _ \ / _ \ / _ \| __| | '_ ` _ \| | | / __|
   | |_) | (_) | (_) | |_| | | | | | | |_| \__ \
   |____/ \___/ \___/ \__|_|_| |_| |_|\__,_|___/

     Appliance: bootimus
     Admin UI:  http://10.0.0.42:8081
     PXE HTTP:  http://10.0.0.42:8080
     SMB share: //10.0.0.42/isos  (read-only, guest)
     Initial admin password: <printed once>
     (delete /var/lib/bootimus/admin-password.txt after you've saved it)
   ```

5. 在局域网内任何其他机器上打开管理 URL。用 `admin` 加上打印出来的密码登录。
6. 通过管理 UI 上传或扫描 ISO — 它们会落到 `/var/lib/bootimus/isos`,并立即通过 HTTP *和* SMB 共享对外提供。

## 注意事项与权衡

- **仅支持有线网络。** 不内置 WiFi 驱动固件。通过 WiFi 提供 PXE 本来就是个糟糕主意(广播泛滥 + 延迟)。
- **支持 UEFI Secure Boot** — 选择内置的 `secureboot-official` bootloader 集,开启了 Secure Boot 的目标机器无需改动固件即可网络引导,和常规 bootimus 安装一样。相关限制见 [Secure Boot 指南](secure-boot.md)(NBD/NFS 引导和未签名的发行版仍需关闭 Secure Boot)。
- **单分区。** ISO 和 Alpine 共享同一个根分区。32 GB U 盘大约能放 29 GB 的 ISO。要更大的库,在首次启动后手动扩展根分区(`resize2fs /dev/sda1`),或者用 `IMAGE_SIZE=16G make appliance` 重新构建。
- **proxyDHCP 共存。** 如果你接入的局域网已经有 dnsmasq/ISC proxyDHCP 在广播 PXE,两个 proxy 会打架。禁掉一个:要么在 `/etc/conf.d/bootimus` 中设置 `BOOTIMUS_PROXY_DHCP_ENABLED=false`,要么关掉另一个。
- **设备镜像有状态。** U 盘就是服务器本身。ISO、客户端、计划任务和设置都持久化在上面。如果部署到一半 U 盘挂了,你会希望有备份(`make appliance` 产出可复现构建,但你的*数据*在 U 盘上 — 定期使用 Settings 里的 "Download Backup" 按钮)。

## 自定义

构建由三部分驱动:

- **`appliance/build.sh`** — 编排脚本。无需改代码就能调整 `IMAGE_SIZE` 和 `ALPINE_BRANCH` 环境变量。
- **`appliance/setup.sh`** — 构建期间在镜像 chroot 内运行。在这里加 `apk add` 行可以打包额外工具。
- **`appliance/overlay/`** — 放在这里的任何文件都会原样复制到 rootfs。常见编辑:
  - `etc/conf.d/bootimus` — 关闭 proxyDHCP、修改端口、固定特定服务器 IP
  - `etc/samba/smb.conf` — 扩大 SMB 共享范围,添加 Windows 特有调整
  - `etc/network/interfaces` — 用静态 IP 替代 DHCP
  - `etc/profile.d/bootimus-motd.sh` — 替换登录横幅

任何改动后,重新运行 `make appliance`。

## SSH 访问

镜像中**禁用了** root 密码登录(安全卫生 — 你会惊讶有多少"安全"设备镜像出厂就带默认凭据)。要启用远程管理:

1. 先在控制台启动一次设备镜像。
2. 运行 `passwd` 设置 root 密码,或者把 SSH 公钥放到 `/root/.ssh/authorized_keys`。
3. `rc-service sshd restart`(SSH 默认已启用,但不接受无密码登录)。

## 在新 bootimus 版本上重新构建

每次 `make appliance` 都会取当前 bootimus 源码树。提升 `VERSION`、切出一个 release,然后重新构建镜像 — 内嵌的 `bootimus` 二进制会在管理 UI 页脚显示你构建时所用的版本。
