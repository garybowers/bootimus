#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
OUTPUT_DIR="$PROJECT_ROOT/bootloaders"
BUILD_DIR="/tmp/bootimus-thinos-build"

echo "Building Bootimus Thin OS..."
echo "Output directory: $OUTPUT_DIR"

mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

KERNEL_VERSION="6.12.8"
BUSYBOX_VERSION="1.36.1"

echo "==> Downloading kernel source..."
if [ ! -f "linux-${KERNEL_VERSION}.tar.xz" ]; then
    wget "https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-${KERNEL_VERSION}.tar.xz"
fi

echo "==> Downloading busybox source..."
if [ ! -f "busybox-${BUSYBOX_VERSION}.tar.bz2" ]; then
    wget "https://busybox.net/downloads/busybox-${BUSYBOX_VERSION}.tar.bz2"
fi

echo "==> Extracting kernel..."
tar -xf "linux-${KERNEL_VERSION}.tar.xz"
cd "linux-${KERNEL_VERSION}"

echo "==> Configuring kernel (minimal config)..."
make defconfig HOSTCFLAGS="-std=gnu11" KCPPFLAGS="-std=gnu11"
scripts/config --enable CONFIG_OVERLAY_FS
scripts/config --enable CONFIG_SQUASHFS
scripts/config --enable CONFIG_ISO9660_FS
scripts/config --enable CONFIG_UDF_FS
scripts/config --enable CONFIG_JOLIET
scripts/config --enable CONFIG_ZISOFS
scripts/config --enable CONFIG_BLK_DEV_LOOP
scripts/config --enable CONFIG_BLK_DEV_RAM
scripts/config --enable CONFIG_DEVTMPFS
scripts/config --enable CONFIG_DEVTMPFS_MOUNT
scripts/config --enable CONFIG_E1000
scripts/config --enable CONFIG_E1000E
scripts/config --enable CONFIG_VIRTIO_NET
scripts/config --enable CONFIG_KEXEC
scripts/config --enable CONFIG_NET
scripts/config --enable CONFIG_INET
scripts/config --enable CONFIG_IP_PNP
scripts/config --enable CONFIG_IP_PNP_DHCP
scripts/config --enable CONFIG_EFI
scripts/config --enable CONFIG_EFI_STUB
scripts/config --enable CONFIG_EFI_VARS
scripts/config --enable CONFIG_FB_EFI
scripts/config --enable CONFIG_FRAMEBUFFER_CONSOLE
scripts/config --enable CONFIG_SERIAL_8250
scripts/config --enable CONFIG_SERIAL_8250_CONSOLE
scripts/config --enable CONFIG_EFI_EARLY_PRINTK
scripts/config --enable CONFIG_EARLY_PRINTK
scripts/config --enable CONFIG_VGA_CONSOLE
scripts/config --enable CONFIG_BINFMT_ELF
scripts/config --enable CONFIG_BINFMT_SCRIPT
scripts/config --enable CONFIG_X86_MSR
scripts/config --enable CONFIG_X86_CPUID
scripts/config --enable CONFIG_PRINTK
scripts/config --enable CONFIG_TTY
scripts/config --enable CONFIG_VT
scripts/config --enable CONFIG_VT_CONSOLE
scripts/config --enable CONFIG_HW_CONSOLE
scripts/config --enable CONFIG_UNIX
scripts/config --enable CONFIG_TMPFS
scripts/config --enable CONFIG_SYSFS
scripts/config --enable CONFIG_PROC_FS
make olddefconfig HOSTCFLAGS="-std=gnu11" KCPPFLAGS="-std=gnu11"

echo "==> Building kernel..."
make -j$(nproc) bzImage HOSTCFLAGS="-std=gnu11" KCPPFLAGS="-std=gnu11" KCFLAGS="-Wno-error=unterminated-string-initialization" CC="gcc -std=gnu11"

cp arch/x86/boot/bzImage "$OUTPUT_DIR/thinos-kernel"
echo "Kernel built: $OUTPUT_DIR/thinos-kernel"

cd "$BUILD_DIR"

echo "==> Extracting busybox..."
tar -xf "busybox-${BUSYBOX_VERSION}.tar.bz2"
cd "busybox-${BUSYBOX_VERSION}"

echo "==> Configuring busybox (static)..."
make defconfig
sed -i 's/# CONFIG_STATIC is not set/CONFIG_STATIC=y/' .config
sed -i 's/CONFIG_TC=y/# CONFIG_TC is not set/' .config

echo "==> Building busybox..."
make -j$(nproc)
make install

echo "==> Creating initramfs..."
INITRAMFS_DIR="$BUILD_DIR/initramfs"
mkdir -p "$INITRAMFS_DIR"
cd "$INITRAMFS_DIR"

mkdir -p bin sbin etc proc sys tmp dev mnt/iso mnt/root newroot

cp -a "$BUILD_DIR/busybox-${BUSYBOX_VERSION}/_install/"* .

cat > init <<'INIT_SCRIPT'
#!/bin/sh

mount -t proc none /proc
mount -t sysfs none /sys
mount -t devtmpfs none /dev

echo "Bootimus Thin OS - ISO Loader"
echo "=============================="
echo ""

