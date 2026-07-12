#  Leitfaden zur Admin-Konsole

Kompletter Leitfaden zur Nutzung der Bootimus-Admin-Oberfläche und der REST-API.

##  Inhaltsverzeichnis

- [Zugriff auf das Admin-Panel](#zugriff-auf-das-admin-panel)
- [Dashboard](#dashboard)
- [Client-Verwaltung](#client-verwaltung)
- [Image-Verwaltung](#image-verwaltung)
- [Boot-Logs](#boot-logs)
- [REST-API](#rest-api)
- [Automatisierungs-Beispiele](#automatisierungs-beispiele)
- [Security Best Practices](#security-best-practices)

## Zugriff auf das Admin-Panel

### Web-Oberfläche

```
http://your-server:8081/
```

**Voraussetzungen**:
- Admin-Oberfläche läuft auf separatem Port (Default 8081)
- Funktioniert mit SQLite oder PostgreSQL
- Token-basierte JWT-Authentifizierung (mit optionalem LDAP/AD-Backend)

### Erster Login

Beim ersten Start erzeugt Bootimus ein zufälliges Admin-Passwort:

```
╔════════════════════════════════════════════════════════════════╗
║                    ADMIN PASSWORD GENERATED                    ║
╠════════════════════════════════════════════════════════════════╣
║  Username: admin                                               ║
║  Password: AbCdEfGh1234567890-_XyZ123456                       ║
╠════════════════════════════════════════════════════════════════╣
║  This password will NOT be shown again!                        ║
║  Save it now or reset it using --reset-admin-password flag     ║
╚════════════════════════════════════════════════════════════════╝
```

Navigiere zu `http://your-server:8081` — du siehst eine dedizierte Login-Seite. Gib die Admin-Zugangsdaten ein, um ins Panel zu kommen.

**Login-Daten**:
- **Username**: `admin`
- **Passwort**: Aus den Server-Startup-Logs

Wenn LDAP konfiguriert ist, erscheint auf der Login-Seite ein Dropdown zur Auswahl zwischen lokaler und LDAP-Authentifizierung. Details im [Authentifizierungs-Leitfaden](authentication.md).

### Schnellstart

1. Bootimus starten:
   ```bash
   docker-compose up -d
   # ODER
   ./bootimus serve
   ```

2. Admin-Passwort aus den Server-Logs kopieren

3. Browser öffnen auf `http://localhost:8081/`

4. Mit Username `admin` und generiertem Passwort einloggen

## Dashboard

Das Dashboard bietet Echtzeit-Statistiken:

-  **Gesamtzahl Clients** - Alle registrierten Clients
-  **Aktive Clients** - Aktivierte Clients, die booten dürfen
-  **Gesamtzahl Images** - Alle ISO-Images
-  **Aktivierte Images** - Images, die im Boot-Menü verfügbar sind
-  **Gesamtzahl Boots** - Anzahl der Boot-Versuche

Alle Statistiken aktualisieren sich in Echtzeit über WebSocket/SSE.

## Client-Verwaltung

### Client hinzufügen

1. Klicke auf den Button **"Add Client"**
2. MAC-Adresse eingeben (Format: `00:11:22:33:44:55`)
3. Optional Name und Beschreibung hinzufügen
4. **"Enabled"** anhaken, um Boot zuzulassen
5. Auf **"Create Client"** klicken

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Machine 1",
    "description": "Test workstation",
    "enabled": true
  }'
```

### Client bearbeiten

1. Klicke **"Edit"** in einer beliebigen Client-Zeile
2. Name, Beschreibung oder Enabled-Status anpassen
3. ISOs auswählen, auf die dieser Client zugreifen darf (Multi-Select)
4. Auf **"Update Client"** klicken

**Per API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Name",
    "enabled": false
  }'
```

### Client löschen

Klicke **"Delete"** in einer beliebigen Client-Zeile und bestätige das Löschen.

**Per API**:
```bash
curl -u admin:password -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

### Images einem Client zuweisen

**Per Web-Oberfläche**:
1. Klicke **"Edit"** am Client
2. Wähle Images aus dem Multi-Select-Dropdown
3. Klicke **"Update Client"**

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "image_filenames": ["ubuntu-24.04.iso", "debian-12.iso"]
  }'
```

## Image-Verwaltung

### ISO hochladen

**Per Web-Oberfläche**:
1. Klicke auf den Button **"Upload ISO"**
2. ISO-Datei per Drag & Drop oder Klick auswählen
3. Optional Beschreibung hinzufügen
4. **"Public"** anhaken, um sie für alle Clients verfügbar zu machen
5. Klicke **"Upload"**

**Upload-Limit**: 10 GB pro Datei

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Von URL herunterladen

ISOs direkt auf den Server herunterladen:

**Per Web-Oberfläche**:
1. Klicke auf den Button **"Download from URL"**
2. ISO-Download-URL eingeben
3. Beschreibung hinzufügen
4. Klicke **"Download"**

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'

# Fortschritt überwachen
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Kernel/Initrd extrahieren

Boot-Dateien für schnelleres Booten und geringere Bandbreite extrahieren:

**Per Web-Oberfläche**:
1. Image im Tab **Images** finden
2. Auf **"Extract"** klicken
3. Auf Abschluss der Extraktion warten

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'
```

**Vorteile**:
-  Schnelleres Booten (100 MB statt 6 GB Download)
-  Geringere Bandbreite (kritisch bei mehreren Clients)
-  Bessere Kompatibilität (manche ISOs unterstützen sanboot nicht)

Siehe [Image-Verwaltungs-Leitfaden](images.md) für detaillierte Extraktions-Infos.

### Netboot-Dateien herunterladen

Für Debian-/Ubuntu-Installer-ISOs, die Netboot benötigen:

**Per Web-Oberfläche**:
1. Image mit Badge **"Netboot Required"** finden
2. Auf **"Download Netboot"** klicken
3. Auf Download und Extraktion warten

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

**Was sind Netboot-Dateien?**
- Offizielle minimale Boot-Dateien von Debian/Ubuntu
- ~30-50 MB Download (statt komplettem ISO)
- Installer lädt während der Installation Pakete aus dem Internet
- Immer aktuelle Pakete

Siehe [Netboot-Unterstützung](images.md#netboot-support) für Details.

### Nach ISOs scannen

Daten-Verzeichnis nach manuell hinzugefügten ISOs scannen:

**Per Web-Oberfläche**:
1. ISO-Dateien manuell ins Verzeichnis `/data/isos/` kopieren
2. Auf **"Scan for ISOs"** klicken
3. Bootimus erkennt und registriert die neuen ISOs

**Per API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### Image aktivieren/deaktivieren

**Per Web-Oberfläche**:
- **"Enable"** oder **"Disable"** in einer beliebigen Image-Zeile klicken
- Deaktivierte Images erscheinen nicht in Boot-Menüs

**Per API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

### Öffentlich/Privat machen

**Per Web-Oberfläche**:
- **"Make Public"** klicken, damit alle Clients zugreifen dürfen
- **"Make Private"** klicken, um den Zugriff auf zugewiesene Clients zu beschränken

**Per API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": true}'
```

### Image löschen

**Per Web-Oberfläche**:
- **"Delete"** in einer beliebigen Image-Zeile klicken
- Löschvorgang bestätigen
- Image wird aus der Datenbank entfernt
- ISO-Datei bleibt auf der Platte (bei Bedarf manuell löschen)

**Per API**:
```bash
# Nur aus der Datenbank löschen
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso"

# Aus Datenbank und Dateisystem löschen
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso&delete_file=true"
```

## Boot-Logs

Aktuelle Boot-Versuche mit Live-Streaming einsehen:

**Angezeigte Infos**:
-  Zeitstempel
-  Client-MAC-Adresse
-  Image-Name
-  IP-Adresse
- / Erfolgs-/Fehler-Status
-  Fehlermeldungen (falls vorhanden)

**Auto-Refresh**: Logs aktualisieren sich in Echtzeit per SSE (Server-Sent Events)

**Per API**:
```bash
# Letzte 100 Logs holen (Default)
curl -u admin:password http://localhost:8081/api/logs

# Letzte 10 Logs holen
curl -u admin:password http://localhost:8081/api/logs?limit=10

# Letzte 500 Logs holen (max. 1000)
curl -u admin:password http://localhost:8081/api/logs?limit=500
```

## REST-API

Alle Admin-Funktionen sind zur Automatisierung über die REST-API verfügbar.

### Authentifizierung

Für alle Endpunkte ist HTTP Basic Authentication erforderlich:
- **Username**: `admin`
- **Passwort**: Beim ersten Lauf automatisch generiert

```bash
curl -u admin:your-password http://localhost:8081/api/stats
```

### API-Endpunkte

#### Stats

```bash
GET /api/stats
```

**Antwort**:
```json
{
  "success": true,
  "data": {
    "total_clients": 10,
    "active_clients": 8,
    "total_images": 5,
    "enabled_images": 4,
    "total_boots": 127
  }
}
```

#### Clients

| Methode | Endpunkt | Beschreibung |
|--------|----------|-------------|
| `GET` | `/api/clients` | Alle Clients auflisten |
| `GET` | `/api/clients?mac=<MAC>` | Client per MAC abrufen |
| `POST` | `/api/clients` | Client anlegen |
| `PUT` | `/api/clients?mac=<MAC>` | Client aktualisieren |
| `DELETE` | `/api/clients?mac=<MAC>` | Client löschen |
| `POST` | `/api/clients/assign` | Images einem Client zuweisen |

#### Images

| Methode | Endpunkt | Beschreibung |
|--------|----------|-------------|
| `GET` | `/api/images` | Alle Images auflisten |
| `GET` | `/api/images?filename=<name>` | Image abrufen |
| `PUT` | `/api/images?filename=<name>` | Image aktualisieren |
| `DELETE` | `/api/images?filename=<name>` | Image löschen |
| `POST` | `/api/images/upload` | ISO hochladen |
| `POST` | `/api/images/download` | ISO von URL herunterladen |
| `POST` | `/api/images/extract` | Kernel/initrd extrahieren |
| `POST` | `/api/images/netboot/download` | Netboot-Dateien herunterladen |
| `POST` | `/api/scan` | Nach neuen ISOs scannen |

#### Downloads

| Methode | Endpunkt | Beschreibung |
|--------|----------|-------------|
| `GET` | `/api/downloads` | Laufende Downloads auflisten |
| `GET` | `/api/downloads/progress?filename=<name>` | Download-Fortschritt abrufen |

#### Logs

| Methode | Endpunkt | Beschreibung |
|--------|----------|-------------|
| `GET` | `/api/logs?limit=<N>` | Boot-Logs holen |
| `GET` | `/api/logs/stream` | SSE-Stream der Echtzeit-Logs |

## Automatisierungs-Beispiele

### Clients in Bulk hinzufügen

```bash
#!/bin/bash
# bulk-add-clients.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

CLIENTS=(
  "00:11:22:33:44:01:Server1"
  "00:11:22:33:44:02:Server2"
  "00:11:22:33:44:03:Workstation1"
)

for entry in "${CLIENTS[@]}"; do
  IFS=':' read -r mac1 mac2 mac3 mac4 mac5 mac6 name <<< "$entry"
  mac="${mac1}:${mac2}:${mac3}:${mac4}:${mac5}:${mac6}"

  curl -u admin:$ADMIN_PASSWORD -X POST http://localhost:8081/api/clients \
    -H "Content-Type: application/json" \
    -d "{\"mac_address\":\"$mac\",\"name\":\"$name\",\"enabled\":true}"

  echo "Added $name ($mac)"
done
```

### Alle Images öffentlich machen

```bash
#!/bin/bash
# make-all-public.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

images=$(curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/images | jq -r '.data[].filename')

for filename in $images; do
  curl -u admin:$ADMIN_PASSWORD -X PUT "http://localhost:8081/api/images?filename=$filename" \
    -H "Content-Type: application/json" \
    -d '{"public":true}'
  echo "Made $filename public"
done
```

### Boot-Versuche überwachen

```bash
#!/bin/bash
# monitor-boots.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

while true; do
  clear
  echo "=== Recent Boot Attempts ==="
  curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/logs?limit=20 | \
    jq -r '.data[] | "\(.created_at) | \(.mac_address) | \(.image_name) | \(if .success then "" else "✗" end)"'
  sleep 5
done
```

### Statistiken exportieren

```bash
#!/bin/bash
# export-stats.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

echo "Bootimus Usage Report - $(date)"
echo "================================"

stats=$(curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/stats | jq '.data')

echo "Total Clients: $(echo $stats | jq -r '.total_clients')"
echo "Active Clients: $(echo $stats | jq -r '.active_clients')"
echo "Total Images: $(echo $stats | jq -r '.total_images')"
echo "Total Boots: $(echo $stats | jq -r '.total_boots')"

echo -e "\nTop Clients by Boot Count:"
curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/clients | \
  jq -r '.data | sort_by(.boot_count) | reverse | .[:5] | .[] | "\(.boot_count) boots - \(.name // .mac_address)"'
```

## Security Best Practices

### Netzwerk-Isolation

Halte den Admin-Port vom Boot-Netzwerk getrennt:

```bash
# Boot-Traffic (TFTP/HTTP) auf einem Interface erlauben
# Admin-Traffic auf einem anderen Interface oder nur localhost erlauben
```

### Firewall-Regeln

```bash
# Admin-Zugriff nur aus bestimmtem IP-Bereich erlauben
sudo ufw allow from 192.168.1.0/24 to any port 8081

# Oder Admin-Port komplett von außen blockieren
sudo ufw deny 8081
```

### SSH-Tunnel

Zugriff auf die Admin-Oberfläche sicher per SSH-Tunnel:

```bash
# SSH-Tunnel aufbauen
ssh -L 8081:localhost:8081 user@bootimus-server

# Admin-Panel öffnen
open http://localhost:8081/
```

### VPN-Zugriff

- Bootimus-Admin-Port nur ins VPN-Netz legen
- VPN-Verbindung für Admin-Zugriff voraussetzen
- Boot-Ports (69, 8080) in separates Netz-Segment legen

### Passwort-Management

-  Admin-Passwort sicher ablegen (Passwortmanager)
-  Passwort regelmäßig rotieren, indem `.admin_password` gelöscht und neu gestartet wird
- 🛡 Zusätzliche Authentifizierungsschicht erwägen (nginx mit Client-Zertifikaten)

## Fehlersuche

### Admin-Oberfläche lädt nicht

```bash
# Prüfen, ob der Service läuft
docker ps | grep bootimus

# Logs prüfen
docker logs bootimus

# Port-Erreichbarkeit prüfen
curl -u admin:password http://localhost:8081/api/stats

# Firewall prüfen
sudo ufw status | grep 8081
```

### Große ISOs lassen sich nicht hochladen

```bash
# Verfügbaren Speicherplatz prüfen
df -h /opt/bootimus/data

# Upload-Limit ist standardmäßig 10 GB
# Bei größeren ISOs Download per URL oder manuelles Kopieren + Scan nutzen
```

### Änderungen werden nicht übernommen

- Browser hart neu laden (Ctrl+F5 oder Cmd+Shift+R)
- Browser-Konsole auf Fehler prüfen (F12)
- API-Antworten mit curl verifizieren
- Server-Logs auf Detail-Fehler prüfen

### API gibt Fehler zurück

```bash
# Request-Format prüfen (JSON-Content-Type bei POST/PUT)
curl -v -u admin:password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","name":"Test"}'

# Existenz der Ressource für Update/Delete prüfen
curl -u admin:password http://localhost:8081/api/images | jq

# Server-Logs prüfen
docker logs bootimus | tail -50
```

## Nächste Schritte

-  Lies den [Image-Verwaltungs-Leitfaden](images.md) zum Umgang mit ISOs
-  Siehe den [Deployment-Leitfaden](deployment.md) für das Produktiv-Setup
-  Konfiguriere den [DHCP-Server](dhcp.md) für PXE-Boot
-  Richte [Client-Verwaltung](clients.md) für Zugriffskontrolle ein
