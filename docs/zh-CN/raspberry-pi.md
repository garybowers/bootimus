# Raspberry Pi 网络引导

无需 SD 卡即可网络引导 Raspberry Pi 3 和 4 系列开发板。Bootimus 内嵌了 Pi 固件所需的全部文件,并把开发板直接引入标准的 UEFI/iPXE 引导菜单。

## 目录

- [支持的开发板](#支持的开发板)
- [工作原理](#工作原理)
- [一次性开发板准备](#一次性开发板准备)
- [DHCP 配置](#dhcp-配置)
- [按机器覆盖](#按机器覆盖)
- [限制](#限制)

## 支持的开发板

| 开发板 | 状态 |
|-------|--------|
| Pi 4B、Pi 400、CM4 | ✅ 支持 |
| Pi 3B、3B+、CM3 | ✅ 支持 |
| Pi 5 | ❌ 尚无成熟的 UEFI(EDK2)移植 |
| Pi 2 及更早型号、Zero | ❌ 固件不具备网络引导能力 |

## 工作原理

Pi 的 boot ROM/EEPROM 通过 TFTP 获取固件,而 Bootimus 用内嵌资源提供整条引导链 — 无需准备文件,也无需刷写任何东西:

```
Pi EEPROM/bootcode netboot (no SD card)
  → TFTP: bootcode.bin / start4.elf + fixup + config.txt
    → config.txt [pi3]/[pi4] filters pick the right UEFI build
      → RPI_EFI_3.fd or RPI_EFI_4.fd (Tianocore EDK2)
        → UEFI PXE → ipxe-arm64.efi → Bootimus boot menu
```

从菜单开始,Pi 就只是又一台 ARM64 UEFI 客户端:提取的镜像、按客户端分配镜像、自动安装 — 一切都和 x86 上完全一样。

固件会先在 `<8-hex-serial>/` 路径前缀下请求文件;Bootimus 会自动去掉该前缀,因此整批设备都能从同一套共享文件引导,无需逐板配置。

## 一次性开发板准备

唯一无法由 Bootimus 通过网络完成的,是让 Pi 去*尝试*网络引导 — 这是存储在开发板上的固件策略:

- **Pi 4 / 400 / CM4**:一次性设置 EEPROM 引导顺序,例如通过 `raspi-config`(Advanced Options → Boot Order → Network Boot),或用 `rpi-eeprom-config` 设置 `BOOT_ORDER=0xf12`。较新的 EEPROM 在没有 SD 卡时会自动回退到网络引导。
- **Pi 3B+**:网络引导支持内置在 boot ROM 中 — 无需任何配置;不插 SD 卡直接开机即可。
- **Pi 3B / CM3**:先用一张在 `config.txt` 中写有 `program_usb_boot_mode=1` 的 SD 卡引导一次,以设置(不可逆的)OTP 引导位。

## DHCP 配置

最简单的路径是 Bootimus 内建的 proxyDHCP(`--proxy-dhcp`):它能识别 Raspberry Pi 的 MAC 地址,用固件要求的 `Raspberry Pi Boot` 厂商选项进行应答,然后为 UEFI 阶段提供其 `ipxe-arm64.efi` bootfile — 而你现有的 DHCP 服务器继续照常分配 IP,不受任何影响。

若改用外部 DHCP 服务器,则两个阶段都要覆盖:

1. **固件阶段**需要指向 Bootimus 的 `next-server`(option 66),*以及*包含字符串 `Raspberry Pi Boot` 的厂商 option 43。
2. **UEFI 阶段**需要为客户端架构 11(UEFI ARM64)提供 bootfile `ipxe-arm64.efi`。

许多 DHCP GUI(OPNsense/pfSense 都在其中)无法表达按架构区分的 bootfile 或 Pi 的厂商选项 — 在这类网络上,请同时运行 proxyDHCP。参见 [DHCP 指南](dhcp.md)。

## 按机器覆盖

要给某块特定开发板不同的固件文件或自定义的 `config.txt`,把文件放在数据目录下以该板序列号命名的路径中:

```
<data>/raspberrypi/<serial>/config.txt      # this Pi only
<data>/raspberrypi/config.txt               # all Pis, overrides embedded copy
```

未被覆盖的内容都会回退到内嵌资源。序列号是在 Pi 上执行 `cat /proc/cpuinfo` 得到的 8 位十六进制值,开发板首次尝试网络引导时也会出现在 Bootimus 的 TFTP 日志里。

## 限制

- **仅支持有线以太网** — Pi 固件无法通过 WiFi 网络引导。
- **UEFI 设置不会持久保存。** 网络引导的固件没有可写的变量存储,因此它总是以 pftf 默认值启动 — 包括 Pi 4 上默认的 3 GB 内存限制。如果你的操作系统需要全部内存,请构建一个关闭该限制的 `RPI_EFI.fd`,并将其作为覆盖文件放入。
- **不支持 Pi 5**,直到其 EDK2 移植成熟为止;任何没有 UEFI 固件的开发板同理。
- 这些资源通过 `scripts/build-raspberrypi-assets.sh` 刷新,该脚本会锁定 [pftf](https://github.com/pftf/RPi4) UEFI 发布版并校验其校验和。它打包的 Broadcom 引导 blob 仅授权用于 Raspberry Pi 设备(`LICENCE.broadcom.txt` 随附)。