# Find first available network interface
echo "Finding network interface..."
for iface in $(ls /sys/class/net/ | grep -v lo); do
    echo "Found interface: $iface"
    NETIF=$iface
    break
done

if [ -z "$NETIF" ]; then
    echo "ERROR: No network interface found"
    exec /bin/sh
fi

echo "Configuring network on $NETIF..."
ip link set lo up
ip link set $NETIF up

# Wait for link
sleep 2

mkdir -p /tmp/udhcpc
udhcpc -i $NETIF -s /usr/share/udhcpc/default.script -q -n 2>&1

# Debug: Show IP config
echo ""
echo "Network configuration:"
ip addr show $NETIF
ip route show
echo ""

# If BOOTIMUS_SERVER not passed as kernel param, try to get it from DHCP next-server
if [ -z "$BOOTIMUS_SERVER" ]; then
    # Try to extract from /tmp/udhcpc info if available
    if [ -f /tmp/udhcpc/siaddr ]; then
        BOOTIMUS_SERVER=$(cat /tmp/udhcpc/siaddr)
        echo "Got server from DHCP siaddr: $BOOTIMUS_SERVER"
    fi
fi

# If still empty, try default gateway as fallback
if [ -z "$BOOTIMUS_SERVER" ]; then
    BOOTIMUS_SERVER=$(ip route | grep default | awk '{print $3}')
    echo "Using default gateway as server: $BOOTIMUS_SERVER"
fi

if [ -z "$BOOTIMUS_SERVER" ]; then
    echo "ERROR: Could not determine boot server"
    echo "BOOTIMUS_SERVER env var is not set"
    exec /bin/sh
fi

echo "Boot server: $BOOTIMUS_SERVER"
echo "ISO name: $ISO_NAME"
echo ""

ISO_URL="${ISO_URL:-http://${BOOTIMUS_SERVER}:8080/isos/${ISO_NAME}}"
echo "Downloading ISO: $ISO_URL"

wget -O /tmp/install.iso "$ISO_URL" || {
    echo "ERROR: Failed to download ISO"
    exec /bin/sh
}

echo "Mounting ISO..."
mount -o loop,ro /tmp/install.iso /mnt/iso || {
    echo "ERROR: Failed to mount ISO"
    exec /bin/sh
}

echo "ISO mounted successfully"
ls -la /mnt/iso

echo "Detecting boot method..."

if [ -f /mnt/iso/casper/vmlinuz ]; then
    echo "Ubuntu/Debian live detected"
    KERNEL=/mnt/iso/casper/vmlinuz
    INITRD=/mnt/iso/casper/initrd
    APPEND="boot=casper iso-scan/filename=/tmp/install.iso"
elif [ -f /mnt/iso/arch/boot/x86_64/vmlinuz-linux ]; then
    echo "Arch Linux detected"
    KERNEL=/mnt/iso/arch/boot/x86_64/vmlinuz-linux
    INITRD=/mnt/iso/arch/boot/x86_64/initramfs-linux.img
    APPEND="archisobasedir=arch archisolabel=ARCH_$(date +%Y%m)"
elif [ -f /mnt/iso/images/pxeboot/vmlinuz ]; then
    echo "Fedora/RHEL detected"
    KERNEL=/mnt/iso/images/pxeboot/vmlinuz
    INITRD=/mnt/iso/images/pxeboot/initrd.img
    APPEND="inst.stage2=/mnt/iso"
else
    echo "ERROR: Unknown ISO format"
    echo "Dropping to shell for manual boot"
    exec /bin/sh
fi

echo "Kernel: $KERNEL"
echo "Initrd: $INITRD"
echo "Append: $APPEND"

echo "Executing kexec..."
kexec -l "$KERNEL" --initrd="$INITRD" --append="$APPEND" || {
    echo "ERROR: kexec failed"
    exec /bin/sh
}

umount /proc
umount /sys

kexec -e
INIT_SCRIPT

chmod +x init

mkdir -p usr/share/udhcpc
cat > usr/share/udhcpc/default.script <<'DHCP_SCRIPT'
#!/bin/sh
[ -z "$1" ] && echo "Error: should be called from udhcpc" && exit 1

case "$1" in
    deconfig)
        ip addr flush dev $interface
        ip link set $interface up
        ;;
    bound|renew)
        ip addr add $ip/$mask dev $interface
        [ -n "$router" ] && ip route add default via $router dev $interface
        [ -n "$domain" ] && echo "search $domain" > /etc/resolv.conf
        for i in $dns; do
            echo "nameserver $i" >> /etc/resolv.conf
        done
        # Save DHCP next-server (siaddr) for boot server detection
        mkdir -p /tmp/udhcpc
        [ -n "$siaddr" ] && echo "$siaddr" > /tmp/udhcpc/siaddr
        [ -n "$boot_file" ] && echo "$boot_file" > /tmp/udhcpc/boot_file
        ;;
esac
exit 0
DHCP_SCRIPT

chmod +x usr/share/udhcpc/default.script

echo "==> Packing initramfs..."
find . | cpio -H newc -o | gzip > "$OUTPUT_DIR/thinos-initrd.gz"

echo ""
echo "Build complete!"
echo "Kernel: $OUTPUT_DIR/thinos-kernel"
echo "Initrd: $OUTPUT_DIR/thinos-initrd.gz"
echo ""
echo "To rebuild, run: $0"
