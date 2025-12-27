# Windows Installation Support

## Important: Windows vs Linux Network Boot

**Linux ISOs** can be directly booted via iPXE's `sanboot` command (the current Bootimus implementation).

**Windows ISOs** cannot be directly booted this way. Windows network installation requires a different approach.

## Current Status

Bootimus currently supports:
- ✅ Linux ISO direct boot (Ubuntu, Debian, Fedora, etc.)
- ❌ Windows ISO direct boot (not supported by iPXE)

## Windows Network Installation Methods

### Method 1: Windows Deployment Services (WDS)

Use Microsoft's official solution:
- Requires Windows Server
- Full PXE boot support
- Complex setup
- Not compatible with Bootimus

### Method 2: WinPE + wimboot (Recommended for Bootimus)

This method can work with Bootimus but requires additional setup:

#### Requirements:
1. Windows ISO file
2. wimboot (iPXE Windows bootloader)
3. Extract BCD, boot.sdi, and boot.wim from Windows ISO

#### Setup Steps:

**1. Download wimboot:**
```bash
mkdir -p boot/windows
wget -O boot/windows/wimboot https://github.com/ipxe/wimboot/releases/latest/download/wimboot
```

**2. Extract Windows boot files from ISO:**
```bash
# Mount Windows ISO
mkdir -p /mnt/windows-iso
mount -o loop windows-server-2022.iso /mnt/windows-iso

# Copy required files
mkdir -p data/windows/server2022
cp /mnt/windows-iso/boot/bcd data/windows/server2022/
cp /mnt/windows-iso/boot/boot.sdi data/windows/server2022/
cp /mnt/windows-iso/sources/boot.wim data/windows/server2022/

# Unmount
umount /mnt/windows-iso
```

**3. Modify iPXE menu generation** (requires code changes):

Add to `internal/server/server.go` in the iPXE menu template:

```
:windows_server_2022
echo Booting Windows Server 2022...
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/windows/wimboot
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/windows/server2022/bcd BCD
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/windows/server2022/boot.sdi boot.sdi
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/windows/server2022/boot.wim boot.wim
boot
```

### Method 3: HTTP Boot + grub2

More complex but works for both Linux and Windows:
- Requires UEFI HTTP Boot support
- Use grub2 instead of iPXE
- Significant code changes required

## Limitations

**Why Windows is difficult:**
1. Windows boot files are fragmented (BCD, boot.sdi, boot.wim, install.wim)
2. Cannot use `sanboot` like Linux ISOs
3. Requires Windows PE environment
4. License activation issues with network deployment
5. Drivers must be injected for network boot

**File sizes:**
- Linux ISO: 1-4 GB (boots directly)
- Windows extraction: 5-10 GB (boot.wim + install.wim)

## Recommended Approach

**For mixed Linux/Windows environment:**

1. **Use Bootimus for Linux installations**
   - Works perfectly out of the box
   - Just drop ISO files in data directory

2. **Use separate Windows Deployment Server (WDS) for Windows**
   - Microsoft's official solution
   - Better Windows integration
   - Handles drivers and licensing

3. **Or: Extend Bootimus with wimboot support**
   - Requires code changes to menu generation
   - Requires Windows boot file extraction
   - More maintenance overhead

## Future Enhancement

If Windows support is critical, I can add:
1. Windows boot file extraction and serving
2. Modified iPXE menu with wimboot support
3. Admin interface for Windows image management
4. Separate menu entries for Windows vs Linux

This would require:
- New image type field (ISO vs Windows)
- Different boot commands per type
- Windows boot file extraction logic
- Additional storage for extracted files

Would you like me to implement full Windows support? It's a significant feature addition.

## Quick Start (Linux Only)

For now, stick with Linux ISOs:

```bash
# Download bootloaders
./download-bootloaders.sh

# Add Linux ISOs to data directory
mkdir -p data
# Copy your Ubuntu/Debian/Fedora ISOs here

# Start server
./bootimus serve

# Boot clients will see menu with Linux options
```

Linux ISOs that work great:
- Ubuntu Server (live ISO)
- Debian (netinst or live)
- Fedora Workstation/Server
- Rocky Linux
- AlmaLinux
- Arch Linux
- Any other live/installer ISO
