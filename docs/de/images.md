#  Leitfaden zur Image-Verwaltung

Kompletter Leitfaden zur Verwaltung von ISO-Images, zum Extrahieren von Boot-Dateien und zur Behandlung von Sonderfällen wie Debian-/Ubuntu-Netboot.

##  Inhaltsverzeichnis

- [Images hinzufügen](#images-hinzufügen)
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
| **TrueNAS SCALE** |  Ja |  Nein | `/vmlinuz` + `/initrd.img` (Root) |
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
```

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
