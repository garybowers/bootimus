# Bootimus USB-Appliance

Ein flashbares, eigenständiges Alpine-Linux-Image, das in einen einsatzbereiten Bootimus-PXE-Server bootet. In einen Switch stecken, Strom dran — und jede Maschine in derselben Broadcast-Domäne kann per PXE dagegen booten. Keine DHCP-Umkonfiguration, keine OS-Installation, kein Setup.

## Was drin ist

- **Alpine Linux** (minimal, ~100 MB Basis)
- **bootimus** mit standardmäßig aktiviertem proxyDHCP
- **Samba** stellt `/var/lib/bootimus/isos` schreibgeschützt als `\\BOOTIMUS\isos` bereit — für Windows-Installer, die während Setup SMB-Zugriff wollen
- **dnsmasq**-Paket verfügbar, aber deaktiviert (das eingebaute proxyDHCP von bootimus deckt das standardmäßig ab)
- **SSH-Server** für Remote-Admin

## Image bauen

Voraussetzungen auf dem Build-Host:
- Docker (mit verfügbarem `--privileged`)
- Go 1.24+ für das Cross-Kompilieren von bootimus
- ~3 GB freier Speicherplatz

Der Build läuft komplett innerhalb eines privilegierten Alpine-Containers — keine Host-Kernel-Module werden geladen, keine Tools auf deiner Maschine installiert.

```bash
make appliance
```

Erzeugt `appliance/build/bootimus-appliance.img` — ein einfaches Disk-Image, bereit zum Flashen mit Etcher, Rufus oder `dd`.

## Auf einen USB-Stick flashen

**Identifiziere dein Zielgerät sorgfältig** — `dd` überschreibt ohne Nachfrage.

```bash
lsblk                                   # finde deinen USB-Stick, z.B. /dev/sdb
sudo dd if=appliance/build/bootimus-appliance.img \
        of=/dev/sdX bs=4M conv=fsync status=progress
sync
```

Unter macOS/Windows funktionieren [Etcher](https://etcher.balena.io) oder [Rufus](https://rufus.ie) direkt mit der `.img`-Datei.

## Erster Boot

1. Stecke den USB-Stick in einen beliebigen PC mit Ethernet und kabelgebundenem Netzwerk.
2. Boote vom USB (einmaliges Boot-Menü oder BIOS-Priorität ändern).
3. Alpine bootet, holt sich per DHCP eine IP aus dem LAN und startet bootimus + samba + proxyDHCP.
4. Die Konsole zeigt:

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

5. Öffne die Admin-URL von einer anderen Maschine im LAN. Melde dich als `admin` mit dem ausgegebenen Passwort an.
6. Lade ISOs über das Admin-UI hoch oder scanne sie ein — sie landen in `/var/lib/bootimus/isos` und werden sofort über HTTP *und* die SMB-Freigabe ausgeliefert.

## Vorbehalte und Tradeoffs

- **Nur kabelgebundenes Netzwerk.** Keine WiFi-Treiber-Firmware ist gebündelt. PXE über WiFi auszuliefern ist sowieso eine furchtbare Idee (Broadcast-Flooding + Latenz).
- **UEFI Secure Boot wird unterstützt** — wähle das eingebaute Bootloader-Set `secureboot-official` aus, und Zielmaschinen mit aktiviertem Secure Boot booten ohne Firmware-Änderungen übers Netzwerk, genau wie bei einer regulären bootimus-Installation. Siehe den [Secure-Boot-Leitfaden](secure-boot.md) für Einschränkungen (NBD-/NFS-Boots und unsignierte Distributionen brauchen weiterhin deaktiviertes Secure Boot).
- **Eine einzige Partition.** ISOs liegen auf derselben Root-Partition wie Alpine. Ein 32-GB-Stick gibt dir ~29 GB für ISOs. Für eine größere Library erweitere die Root-Partition manuell nach dem ersten Boot (`resize2fs /dev/sda1`) oder baue mit `IMAGE_SIZE=16G make appliance` neu.
- **proxyDHCP-Koexistenz.** Wenn das LAN, in das du dich einklinkst, schon einen dnsmasq/ISC-proxyDHCP hat, der PXE ausspielt, prügeln sich zwei Proxies. Schalte einen ab: entweder setze `BOOTIMUS_PROXY_DHCP_ENABLED=false` in `/etc/conf.d/bootimus` oder schalte den anderen aus.
- **Die Appliance ist zustandsbehaftet.** Der USB-Stick IST der Server. ISOs, Clients, Pläne und Einstellungen liegen dauerhaft auf ihm. Wenn der Stick mitten im Deploy abraucht, willst du ein Backup haben (`make appliance` erzeugt deterministische Builds, aber deine *Daten* liegen auf dem Stick — nutze regelmäßig den "Download Backup"-Button in den Einstellungen).

## Anpassen

Der Build wird von drei Teilen gesteuert:

- **`appliance/build.sh`** — Orchestrator. Stelle `IMAGE_SIZE` und `ALPINE_BRANCH` als Env-Vars um, ohne Code zu editieren.
- **`appliance/setup.sh`** — läuft beim Build innerhalb des Image-chroot. Füge hier `apk add`-Zeilen ein, um zusätzliches Tooling zu bündeln.
- **`appliance/overlay/`** — jede Datei, die hier abgelegt wird, kommt unverändert ins Rootfs. Übliche Anpassungen:
  - `etc/conf.d/bootimus` — proxyDHCP abschalten, Ports ändern, eine bestimmte Server-IP festnageln
  - `etc/samba/smb.conf` — SMB-Freigabe öffnen, Windows-spezifische Tweaks hinzufügen
  - `etc/network/interfaces` — statische IP statt DHCP
  - `etc/profile.d/bootimus-motd.sh` — Login-Banner austauschen

Nach jeder Änderung `make appliance` erneut ausführen.

## SSH-Zugriff

Root-Login per Passwort ist im Image **deaktiviert** (Security-Hygiene — du wärst schockiert, wie viele "sichere" Appliance-Images mit Default-Credentials ausgeliefert werden). Um Remote-Admin zu aktivieren:

1. Starte die Appliance einmal an der Konsole.
2. Führe `passwd` aus, um ein Root-Passwort zu setzen, ODER legge einen SSH-Key in `/root/.ssh/authorized_keys` ab.
3. `rc-service sshd restart` (SSH ist bereits standardmäßig aktiviert, akzeptiert aber keine passwortlosen Logins).

## Rebuild bei neuem bootimus-Release

Jedes `make appliance` zieht den aktuellen bootimus-Source-Tree. `VERSION` hochziehen, ein Release schneiden, dann das Image neu bauen — die mitgelieferte `bootimus`-Binary meldet die Version, gegen die du gebaut hast, im Footer des Admin-UI.
