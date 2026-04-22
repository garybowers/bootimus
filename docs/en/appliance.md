# Bootimus USB Appliance

A flashable, self-contained Alpine Linux image that boots into a ready-to-use Bootimus PXE server. Plug into a switch, power on, and every machine on the same broadcast domain can PXE-boot against it — no DHCP reconfiguration, no OS install, no setup.

## What's inside

- **Alpine Linux** (minimal, ~100 MB base)
- **bootimus** with proxyDHCP enabled by default
- **Samba** serving `/var/lib/bootimus/isos` read-only as `\\BOOTIMUS\isos` for Windows installers that want SMB access during setup
- **dnsmasq** package available but disabled (bootimus's built-in proxyDHCP covers this by default)
- **SSH server** for remote admin

## Building the image

Requirements on the build host:
- Docker (with `--privileged` available)
- Go 1.24+ for cross-compiling bootimus
- ~3 GB free disk space

The build runs entirely inside a privileged Alpine container — no host kernel modules are loaded, no tools installed on your machine.

```bash
make appliance
```

Produces `appliance/build/bootimus-appliance.img` — a plain disk image ready to flash with Etcher, Rufus, or `dd`.

## Flashing to a USB stick

**Identify your target device carefully** — `dd` will overwrite without asking.

```bash
lsblk                                   # find your USB stick, e.g. /dev/sdb
sudo dd if=appliance/build/bootimus-appliance.img \
        of=/dev/sdX bs=4M conv=fsync status=progress
sync
```

On macOS/Windows, [Etcher](https://etcher.balena.io) or [Rufus](https://rufus.ie) work with the `.img` file directly.

## First boot

1. Plug the USB stick into any PC with Ethernet and wired network.
2. Boot from USB (one-time boot menu or BIOS priority change).
3. Alpine boots, DHCPs its own IP from the LAN, and starts bootimus + samba + proxyDHCP.
4. The console shows:

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

5. Open the admin URL from any other machine on the LAN. Log in as `admin` with the printed password.
6. Upload or scan ISOs via the admin UI — they land in `/var/lib/bootimus/isos` and are immediately served over HTTP *and* the SMB share.

## Caveats and tradeoffs

- **Wired network only.** No WiFi driver firmware is bundled. Serving PXE over WiFi is a terrible idea anyway (broadcast-flooding + latency).
- **No UEFI Secure Boot** — the bundled iPXE is unsigned (same as the regular bootimus install, since the Secure Boot shim chain was removed in v0.2.x). Target machines with Secure Boot on need it disabled, or MOK-enrol the iPXE binary.
- **Single partition.** ISOs live on the same root partition as Alpine. A 32 GB stick gives you ~29 GB for ISOs. For a bigger library, extend the root partition manually after first boot (`resize2fs /dev/sda1`) or rebuild with `IMAGE_SIZE=16G make appliance`.
- **proxyDHCP coexistence.** If the LAN you plug into already has a dnsmasq/ISC proxyDHCP advertising PXE, two proxies will fight. Disable one: either set `BOOTIMUS_PROXY_DHCP_ENABLED=false` in `/etc/conf.d/bootimus` or turn off the other.
- **Appliance is stateful.** The USB stick IS the server. ISOs, clients, schedules, and settings persist on it. If the stick dies mid-deploy you'll want a backup (`make appliance` produces deterministic builds but your *data* lives on the stick — use the "Download Backup" button in Settings regularly).

## Customising

The build is driven by three pieces:

- **`appliance/build.sh`** — orchestrator. Tweak `IMAGE_SIZE` and `ALPINE_BRANCH` env vars without editing code.
- **`appliance/setup.sh`** — runs inside the image chroot during build. Add `apk add` lines here to bundle extra tooling.
- **`appliance/overlay/`** — any file placed here is copied into the rootfs as-is. Common edits:
  - `etc/conf.d/bootimus` — turn proxyDHCP off, change ports, pin a specific server IP
  - `etc/samba/smb.conf` — widen the SMB share, add Windows-specific tweaks
  - `etc/network/interfaces` — static IP instead of DHCP
  - `etc/profile.d/bootimus-motd.sh` — swap the login banner

After any change, rerun `make appliance`.

## SSH access

Root login via password is **disabled** in the image (security hygiene — you'd be shocked how many "secure" appliance images ship with default credentials). To enable remote admin:

1. Boot the appliance once at the console.
2. Run `passwd` to set a root password, OR drop an SSH key into `/root/.ssh/authorized_keys`.
3. `rc-service sshd restart` (SSH is already enabled by default but won't accept passwordless logins).

## Rebuilding on a new bootimus release

Every `make appliance` picks up the current bootimus source tree. Bump `VERSION`, cut a release, then rebuild the image — the bundled `bootimus` binary reports the version you built against in the admin UI footer.
