#  Deployment-Leitfaden

Kompletter Leitfaden zum Ausrollen von Bootimus in verschiedenen Umgebungen mit Netzwerk- und Storage-Konfigurationen.

##  Inhaltsverzeichnis

- [Schnellstart](#schnellstart)
- [Docker-Deployment](#docker-deployment)
- [Binary-Deployment](#binary-deployment)
- [Netzwerk-Konfiguration](#netzwerk-konfiguration)
- [Storage-Konfiguration](#storage-konfiguration)
- [Datenbank-Optionen](#datenbank-optionen)
- [Remote-Updates & Datenschutz](#remote-updates--datenschutz)
- [Produktiv-Deployment](#produktiv-deployment)

## Schnellstart

### Docker (empfohlen)

```bash
# Daten-Verzeichnis anlegen
mkdir -p data

# Mit SQLite starten (kein DB-Container nötig)
docker run -d \
  --name bootimus \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -p 8081:8081/tcp \
  -v $(pwd)/data:/data \
  garybowers/bootimus:latest

# Admin-Passwort aus den Logs holen
docker logs bootimus | grep "Password"

# Admin-Oberfläche aufrufen
open http://localhost:8081
```

### Standalone-Binary

```bash
# Binary herunterladen
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64
chmod +x bootimus-amd64

# Daten-Verzeichnis anlegen
mkdir -p data

# Starten (SQLite-Modus — keine Datenbank nötig)
./bootimus-amd64 serve

# Admin-Panel: http://localhost:8081
# Admin-Passwort steht in den Startup-Logs
```

## Docker-Deployment

### Docker Compose mit PostgreSQL

```bash
# Repository klonen
git clone https://github.com/garybowers/bootimus
cd bootimus

# Mit PostgreSQL starten
docker-compose up -d

# Logs ansehen
docker-compose logs -f bootimus
```

Der Docker-Compose-Stack umfasst:
- **Bootimus-Server**: Hauptserver für PXE/HTTP-Boot
- **PostgreSQL**: Datenbank für Client-/Image-Verwaltung
- **Health Checks**: Automatisches Service-Monitoring
- **Persistenter Storage**: Datenvolumes für ISOs und Datenbank

### Verzeichnisstruktur

Bootimus legt Unterverzeichnisse automatisch an:
- `/data/isos/` - ISO-Image-Dateien und extrahierte Boot-Dateien (in Unterverzeichnissen pro ISO)
- `/data/bootloaders/` - Eigene Bootloader-Dateien (optional)
- `/data/bootimus.db` - SQLite-Datenbank (im SQLite-Modus)

## Netzwerk-Konfiguration

### Standard-Internes Bridge-Netzwerk

Standardmäßig nutzen Container ein internes Bridge-Netzwerk mit Port-Forwarding:

```yaml
networks:
  bootimus_net:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
          gateway: 172.20.0.1
```

- **Bootimus-Server**: `172.20.0.3`
- **PostgreSQL**: `172.20.0.2`
- **Zugriff vom Host**: Über Port-Forwarding (z.B. `localhost:8081`)

### Bridged-Netzwerk mit statischer IP im LAN

Für produktive PXE-Umgebungen willst du den Container vielleicht direkt mit einer statischen IP in deinem LAN haben.

#### Schritt 1: Netzwerk-Interface ermitteln

```bash
ip addr show  # Linux
# Suche dein Haupt-Interface (z.B. eth0, ens33, enp0s3)
```

#### Schritt 2: docker-compose.yml editieren

Die `host_bridge`-Netzwerkblöcke einkommentieren:

```yaml
services:
  bootimus:
    networks:
      # Internes Bridge auskommentieren
      # bootimus_net:
      #   ipv4_address: 172.20.0.3
      # Host-Bridge aktivieren
      host_bridge:
        ipv4_address: 192.168.1.100  # Deine gewünschte statische IP
    environment:
      BOOTIMUS_SERVER_ADDR: 192.168.1.100  # Statische Server-Adresse setzen

networks:
  # Für dein LAN einkommentieren und anpassen
  host_bridge:
    driver: macvlan
    driver_opts:
      parent: eth0  # Dein Netzwerk-Interface
    ipam:
      config:
        - subnet: 192.168.1.0/24      # Dein LAN-Subnetz
          gateway: 192.168.1.1         # Dein LAN-Gateway
          ip_range: 192.168.1.100/32   # Statische IP des Containers
```

#### Schritt 3: Netzwerk-Details konfigurieren

Diese Werte für dein Netzwerk anpassen:
- `parent`: Netzwerk-Interface des Hosts (z.B. `eth0`, `ens33`)
- `subnet`: Dein LAN-Subnetz (z.B. `192.168.1.0/24`)
- `gateway`: IP deines Routers (z.B. `192.168.1.1`)
- `ip_range`: Statische IP für Bootimus (z.B. `192.168.1.100/32`)
- `BOOTIMUS_SERVER_ADDR`: Gleich wie die statische IP

#### Schritt 4: Container starten

```bash
docker-compose down
docker-compose up -d
```

#### Schritt 5: Konnektivität prüfen

```bash
# Von einer anderen Maschine im LAN
curl http://192.168.1.100:8081

# Container pingen
ping 192.168.1.100
```

###  Wichtige Hinweise zu Macvlan-Networking

- **Macvlan-Networking**: Der Container erscheint als eigenständiges Gerät in deinem LAN
- **Host kann den Container nicht erreichen**: Die Host-Maschine kann mit Macvlan-Containern nicht direkt sprechen. Nutze eine separate VM/Container für Admin-Zugriff oder lege ein Macvlan-Interface auf dem Host an.
- **DHCP-Konflikte**: Stelle sicher, dass die statische IP außerhalb deines DHCP-Range liegt oder im DHCP-Server reserviert ist
- **Firewall-Regeln**: Der Container umgeht die Host-Firewall — konfiguriere ggf. die Container-Firewall separat

### Macvlan-Container vom Host aus erreichen

Wenn du vom Host auf den Macvlan-Container zugreifen musst:

```bash
# Macvlan-Interface auf dem Host anlegen
sudo ip link add macvlan0 link eth0 type macvlan mode bridge
sudo ip addr add 192.168.1.101/32 dev macvlan0
sudo ip link set macvlan0 up
sudo ip route add 192.168.1.100/32 dev macvlan0

# Jetzt kannst du den Container vom Host aus erreichen
curl http://192.168.1.100:8081
```

## Binary-Deployment

### Systemanforderungen

- **OS**: Linux (amd64, arm64, armv7)
- **Rechte**: Root für Port 69 (TFTP), oder nicht-privilegierte Ports nutzen
- **Plattenplatz**: 10 GB+ für ISO-Storage
- **Speicher**: Min. 512 MB, empfohlen 2 GB+

### Installation

```bash
# Binary für deine Architektur herunterladen
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64

# Ausführbar machen
chmod +x bootimus-amd64

# An Systemort verschieben
sudo mv bootimus-amd64 /usr/local/bin/bootimus

# Daten-Verzeichnis anlegen
sudo mkdir -p /opt/bootimus/data

# Systemd-Service anlegen
sudo nano /etc/systemd/system/bootimus.service
```

### Systemd-Service

```ini
[Unit]
Description=Bootimus PXE/HTTP Boot Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/bootimus
ExecStart=/usr/local/bin/bootimus serve --data-dir /opt/bootimus/data
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
# Service aktivieren und starten
sudo systemctl daemon-reload
sudo systemctl enable bootimus
sudo systemctl start bootimus

# Status prüfen
sudo systemctl status bootimus

# Logs ansehen
sudo journalctl -u bootimus -f
```

## Storage-Konfiguration

### Struktur des Daten-Verzeichnisses

```
/opt/bootimus/data/
├── isos/                           # ISO-Dateien
│   ├── ubuntu-24.04.iso           # ISO-Datei
│   ├── ubuntu-24.04/              # Extrahierte Boot-Dateien
│   │   ├── vmlinuz
│   │   ├── initrd
│   │   └── casper/
│   │       └── filesystem.squashfs
│   └── debian-12.iso
├── bootloaders/                    # Eigene Bootloader (optional)
├── bootimus.db                     # SQLite-Datenbank (im SQLite-Modus)
└── .admin_password                 # Erzeugtes Admin-Passwort
```

### Plattenplatz-Bedarf

- **ISOs**: 1-10 GB pro ISO
- **Extrahierte Dateien**: 100 MB-3 GB pro ISO
- **Datenbank**: < 100 MB
- **Empfohlen**: 50 GB+ für mehrere ISOs

### Storage Best Practices

1. **SSD nutzen**: Schnellere Boot-Zeiten für Clients
2. **Regelmäßige Backups**: Datenbank und ISOs sichern
3. **Plattenplatz überwachen**: Alerts bei wenig freiem Speicher einrichten
4. **Alte ISOs aufräumen**: Ungenutzte ISOs entfernen, um Platz zu schaffen

## Datenbank-Optionen

### SQLite-Modus (Default)

SQLite ist **standardmäßig aktiviert** — keine Konfiguration nötig!

```bash
# Mit SQLite starten (Default)
./bootimus serve

# Datenbank wird automatisch angelegt unter: <data_dir>/bootimus.db
```

**Vorteile**:
-  Null Konfiguration
-  Einzeldatei-Datenbank
-  Ideal für Single-Server-Deployments
-  Einfache Backups (einfach die Datei kopieren)

**Einschränkungen**:
-  Geringere Concurrency als PostgreSQL
-  Nur Single-Server (kein Clustering)

### PostgreSQL-Modus

Für Enterprise-Deployments mit hoher Concurrency:

#### Methode Konfigurationsdatei

```yaml
# bootimus.yaml
db:
  host: postgres.example.com
  port: 5432
  user: bootimus
  password: secretpassword
  name: bootimus
  sslmode: require
```

#### Methode Environment-Variable

```bash
export BOOTIMUS_DB_HOST=postgres.example.com
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secretpassword
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=require

./bootimus serve
```

**Vorteile**:
-  Hohe Concurrency
-  Multi-Server-Deployments
-  Erweiterte Replikation
-  Bessere Performance bei Scale

**Voraussetzungen**:
- PostgreSQL-12+-Server
- Netzwerk-Konnektivität zur Datenbank
- Zusätzliche Infrastruktur

## Remote-Updates & Datenschutz

Bootimus ist selbst gehostet und funkt **nicht** im Hintergrund nach Hause. Es wird mit einem vollständigen Katalog an Distro- und Tool-Profilen ausgeliefert, die ins Binary eingebettet sind, sodass es ohne ausgehenden Internetzugang voll funktionsfähig ist.

Der **einzige** Zeitpunkt, zu dem Bootimus einen externen Dienst kontaktiert, ist, wenn ein Operator **explizit** ein Profil-/Tool-Update auslöst — über die "Auf Updates prüfen"-Buttons im Admin-UI, den CLI-Befehl `bootimus profiles update` oder die Endpunkte `POST /api/profiles/update` und `POST /api/tools/update`. Jeder davon führt ein nicht authentifiziertes `GET` einer statischen JSON-Datei auf GitHub aus (`raw.githubusercontent.com/garybowers/bootimus/main/...`) und sendet keine Systeminformationen oder Kennungen mit.

Um zu garantieren, dass niemals ein Remote-Kontakt stattfindet (z.B. bei air-gapped Deployments), starte mit deaktivierten Remote-Updates:

```bash
bootimus serve --disable-remote-profiles
# or in bootimus.yaml:  disable_remote_profiles: true
# or via env:           BOOTIMUS_DISABLE_REMOTE_PROFILES=true
```

Alle Details findest du im [Distro-Profile-Leitfaden](distro-profiles.md#remote-updates--datenschutz).

## Produktiv-Deployment

### Docker mit SQLite (am einfachsten)

```bash
docker run -d \
  --name bootimus \
  --restart unless-stopped \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -p 8081:8081/tcp \
  -v /opt/bootimus/data:/data \
  garybowers/bootimus:latest
```

### Docker Compose mit PostgreSQL

```yaml
version: '3.8'

services:
  bootimus:
    image: garybowers/bootimus:latest
    container_name: bootimus
    restart: unless-stopped
    cap_add:
      - NET_BIND_SERVICE
    ports:
      - "69:69/udp"
      - "8080:8080/tcp"
      - "8081:8081/tcp"
    volumes:
      - ./data:/data
      - ./bootimus.yaml:/app/bootimus.yaml
    environment:
      - BOOTIMUS_DB_HOST=postgres
      - BOOTIMUS_DB_PASSWORD=secretpassword
    depends_on:
      - postgres

  postgres:
    image: postgres:17-alpine
    container_name: bootimus-db
    restart: unless-stopped
    environment:
      - POSTGRES_USER=bootimus
      - POSTGRES_PASSWORD=secretpassword
      - POSTGRES_DB=bootimus
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

### Konfigurations-Optionen

Bootimus nutzt sinnvolle Defaults und braucht minimale Konfiguration.

#### Konfigurations-Priorität

1. Command-Line-Flags (höchste Priorität)
2. Environment-Variablen (mit Präfix `BOOTIMUS_`)
3. Konfigurationsdatei (`bootimus.yaml`)

#### Beispiel-Konfigurationsdatei

```yaml
# bootimus.yaml
tftp_port: 69
http_port: 8080
admin_port: 8081
data_dir: ./data          # Basis-Daten-Verzeichnis
server_addr: ""           # Wird auto-erkannt, wenn leer

# Datenbank-Konfiguration (optional)
# Wenn kein db.host angegeben ist, wird automatisch SQLite verwendet
db:
  host: localhost       # Leer lassen für SQLite
  port: 5432
  user: bootimus
  password: bootimus
  name: bootimus
  sslmode: disable
```

#### Environment-Variablen

```bash
# Server-Einstellungen
export BOOTIMUS_TFTP_PORT=69
export BOOTIMUS_HTTP_PORT=8080
export BOOTIMUS_ADMIN_PORT=8081
export BOOTIMUS_DATA_DIR=/var/lib/bootimus/data
export BOOTIMUS_SERVER_ADDR=192.168.1.100

# Datenbank-Einstellungen (nur PostgreSQL)
export BOOTIMUS_DB_HOST=postgres      # Leer = SQLite
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secret
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=disable

./bootimus serve
```

## Fehlersuche

### Permission Denied auf Port 69

```bash
# Als Root starten
sudo ./bootimus serve

# Oder Docker mit NET_BIND_SERVICE-Capability
docker run --cap-add NET_BIND_SERVICE ...

# Oder nicht-privilegierten Port nutzen
./bootimus serve --tftp-port 6969
```

### Datenbank-Verbindung fehlgeschlagen

```bash
# SQLite-Datenbank prüfen
ls -la data/bootimus.db

# Bei PostgreSQL die Verbindung testen
psql -h localhost -U bootimus -d bootimus

# PostgreSQL-Logs prüfen
docker logs bootimus-db
```

### Container im LAN nicht erreichbar

```bash
# Macvlan-Konfiguration prüfen
docker network inspect bootimus_host_bridge

# IP-Adress-Zuweisung prüfen
docker exec bootimus ip addr show

# Routing prüfen
ip route | grep 192.168.1.100

# Firewall prüfen
sudo iptables -L -n | grep 192.168.1.100
```

### Plattenplatz voll

```bash
# Plattenplatz prüfen
df -h /opt/bootimus/data

# Große Dateien finden
du -sh /opt/bootimus/data/*

# Alte ISOs aufräumen
rm /opt/bootimus/data/isos/old-image.iso

# Nach neuen ISOs scannen, um die Datenbank zu aktualisieren
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Nächste Schritte

-  Lies den [Image-Verwaltungs-Leitfaden](images.md) zum Umgang mit ISOs
-  Siehe den [Admin-Konsolen-Leitfaden](admin.md) zur Verwaltung
-  Konfiguriere den [DHCP-Server](dhcp.md) für PXE-Boot
