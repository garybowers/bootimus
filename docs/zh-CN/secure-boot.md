# UEFI Secure Boot

如何在不改动固件设置的情况下,网络引导启用了 UEFI Secure Boot 的机器。

## 目录

- [工作原理](#工作原理)
- [启用 Secure Boot 支持](#启用-secure-boot-支持)
- [引导镜像](#引导镜像)
- [哪些能用、哪些不能用](#哪些能用哪些不能用)
- [高级:NBD/NFS 的自签名 bootloader](#高级nbdnfs-的自签名-bootloader)
- [故障排查](#故障排查)

## 工作原理

Bootimus 内置了一个名为 `secureboot-official` 的 bootloader 集。它只包含官方签名的正式发布二进制,因此引导链上的每一环都能用出厂固件本就信任的密钥完成校验 — 无需注册证书,也无需在客户端上改动固件:

```
Firmware (Secure Boot on, stock Microsoft keys)
  → ipxe-shimx64.efi     Microsoft-signed shim from the iPXE release
    → ipxe.efi           signed by the iPXE project's Secure Boot CA,
                         which the shim carries as its vendor certificate
      → autoexec.ipxe    fetched over TFTP; Bootimus generates it per client
        → boot menu      as normal
```

当你引导一个已提取的镜像时,生成的菜单条目会加载 kernel 和 initrd,然后用 iPXE 的 `shim` 命令从提取出的 ISO 中链式加载**发行版自带的微软签名 shim**。该 shim 校验发行版 kernel 的签名,从而完成整条链:

```ipxe
kernel http://server:8080/boot/ubuntu-26.04/vmlinuz ...
initrd http://server:8080/boot/ubuntu-26.04/initrd
iseq ${platform} efi && shim http://server:8080/boot/ubuntu-26.04/iso/EFI/boot/bootx64.efi ||
boot
```

Bootimus 在 ISO 提取过程中自动检测 shim。`iseq … ||` 守卫使这一行在 BIOS 客户端以及不带 `shim` 命令的 iPXE 构建上成为空操作,而且只有当 Secure Boot 实际强制执行时,iPXE 才会调用已加载的 shim — 关闭了 Secure Boot 的机器的引导流程和以前完全一致。

## 启用 Secure Boot 支持

1. 在管理 UI 中打开 **Bootloaders**,选择 **`secureboot-official`** 作为激活集(或在客户端的编辑表单上按客户端设置)。
2. 如果你使用内建 proxyDHCP,到此就完成了 — 它会自动向 UEFI 客户端通告 `ipxe-shimx64.efi`。
3. 如果你运行外部 DHCP 服务器,把它的 UEFI bootfile 名称改为 `ipxe-shimx64.efi`(ARM64 客户端用 `ipxe-shimaa64.efi`)。参见 [DHCP 指南](dhcp.md)。

官方 iPXE 二进制不内嵌 Bootimus 脚本;它们依赖 iPXE ≥ 2.0 通过 TFTP 获取 `autoexec.ipxe`,由 Bootimus 提供。请保证客户端能访问 UDP 端口 69。

## 引导镜像

对 Secure Boot 客户端,请为你的镜像使用**提取(kernel/initrd)引导** — 上传 ISO 后在镜像列表中点击 **Extract**。提取过程中,Bootimus 会记录 ISO 的 `EFI/BOOT/BOOTX64.EFI`(或 `BOOTAA64.EFI`)shim,并自动接入菜单。

在 Secure Boot 支持加入之前提取的镜像需要重新提取才能获得其 shim:删除该镜像的 boot 文件夹后再次提取。

Windows 安装通过官方微软签名的 `wimboot` 引导,`secureboot-official` 集已包含它;此后 wimboot 从 `boot.wim` 加载的所有内容都带微软签名。

## 哪些能用、哪些不能用

| 场景 | Secure Boot 下 |
|----------|------------------|
| 从支持 Secure Boot 的发行版(Ubuntu、Debian、Fedora、openSUSE、Rocky、Alma 等)提取的 Linux ISO | ✅ 可用 |
| Windows 安装器(wimboot) | ✅ 可用 |
| 自身介质不支持 Secure Boot 的发行版(Arch、Alpine、Void 等) | ❌ 它们的 kernel 未签名 — 和用它们的 U 盘引导时一样。对这些请关闭 Secure Boot。 |
| `sanboot`(整个 ISO)引导 | ❌ 请改用提取引导 |
| NBD / NFS 引导方式 | ❌ 它们引导的是 Bootimus 自己的未签名 kernel — 见[下文](#高级nbdnfs-的自签名-bootloader) |
| 工具镜像(ShredOS、memtest 等) | ❌ Kernel 未签名 |

## 高级:NBD/NFS 的自签名 bootloader

NBD 和 NFS 引导方式加载的是 Bootimus 自己的引导环境 kernel,没有任何公共 CA 会为它签名。如果你需要在 Secure Boot 下使用它们,请构建一个自签名集,并在每台客户端上一次性注册你的证书:

```bash
scripts/generate-signing-key.sh
BOOTIMUS_SIGNING_KEY=… BOOTIMUS_SIGNING_CERT=… scripts/build-secureboot-set.sh
```

把输出复制到 `<data>/bootloaders/secureboot/`,在 UI 中选中它,然后在每台客户端上用 `mokutil --import bootimus-signing.crt`(或通过固件的密钥管理)注册证书。对大多数部署而言,在受影响的机器上关闭 Secure Boot 才是更简单的答案。

## 故障排查

- **iPXE 加载之前固件就弹出 "Security Violation" 画面** — 客户端没有获取到 `ipxe-shimx64.efi`。检查你的 DHCP bootfile 名称,并确认 `secureboot-official` 集处于激活状态。
- **iPXE 加载了但没有出现菜单** — 官方二进制需要通过 TFTP 获取 `autoexec.ipxe`。检查客户端与服务器之间的 UDP 69。
- **引导镜像时 iPXE 日志里出现一行 "Security Violation"** — 预期且无害;那是 iPXE 在探测 shim 镜像,不是失败。
- **某些硬件上菜单按键无响应** — iPXE v2.0.0 在部分机器的交互式菜单上有已知的键盘回归问题。可设置默认镜像或使用 next-boot 选择作为变通;如果该客户端不需要 Secure Boot,也可以把它切换到 `default` 集。
- **Kernel 在 Secure Boot 下拒绝加载** — 该镜像很可能是在 shim 支持加入之前提取的(重新提取即可),或者该发行版根本不支持 Secure Boot。
