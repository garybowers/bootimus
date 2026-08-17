# Auto-Install-Leitfaden

Unbeaufsichtigte Installationen für Windows, Ubuntu, Debian und die Red-Hat-Familie. Einmal eine Config-Datei reinwerfen, an ein Image anhängen — und jeder PXE-Boot zieht die Installation ohne einen einzigen Tastendruck durch.

## Inhaltsverzeichnis

- [Überblick](#überblick)
- [Unterstützte Formate](#unterstützte-formate)
- [Die Dateibibliothek](#die-dateibibliothek)
- [Dateien anhängen](#dateien-anhängen)
- [Auflösungsreihenfolge](#auflösungsreihenfolge)
- [Platzhalter](#platzhalter)
- [Beispiele](#beispiele)
- [Windows-Hinweise](#windows-hinweise)
- [REST-API](#rest-api)
- [Fehlersuche](#fehlersuche)

## Überblick

Auto-Install-Konfigurationen werden in einer Per-Distro-Bibliothek unter `data/autoinstall/` gespeichert. Du kannst:

- **Dateien im UI verwalten** im Tab **Auto-Install** — Configs anlegen, editieren, hochladen, herunterladen und löschen, ohne das Dateisystem anzufassen.
- **Default pro Image setzen** (jeder Client, der dieses Image bootet, bekommt dieselbe Config).
- **Pro Client oder Client-Gruppe überschreiben**, wenn eine Maschine eine andere Config braucht (anderer Hostname, anderes Disk-Layout, andere Rolle etc.).
- **Templates bauen** mit Platzhaltern wie `{{HOSTNAME}}` und `{{IP}}`, die beim Ausliefern anhand der Identität des bootenden Clients aufgelöst werden.

Bootimus liefert das Skript über HTTP am Auto-Install-Endpunkt aus — und für Windows wird `AutoUnattend.xml` auf der SMB-Installations-Freigabe bereitgestellt, sodass `setup.exe /unattend:` sie automatisch aufgreift.

## Unterstützte Formate

| Distro-Familie | Format | Datei-Endung | Erkannt als |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| Alles andere | raw | beliebig | `generic` |

Die Endung steuert sowohl das Label im UI als auch den `Content-Type`-Header beim Ausliefern.

## Die Dateibibliothek

Alle Auto-Install-Dateien liegen unter `data/autoinstall/<distro>/<filename>`:

```
data/autoinstall/
├── windows/
│   ├── kiosk.xml
│   └── server-2022.xml
├── ubuntu/
│   ├── default.yaml
│   └── lab-bench.yaml
├── debian/
│   └── server.cfg
└── rocky/
    └── workstation.ks
```

Das Segment `<distro>` muss zu einer bekannten Distro-Profil-ID passen (siehe [Distro-Profile](distro-profiles.md)). Das Verzeichnis wird beim ersten Start automatisch angelegt.

### Dateien über das UI hinzufügen

Im Tab **Auto-Install** → **New File** öffnet sich der Editor mit einem Distro-Picker, einem Dateinamen-Feld und einer Syntax-freundlichen Textarea. **Upload File** nimmt eine beliebige lokale Datei und legt sie im gewählten Distro-Ordner ab.

### Dateien manuell hinzufügen

Einfach reinwerfen:

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

Sie tauchen sofort im UI auf — kein Neustart, kein Scan.

## Dateien anhängen

Auto-Install-Dateien tun nichts, bis du sie verdrahtest. Es gibt drei Stellen zum Anhängen:

### Image (Default)

Tab **Images** → **Eigenschaften** eines Images öffnen → Abschnitt **Auto-Install** → Datei wählen. Jeder Client, der dieses Image bootet, bekommt diese Config, sofern nichts Spezifischeres überschreibt.

### Client (Per-Maschine-Override)

Tab **Clients** → Client öffnen → Dropdown **Auto-Install File**. Nutze das, wenn eine bestimmte Maschine eine andere Config braucht (z.B. ein Build-Server vs. der Rest der Schreibtisch-Flotte).

### Client-Gruppe (Per-Flotten-Override)

Tab **Groups** → Gruppe öffnen → **Auto-Install File**. Gilt für jeden Client in der Gruppe. Nützlich für "alle Workstations in Lab 3"-Szenarien.

## Auflösungsreihenfolge

Wenn ein Client seine Auto-Install-Datei anfordert, läuft Bootimus diese Hierarchie ab:

```
1. Per-Client-Override        (Client.AutoInstallFile)
2. Per-Group-Override         (ClientGroup.AutoInstallFile, falls Client in einer Gruppe)
3. Image-Default              (Image.AutoInstallFile)
4. Inline-Legacy-Skript       (Image.AutoInstallScript — Pre-0.1.58-Setups)
5. → 404 (kein Auto-Install konfiguriert)
```

Der erste nicht-leere Treffer gewinnt. Der Endpunkt loggt die Quelle, von der er ausgeliefert hat:

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## Platzhalter

Diese Tokens werden beim Ausliefern pro Client ersetzt:

| Token | Wird ersetzt durch |
|-------|---------------|
| `{{MAC}}` | Client-MAC-Adresse (lowercase, mit Doppelpunkten) |
| `{{CLIENT_NAME}}` | Anzeigename aus der Clients-Tabelle |
| `{{HOSTNAME}}` | Wie `{{CLIENT_NAME}}` (Alias zur Klarheit in Configs) |
| `{{IP}}` | Client-IP, die den Request gestellt hat |
| `{{SERVER_ADDR}}` | Adresse des Bootimus-Servers |
| `{{IMAGE_NAME}}` | Anzeigename des bootenden Images |
| `{{IMAGE_FILENAME}}` | ISO-Dateiname des bootenden Images |

Platzhalter sind plain String-Substitution — kein Escaping. Quote sie passend zum Zielformat (XML, YAML etc.).

## Beispiele

### Ubuntu Server (cloud-init)

`data/autoinstall/ubuntu/default.yaml`:

```yaml
#cloud-config
autoinstall:
  version: 1
  identity:
    hostname: {{HOSTNAME}}
    username: ubuntu
    password: "$6$rounds=4096$..."  # mkpasswd -m sha-512
  ssh:
    install-server: true
    allow-pw: false
    authorized-keys:
      - ssh-ed25519 AAAA...
  storage:
    layout:
      name: lvm
  late-commands:
    - curtin in-target -- systemctl enable --now serial-getty@ttyS0.service
```

Boot-Parameter (das passende Image hat diese für Ubuntu bereits als Default):

```
autoinstall ds=nocloud-net;s=http://{{SERVER_ADDR}}:8080/autoinstall/{{IMAGE_FILENAME}}/
```

### Debian (preseed)

`data/autoinstall/debian/server.cfg`:

```
d-i debian-installer/locale string en_GB.UTF-8
d-i keyboard-configuration/xkb-keymap select gb
d-i netcfg/get_hostname string {{HOSTNAME}}
d-i netcfg/get_domain string lan
d-i partman-auto/method string lvm
d-i partman-auto/choose_recipe select atomic
d-i passwd/root-login boolean false
d-i passwd/user-fullname string Admin
d-i passwd/username string admin
d-i passwd/user-password-crypted password $6$rounds=4096$...
d-i pkgsel/include string openssh-server
d-i grub-installer/bootdev string default
d-i finish-install/reboot_in_progress note
```

### Rocky / Fedora (kickstart)

`data/autoinstall/rocky/workstation.ks`:

```
text
lang en_GB.UTF-8
keyboard gb
timezone Europe/London --utc
network --bootproto=dhcp --hostname={{HOSTNAME}}
rootpw --lock
user --name=admin --groups=wheel --password=$6$rounds=4096$... --iscrypted
sshkey --username=admin "ssh-ed25519 AAAA..."
bootloader --location=mbr
clearpart --all --initlabel
autopart --type=lvm
%packages
@^minimal-environment
openssh-server
%end
```

### Windows 11 / Server (autounattend)

`data/autoinstall/windows/kiosk.xml`: Standard-`<unattend>`-Dokument — siehe [Microsofts autounattend-Referenz](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/). Platzhalter funktionieren in jedem Text-Node:

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## Windows-Hinweise

Windows-Installationen laufen über SMB. Wenn an einem Image eine autounattend-Datei hängt, macht Bootimus:

1. Beim Patchen von `boot.wim` wird `AutoUnattend.xml` auf der SMB-Installations-Freigabe bereitgestellt.
2. `startnet.cmd` wird gepatcht, sodass WinPE die Datei beim Boot nach `X:\AutoUnattend.xml` (die lokale RAM-Disk) kopiert.
3. Setup wird als `setup.exe /unattend:X:\AutoUnattend.xml` gestartet.

Ohne die Per-Image-autounattend-Datei läuft Setup wie gehabt interaktiv.

**Reboot-Resilienz.** WinPE startet mitten in der Installation neu und verbindet sich von derselben Client-IP wieder. Die mitgelieferte Samba-Config setzt `reset on zero vc = yes` und deaktiviert Oplocks, damit das zweite `net use` nicht an einem stale Session-State scheitert. Wenn du `data/smb/smb.conf` durch deine eigene ersetzt hast, übernimm diese Einstellungen.

**Patch rückgängig machen.** Beim ersten SMB-Patch wird eine unveränderte Kopie der `boot.wim` als `boot.wim.orig` daneben abgelegt. **Images** → **SMB-Patch entfernen** stellt das Original wieder her und entfernt die SMB-Freigabe des Images — die ISO bootet dann exakt wie hochgeladen. Images, die von älteren Bootimus-Versionen gepatcht wurden, haben keine Sicherung: erst das Image neu extrahieren (das erzeugt eine frische unveränderte Kopie), dann den Patch entfernen.

## REST-API

Alles im UI ist auch ein REST-Aufruf.

```bash
# Alle Auto-Install-Dateien auflisten
curl -u admin:pw http://localhost:8081/api/autoinstall-files

# Datei lesen
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/get?distro=ubuntu&filename=default.yaml"

# Datei anlegen oder überschreiben
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/save \
  -H "Content-Type: application/json" \
  -d '{"distro":"ubuntu","filename":"default.yaml","content":"#cloud-config\n..."}'

# Datei hochladen
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/upload \
  -F "distro=windows" \
  -F "filename=kiosk.xml" \
  -F "file=@./kiosk.xml"

# Herunterladen
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/download?distro=ubuntu&filename=default.yaml" -o default.yaml

# Löschen
curl -u admin:pw -X POST "http://localhost:8081/api/autoinstall-files/delete?distro=ubuntu&filename=default.yaml"
```

Datei an ein Image anhängen:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

Datei an einen Client anhängen:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

Den Auto-Install-Endpunkt, den Clients beim Boot ansprechen:

```
GET /autoinstall/<image-filename>/?mac=<mac>
```

Der Query-Parameter `mac` wird vom Boot-Menü automatisch angehängt, damit Per-Client-Overrides korrekt aufgelöst werden.

## Fehlersuche

### 404 von `/autoinstall/...`

`no auto-install configuration for this image/client` — auf keiner Ebene der Auflösungskette ist etwas angehängt. Entweder hänge eine Datei an das Image, den Client oder seine Gruppe — oder prüfe, ob `auto_install_file` tatsächlich auf eine existierende Datei unter `data/autoinstall/` zeigt.

### Platzhalter werden wörtlich ausgespielt

Wenn `{{HOSTNAME}}` als literaler String im installierten System auftaucht, wurde die Datei vor dem Substitutions-Lauf ausgeliefert — meistens, weil der Client nur per IP gebootet hat und der Request keinen `mac`-Query-Parameter enthielt. Stelle sicher, dass das Boot-Menü URLs der Form `/autoinstall/<iso>/?mac=<mac>` erzeugt.

### Falsche Datei ausgeliefert

Die Auflösung läuft nach Most-Specific-First. Wenn ein Client einen eigenen Override hat und du das nicht erwartest, ist das der Grund, warum das Image-Level-Default nicht greift. Prüfe die Server-Log-Zeile:

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

Das Feld `source:` sagt dir genau, welcher Slot gewonnen hat.

### Windows-Setup läuft interaktiv

- Am Image muss eine autounattend-Datei hängen (Image-Eigenschaften → Auto-Install).
- `boot.wim` nach dem Anhängen neu patchen: **Images** → **Patch SMB** (oder beim nächsten Boot wird automatisch neu gepatcht).
- Erreichbarkeit der SMB-Freigabe vom Client prüfen (`net view \\<server>` aus WinPE).

### "AutoUnattend.xml not on share, running interactive setup"

Wird von `startnet.cmd` geloggt, wenn die Datei nicht dort ist, wo erwartet. Entweder ist der Staging-Schritt fehlgeschlagen (prüfe das Bootimus-Server-Log um den Patch-Zeitpunkt herum) oder die SMB-Freigabe hat die Datei verloren. SMB-Patch aus den Image-Eigenschaften erneut ausführen.

### `net use fails after VM reboot`

In 0.1.58 behoben, indem `reset on zero vc = yes` in der mitgelieferten Samba-Config aktiviert wurde. Wenn du eine eigene `smb.conf` pflegst, ergänze:

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## Nächste Schritte

- Siehe [Image-Verwaltung](images.md), um Dateien an Images zu hängen.
- Siehe [Client-Verwaltung](clients.md) für Per-Client-Overrides.
- Siehe [Distro-Profile](distro-profiles.md) für die zugrunde liegenden Profil-IDs, die auf die Bibliotheks-Unterverzeichnisse mappen.
