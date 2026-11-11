#  Leitfaden zur Image-Verwaltung

Kompletter Leitfaden zur Verwaltung von ISO-Images, zum Extrahieren von Boot-Dateien und zur Behandlung von Sonderfällen wie Debian-/Ubuntu-Netboot.

##  Inhaltsverzeichnis

- [Images hinzufügen](#images-hinzufügen)
- [Prüfsummen und Update-Benachrichtigungen](#prüfsummen-und-update-benachrichtigungen)
- [Unified Kernel Images (UKI)](#unified-kernel-images-uki)
- [OpenBSD und NetBSD](#openbsd-und-netbsd)
- [WinPE und Rettungsmedien](#winpe-und-rettungsmedien)
- [Kernel-Extraktion](#kernel-extraktion)
- [Netboot-Unterstützung](#netboot-unterstützung)
- [Ubuntu-Desktop-Optimierung](#ubuntu-desktop-optimierung)
- [Unterstützte Distributionen](#unterstützte-distributionen)
- [Fehlersuche](#fehlersuche)

## Images hinzufügen

### Upload per Web-Oberfläche

1. Zum Admin-Panel navigieren: `http://your-server:8081`
2. Auf Button **"Upload ISO"** klicken
3. ISO-Datei per Drag & Drop oder Klick auswählen
4. Optional Beschreibung hinzufügen
5. **"Public"** anhaken, um sie für alle Clients verfügbar zu machen
6. **"Upload"** klicken

**Upload-Limits**: 10 GB pro Datei

### Upload per API

```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Von URL herunterladen

ISOs direkt auf den Server laden, ohne lokalen Upload:

**Per Web-Oberfläche**:
1. Auf Button **"Download from URL"** klicken
2. ISO-Download-URL eingeben
3. Beschreibung hinzufügen
4. **"Download"** klicken

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'
```

**Fortschritt überwachen**:
```bash
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Prüfsummen und Update-Benachrichtigungen

**Verifikation beim Download.** Wird ein ISO über **Get Images** (den eingebauten Katalog) geholt, sucht Bootimus daneben nach einer veröffentlichten Prüfsummendatei (`SHA256SUMS`, `sha256sum.txt` und ähnliche) und hasht den Download, während er hereinströmt. Bei einer Abweichung schlägt der Download fehl und die Datei wird entfernt, sodass ein abgeschnittenes oder manipuliertes Image nie ins Boot-Menü gelangt. Downloads von einer einfachen URL lassen sich ebenfalls verifizieren, indem du `/api/images/download` ein `sha256` oder eine `checksum_url` mitgibst:

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{"url": "https://releases.ubuntu.com/24.04.4/ubuntu-24.04.4-live-server-amd64.iso",
       "checksum_url": "https://releases.ubuntu.com/24.04.4/SHA256SUMS"}'
```

Verifizierte Images tragen in der Image-Liste ein **✓ Verified**-Badge und zeigen die aufgezeichnete SHA-256 im Eigenschaften-Panel des Images. Images, die ohne veröffentlichte Prüfsumme heruntergeladen oder von Hand hochgeladen wurden, sind als **Unverified** markiert; füge im Eigenschaften-Panel eine SHA-256 oder die URL einer Prüfsummendatei ein und klicke **Verify**, um die Datei im Hintergrund zu hashen (`POST /api/images/verify?filename=…`, dieselbe URL per `GET` abfragen).

**Neuere Releases.** Einmal täglich (und bei **Settings → Housekeeping → Check now** oder `POST /api/catalogue/check`) vergleicht Bootimus die installierten Images mit dem Katalog. Ein aus dem Katalog heruntergeladenes Image wird über Distribution und Release-Linie zugeordnet, sodass `ubuntu-24.04.3-desktop-amd64.iso` markiert wird, sobald `24.04.4` erscheint; Images, die vor dieser Funktion hinzugekommen sind, werden anhand der Form des Dateinamens zugeordnet. Markierte Images tragen ein **Update available**-Badge und einen Button **Download newer version** im Eigenschaften-Panel, Tools zeigen dasselbe Badge mit **Download newer**, und das Dashboard zählt die ausstehenden Updates. Die Prüfung nutzt den Online-Katalog, wenn Remote-Profil-Updates erlaubt sind, und fällt auf den im Binary eingebauten Katalog zurück, wenn `--disable-remote-profiles` gesetzt ist. Entferne den Haken bei **Check the catalogue daily** unter Housekeeping, um sie abzuschalten.

### Mit Ordnern organisieren

ISOs in Unterverzeichnissen werden im Boot-Menü automatisch gruppiert:

```
/data/isos/
├── ubuntu-24.04.iso              # ungruppiert
├── linux/                        # Gruppe "linux"
│   ├── debian-12.iso
│   └── servers/                  # Untergruppe "servers" unter "linux"
│       └── truenas-scale.iso
└── windows/                      # Gruppe "windows"
    └── win11.iso
```

Gruppen werden beim Start und beim Scannen automatisch angelegt. Sie lassen sich auch manuell im Tab Gruppen im Admin-UI verwalten.

### Bestehende ISOs scannen

Wenn du ISOs manuell ins Daten-Verzeichnis (oder Unterverzeichnisse) kopierst:

1. ISO-Dateien ins Verzeichnis `/data/isos/` (oder Unterverzeichnisse) kopieren
2. Auf **"Scan for ISOs"** im Admin-Panel klicken
3. Bootimus erkennt und registriert neue ISOs und legt Gruppen aus Ordnern an

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Unified Kernel Images (UKI)

Ein Unified Kernel Image (UKI) ist eine einzelne EFI-Anwendung, die einen Linux-Kernel, seine initrd und seine Kernel-Kommandozeile bündelt. Bootimus bootet UKIs direkt, ganz ohne Extraktion.

**UKI hinzufügen**: Lade die `.efi`-Datei hoch, lade sie von einer URL herunter oder kopiere sie nach `/data/isos/` und scanne, genau wie bei einer ISO. Bootimus registriert nur `.efi`-Dateien, die UKIs sind (EFI-Anwendungen mit einer `.linux`-Sektion); andere EFI-Programme werden abgelehnt, und Dateien im extrahierten Ordner einer ISO werden ignoriert.

**Boot-Parameter**: Die in der `.cmdline`-Sektion des UKI eingebettete Kommandozeile wird beim ersten Hinzufügen in die Boot-Parameter des Images übernommen, sodass du sie in den Image-Eigenschaften sehen und ändern kannst. Bootimus bootet das Image mit:

```
kernel http://<server>:8080/isos/<image>.efi <boot parameters>
```

Bei deaktiviertem Secure Boot ersetzen diese Parameter die eingebettete Kommandozeile, deshalb ist die eingebettete vorausgefüllt. Bei aktiviertem Secure Boot ignoriert der systemd-stub in den meisten UKIs sie und verwendet immer die eingebettete Kommandozeile.

**Nur UEFI**: UKIs sind EFI-Programme, daher können Legacy-BIOS-Clients sie nicht booten. Wählt man eines auf einem BIOS-Client, erscheint ein Hinweis, dass UEFI nötig ist.

## OpenBSD und NetBSD

OpenBSD und NetBSD booten nicht aus einem Kernel und einer initrd nach Linux-Art. Beide bringen ihren eigenen zweistufigen Netzwerk-Loader mit, und der spricht nur TFTP: er holt seine Konfigurationsdatei und den Installations-Kernel aus dem Wurzelverzeichnis des TFTP-Servers. Bootimus unterstützt beide, indem es diese Dateien aus dem Installations-ISO extrahiert und über seinen TFTP-Server ausliefert; der TFTP-Port muss also für Clients erreichbar sein (für PXE selbst ist er das ohnehin).

**Image hinzufügen**: lade `install77.iso` (oder `cd77.iso`) für OpenBSD bzw. `NetBSD-10.1-amd64.iso` für NetBSD hoch und führe dann wie bei jedem anderen ISO **Extract Kernel/Initrd** aus. Die Extraktion kopiert die Loader und den Installations-Kernel in einen Ordner `bsd/` neben dem ISO und entpackt den vollständigen ISO-Baum, damit der Installer seine Sets von `http://<server>:8080/boot/<image>/iso/` holen kann.

| Distribution | Aus dem ISO übernommene Loader | Kernel |
|--------------|--------------------------------|--------|
| OpenBSD | `pxeboot` (BIOS), `BOOTX64.EFI`, `BOOTIA32.EFI`, `BOOTAA64.EFI` (UEFI) aus `/<version>/<arch>/` | `bsd.rd` |
| NetBSD | `pxeboot_ia32.bin` (BIOS) aus `/<arch>/installation/misc/`, `bootx64.efi` (UEFI), sofern das ISO eines enthält | `netbsd-INSTALL.gz` |

**So läuft der Boot**: der Menüeintrag chainloadet ein kleines iPXE-Skript auf dem HTTP-Port, das festhält, dass dieser Client (per IP-Adresse) dieses Image gewählt hat, und dann den passenden Loader per TFTP aus `bsd/<image>/` chainloadet. In den folgenden 15 Minuten werden Anfragen von dieser IP nach den Dateien, die die Loader im TFTP-Wurzelverzeichnis suchen (`etc/boot.conf`, `etc/random.seed`, `bsd.rd`, `bsd` bei OpenBSD; `boot.cfg`, `EFI/NetBSD/boot.cfg`, `netbsd` und `netbsd-INSTALL.gz` bei NetBSD), aus diesem Image beantwortet. Die Konfigurationsdatei wird on the fly erzeugt und verweist den Loader auf den Kernel unter `bsd/<image>/`, explizite Pfade funktionieren also ebenfalls. Fehlt dem ISO der Loader für die Firmware des Clients, sieht der Client einen entsprechenden Hinweis und kehrt ins Menü zurück.

**Serielle Konsole und andere Loader-Einstellungen**: trage zusätzliche Loader-Konfigurationszeilen, durch Semikolons getrennt, in die **Boot Parameters** des Images ein. Bei OpenBSD werden sie an die erzeugte `boot.conf` angehängt, zum Beispiel `stty com0 115200;set tty com0`. Bei NetBSD werden sie an die erzeugte `boot.cfg` angehängt, zum Beispiel `consdev=com0`.

**Bekannte Einschränkungen**:

- Diese Unterstützung ist neu und noch nicht auf echter Hardware verifiziert. Bitte melde, was funktioniert und was nicht.
- NetBSDs `pxeboot_ia32.bin` führt nach dem Laden einen eigenen DHCP-Austausch durch und liest das DHCP-Feld `filename`, um zwischen NFS und TFTP zu entscheiden. Ohne `tftp:`-Präfix dort fällt es auf NFS zurück, was Bootimus zu diesem Zweck nicht bereitstellt. Meldet der Loader, dass er `boot.cfg` oder den Kernel nicht öffnen kann, tippe an seinem Prompt `boot tftp:bsd/<image>/netbsd-INSTALL.gz` oder gib NetBSD-Clients auf deinem DHCP-Server `filename "tftp:boot.cfg";` mit. OpenBSDs Loader verwenden immer TFTP und brauchen keinen solchen Schritt.
- Die Zuordnung pro Client ist an die IP-Adresse gebunden, zwei Clients hinter derselben NAT-Adresse, die innerhalb von 15 Minuten verschiedene BSD-Images booten, bekämen also dieselbe Konfiguration.
- Autoinstall ist für die BSD-Installer noch nicht angebunden. Die Installer können ihre Sets trotzdem per HTTP aus dem entpackten ISO-Baum holen.

## WinPE und Rettungsmedien

Bootimus bootet Windows-Installer über [wimboot](https://ipxe.org/wimboot), und derselbe Weg deckt jetzt WinPE-basierte Rettungsmedien ab, etwa Sergei Strelecs WinPE, Active@ Boot Disk und Hiren's BootCD PE.

**Unterstützte Layouts**: jedes ISO mit einer `BCD` (unter `boot/` oder `efi/microsoft/boot/`), `boot/boot.sdi` und mindestens einer `.wim`-Datei. Installationsmedien haben zusätzlich `sources/install.wim` oder `install.esd`; Medien ohne eine solche Datei werden als Profil **WinPE / Rescue Media** (`winpe`) statt als **Windows** eingestuft, sodass weder Auto-Install noch SMB-Behandlung auf sie angewandt wird. WIM-Dateien in den ersten vier Verzeichnisebenen werden gefunden, Strelecs `SSTR/*.wim` und ähnliche Layouts funktionieren also, ohne etwas umzubenennen.

**WIM wählen**: nach der Extraktion listet das Eigenschaften-Panel des Images jede gefundene WIM mit ihrer Größe auf. Wähle die, die wimboot laden soll, und, wenn eine WIM mehrere Images enthält, den Image-Index. Der Index wird nur dann an die wimboot-Zeile angehängt, wenn er größer als eins ist, und ein explizites `index=` in den Boot-Parametern hat Vorrang. Dasselbe lässt sich per API setzen:

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"wim_path":"SSTR/strelec10x64Eng.wim","wim_index":1}' \
  "http://localhost:8081/api/images/winpe?filename=WinPE_Strelec.iso"
```

**So bootet es**: nur die gewählte WIM wird an den Client gesendet; wimboot erzeugt BCD und `boot.sdi` selbst, was den schwarzen Bildschirm vermeidet, den manche Medien zeigen, wenn die DVD-BCD verwendet wird. Boot-Parameter beschränken sich auf wimboots eigene Optionen (`rawbcd`, `rawwim`, `gui`, `linear`, `quiet`, `pause`, `index`, `initrdfile`), genau wie bei Windows-Installern.

**Einschränkungen**:

- Programme, die die Rettungsumgebung auf ihrem eigenen Boot-Medium erwartet (zum Beispiel Strelecs `SSTR`-Ordner mit portablen Tools), liegen nicht in der WIM und fehlen nach einem PXE-Boot deshalb, sofern die Umgebung sie nicht selbst übers Netzwerk holen kann. Der WinPE-Desktop selbst und die in die WIM eingebauten Tools funktionieren.
- Medien, die eigene BCD-Einträge brauchen (eigene `bootmgr`-Menüs, Auswahlmenüs für mehrere WIMs), booten direkt in die gewählte WIM, statt dieses Menü anzuzeigen.
- Nackte `.wim`-Dateien lassen sich noch nicht einzeln hochladen; lege sie auf ein ISO mit `BCD` und `boot.sdi` oder verwende ein vollständiges Rettungs-ISO.

## Kernel-Extraktion

Die meisten modernen ISOs unterstützen direktes HTTP-Booten über iPXEs `sanboot`-Befehl, der das ganze ISO herunterlädt und bootet. Das Extrahieren von Kernel und initrd bringt aber deutliche Vorteile:

###  Vorteile der Kernel-Extraktion

- **Schnellere Boot-Zeiten**: Nur Kernel/initrd herunterladen (~100 MB) statt komplettem ISO (1-10 GB)
- **Geringere Bandbreite**: Kritisch in Netzen mit mehreren Clients
- **Bessere Kompatibilität**: Manche ISOs unterstützen `sanboot` nicht sauber
- **Netzwerk-Installation**: Netboot-Dateien für Debian-/Ubuntu-Installer nutzen
- **UEFI Secure Boot**: Die Extraktion erkennt den signierten shim des ISOs, sodass Secure-Boot-Clients den Kernel verifizieren können — siehe den [Secure-Boot-Leitfaden](secure-boot.md)

### Wie man extrahiert

**Per Web-Oberfläche**:
1. Zum Tab **Images** navigieren
2. Dein ISO-Image suchen
3. Auf **"Extract"** klicken
4. Bis zum Abschluss der Extraktion warten

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04-live-server-amd64.iso"}'
```

### Manuelle Extraktion

Wenn der eingebaute Extraktor dein ISO nicht unterstützt, kannst du die Boot-Dateien manuell extrahieren, und Bootimus erkennt sie automatisch.

1. Lege ein Verzeichnis mit demselben Namen wie das ISO an (ohne `.iso`-Endung):
   ```bash
   mkdir -p data/isos/my-custom-distro/
   ```

2. Lege Kernel und initrd mit exakt diesen Namen in das Verzeichnis:
   ```
   data/isos/
   ├── my-custom-distro.iso
   └── my-custom-distro/
       ├── vmlinuz          # Kernel
       └── initrd           # initrd/initramfs
   ```

3. Klicke im Admin-Panel auf **"Scan for ISOs"** (oder starte bootimus neu). Das Image wird automatisch als extrahiert erkannt und auf die Kernel-Boot-Methode gesetzt.

Das funktioniert auch für ISOs in Unterverzeichnissen:
```
data/isos/linux/my-custom-distro.iso
data/isos/linux/my-custom-distro/vmlinuz
data/isos/linux/my-custom-distro/initrd
```

### Was extrahiert wird

Bootimus erkennt automatisch die Distribution und extrahiert:

- **Kernel**: `vmlinuz` (oder `linux`, `bzImage`)
- **Initrd**: `initrd`, `initrd.gz`, `initrd.lz`
- **Squashfs** (Ubuntu/Debian Live): `filesystem.squashfs`
- **Distributions-Metadaten**: OS-Typ, Boot-Parameter

**Speicherort der extrahierten Dateien**:
```
/data/isos/
├── ubuntu-24.04.iso                    # Original-ISO
└── ubuntu-24.04/                       # Extraktions-Verzeichnis
    ├── vmlinuz                         # Kernel
    ├── initrd                          # Initrd
    └── casper/
        └── filesystem.squashfs         # Squashfs-Dateisystem
```

### Automatische Boot-Methoden-Auswahl

Nach der Extraktion wählt Bootimus automatisch die optimale Boot-Methode:

| Distribution | Boot-Methode | Downloads |
|--------------|-------------|-----------|
| Ubuntu Desktop (extrahiert) | `fetch=` | ~2,8 GB (nur Squashfs) |
| Ubuntu Desktop (nicht extrahiert) | `url=` | ~18 GB (ISO × 3) |
| Ubuntu Server (Netboot) | Netboot | ~50 MB (Netboot-Dateien) |
| Debian Installer (Netboot) | Netboot | ~30 MB (Netboot-Dateien) |
| Arch Linux | HTTP-Boot | ~100 MB (Kernel/initrd) |
| Fedora/RHEL | HTTP-Boot | ~150 MB (Kernel/initrd + stage2) |

## Netboot-Unterstützung

Manche Installer-ISOs (Debian, Ubuntu Server) enthalten kein komplettes OS — sie sind so gebaut, dass sie während der Installation Pakete herunterladen. Für die unterstützt Bootimus den Download offizieller Netboot-Dateien.

###  Netboot-Bedarf erkennen

Wenn du ein Debian- oder Ubuntu-Server-Installer-ISO extrahierst, erkennt Bootimus, dass es Netboot braucht:

**Indikatoren**:
- ISO enthält `/install/`-Verzeichnis (nicht `/casper/`)
- Installer-Typ (nicht Live/Desktop)
- Kleine ISO-Größe (< 1 GB)

**Im Admin-Panel angezeigt**:
-  Badge "Netboot Required"
- 📥 Button "Download Netboot"

### Netboot-Dateien herunterladen

**Per Web-Oberfläche**:
1. Zum Tab **Images** navigieren
2. Installer-ISO mit Badge "Netboot Required" suchen
3. **"Download Netboot"** klicken
4. Auf Download und Extraktion warten

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

### Was sind Netboot-Dateien?

Netboot-Dateien sind offizielle, minimale Boot-Dateien der Distributionen:

**Debian-Netboot**:
- Quelle: `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`
- Größe: ~30 MB
- Enthält: `vmlinuz`, `initrd.gz`, Installer-Dateien

**Ubuntu-Netboot**:
- Quelle: `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`
- Größe: ~50 MB
- Enthält: `vmlinuz`, `initrd.gz`, Installer-Dateien

### Wie Netboot funktioniert

1. **Client bootet**: Lädt Netboot-Kernel/initrd herunter (~50 MB)
2. **Installer startet**: Netboot-initrd startet den Netzwerk-Installer
3. **Paket-Download**: Installer lädt Pakete von Ubuntu-/Debian-Mirrors
4. **Installation**: OS wird direkt aus den Internet-Repositories installiert

**Vorteile**:
-  Immer die neuesten Pakete (keine veralteten ISO-Pakete)
-  Minimale Bandbreite zum PXE-Server (kein ISO-Download)
-  Geringerer Speicherbedarf
-  Offizielle, signierte Boot-Dateien

### Debian-Installer-Netboot

**Unterstützte ISOs**:
- `debian-*-netinst.iso` — Netzwerk-Installer
- Kleine Debian-Installer-ISOs mit `/install/`-Verzeichnis

**Erkennung**:
```
ISO-Struktur:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**Netboot-URL**: `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`

**Boot-Parameter**: `priority=critical ip=dhcp`

### Ubuntu-Server-Netboot

**Unterstützte ISOs**:
- `ubuntu-*-live-server-*.iso` — Live-Server-Installer mit `/install/`-Verzeichnis
- Ältere Ubuntu-Server-Installer

**Erkennung**:
```
ISO-Struktur:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**Netboot-URL**: `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`

**Boot-Parameter**: `ip=dhcp`

###  Wichtig: Ubuntu Desktop vs. Server

Es gibt **zwei Arten** von Ubuntu-ISOs mit unterschiedlichen Boot-Methoden:

| Typ | ISO-Name-Pattern | Verzeichnis | Boot-Methode | Netboot? |
|------|------------------|-----------|-------------|----------|
| **Desktop/Live** | `ubuntu-*-desktop-*.iso` | `/casper/` | `fetch=` oder `url=` |  Nein |
| **Server-Installer** | `ubuntu-*-live-server-*.iso` (mit `/install/`) | `/install/` | Netboot |  Ja |

**Ubuntu Desktop** (`/casper/`):
- Enthält vollständiges Live-OS
- Nutzt Casper-Boot mit `fetch=` oder `url=`
- Kernel extrahieren, um `fetch=` zu nutzen (lädt nur Squashfs)
- Keine Netboot-Unterstützung

**Ubuntu-Server-Installer** (`/install/`):
- Minimaler Netzwerk-Installer
- Benötigt Netboot-Dateien
- Lädt Pakete während der Installation
- Deutlich effizienter

## Ubuntu-Desktop-Optimierung

Ubuntu-Desktop-ISOs nutzen das Casper-Live-Boot-System. Ohne Optimierung wird das gesamte ISO **dreimal** heruntergeladen (~18 GB für ein 6-GB-ISO).

###  Problem: Dreifach-ISO-Download

**Default-Verhalten** (ohne Extraktion):
```
Boot-Parameter: url=http://server/ubuntu.iso

Ergebnis:
- Download 1: Kernel verifiziert ISO (6 GB)
- Download 2: initrd verifiziert ISO (6 GB)
- Download 3: Casper mountet ISO (6 GB)
Gesamt: ~18 GB heruntergeladen
```

###  Lösung 1: Extrahieren und `fetch=`-Parameter nutzen

**Nach Extraktion**:
```
Boot-Parameter: fetch=http://server/ubuntu/casper/filesystem.squashfs

Ergebnis:
- Download: Nur Squashfs (~2,8 GB)
Gesamt: ~2,8 GB heruntergeladen
```

**So aktivieren**:
1. Kernel/initrd aus dem ISO extrahieren
2. Bootimus nutzt automatisch den `fetch=`-Parameter
3. Nur Squashfs wird heruntergeladen (nicht das ganze ISO)

**Einsparung**: 85% Reduktion (18 GB → 2,8 GB)

###  Lösung 2: Stattdessen Ubuntu-Server-Netboot nutzen

Für Server-Deployments den Ubuntu-Server-Installer mit Netboot nutzen:

**Netboot-Ansatz**:
```
1. ubuntu-server.iso hochladen
2. Kernel/initrd extrahieren
3. Netboot-Dateien herunterladen
4. Mit Netboot booten (~50 MB Download)
5. Aus Ubuntu-Repositories installieren
```

**Einsparung**: 99% Reduktion (18 GB → 50 MB)

### Boot-Parameter-Referenz

**Ubuntu Desktop (Casper)**:
```bash
# Default (ohne Extraktion) — lädt ISO 3-mal
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp url=http://server/ubuntu.iso

# Optimiert (mit Extraktion) — lädt Squashfs einmal
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp fetch=http://server/ubuntu/casper/filesystem.squashfs
```

**Ubuntu Server (Netboot)**:
```bash
# Netboot — minimaler Download
ip=dhcp
```

## Unterstützte Distributionen

### Vollständig getestet

| Distribution | Kernel-Extraktion | Netboot | Hinweise |
|--------------|-------------------|---------|-------|
| **Arch Linux** |  Ja |  N/A | `/arch/boot/x86_64/vmlinuz-linux` |
| **Fedora Workstation** |  Ja |  N/A | `/isolinux/vmlinuz` |
| **Rocky Linux** |  Ja |  N/A | `/isolinux/vmlinuz` |
| **Debian (Installer)** |  Ja |  Ja | `/install/vmlinuz` + Netboot |
| **Debian Live** |  Ja |  Nein | `/live/vmlinuz` |
| **Ubuntu Desktop** |  Ja |  Nein | `/casper/vmlinuz` + fetch-Optimierung |
| **Ubuntu Server** |  Ja |  Ja | `/install/vmlinuz` + Netboot |
| **Pop!_OS** |  Ja |  Nein | `/casper/vmlinuz` |
| **TrueNAS SCALE** |  Ja |  Nein | 25.10.2.1: Installation und Festplattenstart in UEFI-QEMU; vollständiger ISO-Download |
| **Proxmox VE** |  Ja |  Nein | `/boot/linux26` + `/boot/initrd.img` |
| **openSUSE** |  Ja |  N/A | `/boot/x86_64/loader/linux` |
| **NixOS** |  N/A |  N/A | Sanboot |

### Erkennungs-Patterns

Bootimus erkennt Distributionen, indem er nach bestimmten Datei-Patterns scannt:

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

**Ubuntu Desktop (Casper)**:
```
/casper/vmlinuz oder /casper/vmlinuz.efi
/casper/initrd oder /casper/initrd.gz oder /casper/initrd.lz
/casper/filesystem.squashfs
```

**Ubuntu-Server-Installer**:
```
/install/vmlinuz oder /install.amd/vmlinuz
/install/initrd.gz oder /install.amd/initrd.gz
```

**Debian-Installer**:
```
/install/vmlinuz oder /install.amd/vmlinuz
/install/initrd.gz oder /install.amd/initrd.gz
```

**TrueNAS SCALE**:
```
/vmlinuz
/initrd.img
/live/filesystem.squashfs
/TrueNAS-SCALE.update
```

Bootimus erkennt SCALE an der Datei `TrueNAS-SCALE.update` und wählt ein eigenes Profil. Kernel/Initrd wie gewohnt extrahieren; das Profil lädt die vollständige ISO und bindet sie für den Installer unter `/cdrom` ein. Die ISO bleibt unverändert. Der Installer benötigt systemd 254 oder neuer.

Mit SCALE 25.10.2.1 in UEFI-QEMU und 8 GiB RAM geprüft: Installation auf eine neue virtuelle Festplatte und anschließender Start davon. Die vollständige ISO bleibt während der Installation im RAM. Andere Versionen, echte Hardware und Secure Boot sind ungetestet. Das Profil gilt nicht für das FreeBSD-basierte TrueNAS CORE.

Bereits extrahierte Images erneut extrahieren, um die alten Debian-Einstellungen zu ersetzen. Eigene Boot-Parameter danach prüfen.

**Proxmox VE**:
```
/boot/linux26
/boot/initrd.img
```

## Fehlersuche

### Extraktion fehlgeschlagen

**Symptome**: Fehlermeldung "Extraction failed" im Admin-Panel

**Häufige Ursachen**:
1. **Beschädigtes ISO**: ISO neu herunterladen
2. **Nicht unterstütztes ISO**: Prüfen, ob die Distribution unterstützt wird
3. **Plattenplatz**: Sicherstellen, dass genug Platz für die Extraktion da ist
4. **Rechte**: Dateirechte auf dem Daten-Verzeichnis prüfen

**Debugging**:
```bash
# Extraktions-Logs prüfen
docker logs bootimus | grep -i extract

# ISO-Integrität verifizieren
sha256sum ubuntu.iso

# Plattenplatz prüfen
df -h /data/isos/

# Manuelles Mounten testen
sudo mount -o loop ubuntu.iso /mnt
ls /mnt/casper/
sudo umount /mnt
```

### Netboot-Download fehlgeschlagen

**Symptome**: Fehlermeldung "Netboot download failed"

**Häufige Ursachen**:
1. **Netzwerk-Konnektivität**: Debian-/Ubuntu-Mirrors nicht erreichbar
2. **URL geändert**: Mirror-URL wurde aktualisiert
3. **Tarball-Extraktion fehlgeschlagen**: Beschädigter Download

**Lösungen**:
```bash
# Mirror-Konnektivität testen
curl -I http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz

# Server-Logs prüfen
docker logs bootimus | grep -i netboot

# Netboot-URL manuell verifizieren
wget http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz
tar -tzf netboot.tar.gz | grep vmlinuz
```

### Boot-Menü zeigt falschen Image-Typ

**Symptome**: Image zeigt Badge "[kernel]", bootet aber nicht mit Kernel-Methode

**Ursache**: Datenbank und Dateisystem sind nicht synchron

**Lösung**:
```bash
# Kernel/initrd neu extrahieren
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'

# Oder ISOs neu scannen
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### Client lädt ISO mehrfach herunter

**Symptome**: Ubuntu-Desktop-ISO wird 3-mal heruntergeladen

**Ursache**: `url=`-Parameter ohne Extraktion verwendet

**Lösung**:
1. Kernel/initrd aus dem ISO extrahieren
2. Bootimus nutzt automatisch den `fetch=`-Parameter
3. Nur Squashfs wird heruntergeladen (nicht das ganze ISO)

**Verifizieren**:
```bash
# Prüfen, ob extrahiert wurde
ls -la /data/isos/ubuntu-24.04/casper/filesystem.squashfs

# Server-Logs während Boot prüfen
docker logs -f bootimus
# Achte auf: "fetch=..." statt "url=..."
```

### Netboot erforderlich, aber kein Download-Button

**Symptome**: Image zeigt "Netboot Required", aber kein Download-Button

**Ursache**: Netboot-URL nicht konfiguriert oder Erkennung fehlgeschlagen

**Lösung**:
```bash
# Image-Details prüfen
curl -u admin:password http://localhost:8081/api/images | jq '.data[] | select(.filename=="debian-13.2.0-amd64-netinst.iso")'

# Felder netboot_required und netboot_url verifizieren
# Wenn netboot_url leer ist, ist die ISO-Erkennung wohl fehlgeschlagen

# Neu-Extraktion versuchen
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

## Nächste Schritte

-  Siehe den [Admin-Konsolen-Leitfaden](admin.md) zur Image-Verwaltung
-  Lies den [Deployment-Leitfaden](deployment.md) für Storage-Konfiguration
-  Konfiguriere den [DHCP-Server](dhcp.md) für PXE-Boot
-  Richte die [Client-Verwaltung](clients.md) für Zugriffskontrolle ein
