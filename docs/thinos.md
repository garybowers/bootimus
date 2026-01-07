# Thin OS (Memdisk) Boot Method

Bootimus Thin OS is a minimal Linux environment that downloads and mounts ISO files, then chainloads into them. This solves compatibility issues with sanboot and provides universal ISO support.

## Benefits

- **Universal compatibility**: Works with any ISO format
- **NVME support**: No driver issues since the thin OS has all drivers
- **UEFI/BIOS**: Works on both firmware types
- **Simplified booting**: No need to extract kernels for every ISO

## How It Works

1. Client boots iPXE and selects an ISO with boot method set to "memdisk"
2. iPXE loads the thin OS kernel and initramfs
3. Thin OS boots, configures network via DHCP
4. Downloads the target ISO from Bootimus server
5. Mounts the ISO as a loop device
6. Detects the boot method (Ubuntu/Arch/Fedora/etc.)
7. Uses kexec to chainload into the ISO's kernel

## Building Thin OS

Run the build script:

```bash
cd scripts
sudo ./build-thinos.sh
```

This will:
- Download and compile a minimal Linux kernel (~6MB)
- Build a static busybox
- Create an initramfs with the boot logic
- Output files to `bootloaders/` directory:
  - `thinos-kernel`
  - `thinos-initrd.gz`

Build time: ~10-15 minutes

## Requirements

- Build tools: `gcc`, `make`, `wget`, `cpio`, `gzip`
- Disk space: ~2GB for build, ~10MB for output
- RAM: 512MB minimum on client machines

## Usage

### Via Admin UI

1. Upload or scan an ISO
2. Set boot method to "memdisk"
3. No extraction needed
4. Boot the client

### Via API

```bash
curl -u admin:password -X PUT \
  http://localhost:8081/api/images/boot-method \
  -H "Content-Type: application/json" \
  -d '{
    "filename": "ubuntu-24.04.iso",
    "boot_method": "memdisk"
  }'
```

## Supported ISOs

The thin OS auto-detects:
- Ubuntu/Debian (casper)
- Arch Linux
- Fedora/RHEL/CentOS
- Generic ISOs (manual mode)

For unsupported ISOs, the thin OS drops to a shell where you can manually boot.

## Troubleshooting

### Build fails
- Ensure you have build tools installed: `gcc`, `make`, `wget`, `cpio`, `gzip`
- Check disk space: needs ~2GB in `/tmp`

### Boot hangs at "Downloading ISO"
- Check network connectivity
- Verify DHCP is working
- Check Bootimus server is accessible

### kexec fails
- ISO may not be bootable via kexec
- Try using "kernel" or "sanboot" boot method instead
- Drop to shell and manually boot

## Customization

Edit `scripts/build-thinos.sh` to:
- Change kernel version
- Add additional drivers
- Modify init script logic
- Add custom tools to initramfs

## Performance

- Boot time: +15-30s vs direct boot (due to ISO download)
- Network bandwidth: Full ISO download required
- RAM usage: ISO size + kernel + initramfs (~200MB + ISO size)

## Future Enhancements

- Caching downloaded ISOs in RAM
- Progress bars during download
- NFS/iSCSI support for large ISOs
- Multi-distro detection improvements
- Windows PE support via GRUB2
