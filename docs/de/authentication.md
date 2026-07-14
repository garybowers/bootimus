# Authentifizierungs-Leitfaden

Bootimus nutzt JWT-Authentifizierung (JSON Web Token) für das Admin-Panel. Optional kannst du einen LDAP- oder Active-Directory-Server als Authentifizierungs-Backend anbinden.

## Inhaltsverzeichnis

- [Lokale Authentifizierung](#lokale-authentifizierung)
- [Login-Ablauf](#login-ablauf)
- [API-Authentifizierung](#api-authentifizierung)
- [LDAP / Active Directory](#ldap--active-directory)
- [Konfigurations-Referenz](#konfigurations-referenz)
- [Fehlersuche](#fehlersuche)

## Lokale Authentifizierung

Standardmäßig nutzt Bootimus lokale Benutzerkonten, die in der Datenbank (SQLite oder PostgreSQL) gespeichert sind.

### Standard-Admin-Konto

Beim ersten Start wird ein zufälliges Passwort generiert und in die Server-Logs geschrieben:

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

### Admin-Passwort zurücksetzen

Auf ein neues zufälliges Passwort zurücksetzen (startet eine vollständige Server-Instanz):
```bash
./bootimus serve --reset-admin-password
# oder mit Docker
docker exec bootimus /bootimus serve --reset-admin-password
```

Oder ein bestimmtes Passwort direkt in der Datenbank setzen, ohne einen Port
zu binden (ideal für die Notfallwiederherstellung):
```bash
# Interaktive Abfrage (verdeckte Eingabe, hält das Passwort aus der Shell-Historie heraus)
./bootimus user set-password admin

# Oder es für Skripte nicht-interaktiv übergeben
./bootimus user set-password admin --password 'neues-passwort'
```

### Nutzerverwaltung

Zusätzliche Nutzer können im Tab **Users** des Admin-Panels angelegt werden. Jeder Nutzer hat:
- **Username**: Eindeutiger Login-Name
- **Password**: Als bcrypt-Hash gespeichert
- **Admin**: Ob der Nutzer Admin-Rechte hat
- **Enabled**: Kann ohne Löschung deaktiviert werden

Nutzer können auch über die CLI verwaltet werden, ohne den Server zu starten
(nützlich für die Wiederherstellung). Diese Befehle wirken direkt auf die
konfigurierte Datenbank (SQLite oder PostgreSQL):
```bash
./bootimus user list                       # alle lokalen Nutzer auflisten
./bootimus user enable <username>          # ein Konto aktivieren
./bootimus user disable <username>         # ein Konto deaktivieren
./bootimus user set-admin <username>       # Admin-Rechte erteilen
./bootimus user unset-admin <username>     # Admin-Rechte entziehen
./bootimus user set-password <username>    # ein Passwort setzen (Abfrage, oder --password)
```

## Login-Ablauf

1. Navigiere zu `http://your-server:8081`
2. Die Login-Seite zeigt Benutzername- und Passwortfelder
3. Wenn LDAP konfiguriert ist, erscheint ein Auth-Dropdown zur Backend-Auswahl
4. Bei erfolgreichem Login wird ein JWT-Token ausgestellt (24 Stunden gültig)
5. Der Token wird im Browser gespeichert und mit allen API-Requests mitgeschickt
6. Bei Logout oder Token-Ablauf wird wieder die Login-Seite gezeigt

## API-Authentifizierung

Alle API-Endpunkte (außer `/api/login` und `/api/auth-info`) verlangen einen gültigen JWT-Token.

### Token holen

```bash
# Login und Token holen
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.data.token')

echo $TOKEN
```

### Token verwenden

```bash
# In allen API-Requests mitgeben
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients

# Beispiel: Images auflisten
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images
```

### Token-Details

- **Algorithmus**: HMAC-SHA256
- **Ablauf**: 24 Stunden nach Ausstellung
- **Secret**: Zufällig bei jedem Server-Start generiert (alle Token werden bei Neustart ungültig)
- **Claims**: Username, Admin-Status, Ausstellungszeit, Ablauf

### Verfügbare Auth-Backends abfragen

```bash
# Keine Authentifizierung nötig
curl http://localhost:8081/api/auth-info
```

Antwort:
```json
{
  "success": true,
  "data": [
    {"id": "local", "name": "Local"},
    {"id": "ldap", "name": "LDAP (dc.example.com)"}
  ]
}
```

## LDAP / Active Directory

Bootimus unterstützt LDAP-Authentifizierung als zusätzliches Backend. Wenn konfiguriert, können Nutzer auf der Login-Seite zwischen lokaler und LDAP-Authentifizierung wählen. Lokale Konten funktionieren immer als Fallback.

### Funktionsweise

1. Nutzer wählt "LDAP" auf der Login-Seite und gibt Zugangsdaten ein
2. Bootimus verbindet sich mit dem LDAP-Server über das Service-Konto (Bind-DN)
3. Sucht den Nutzer über den konfigurierten Filter
4. Versucht ein Bind als gefundener Nutzer mit dem angegebenen Passwort
5. Bei Erfolg wird die Gruppenmitgliedschaft für Admin-Zugriff geprüft
6. Stellt einen JWT-Token aus (genau wie bei lokaler Auth)

### Active-Directory-Beispiel

```bash
# Environment-Variablen
export BOOTIMUS_LDAP_HOST=dc.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="service-account-password"
export BOOTIMUS_LDAP_USER_FILTER="(sAMAccountName=%s)"
export BOOTIMUS_LDAP_GROUP_FILTER="cn=bootimus-admins"
```

### OpenLDAP-Beispiel

```bash
export BOOTIMUS_LDAP_HOST=ldap.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=readonly,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="readonly-password"
export BOOTIMUS_LDAP_USER_FILTER="(uid=%s)"
```

### LDAPS (TLS)

```bash
export BOOTIMUS_LDAP_HOST=ldaps.example.com
export BOOTIMUS_LDAP_PORT=636
export BOOTIMUS_LDAP_TLS=true

# Oder StartTLS auf Port 389 nutzen
export BOOTIMUS_LDAP_HOST=ldap.example.com
export BOOTIMUS_LDAP_STARTTLS=true

# Zertifikatsprüfung überspringen (nicht für Produktion empfohlen)
export BOOTIMUS_LDAP_SKIP_VERIFY=true
```

### Docker-Compose-Beispiel

```yaml
services:
  bootimus:
    image: garybowers/bootimus:latest
    environment:
      BOOTIMUS_LDAP_HOST: dc.example.com
      BOOTIMUS_LDAP_BASE_DN: dc=example,dc=com
      BOOTIMUS_LDAP_BIND_DN: cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com
      BOOTIMUS_LDAP_BIND_PASSWORD: service-account-password
      BOOTIMUS_LDAP_USER_FILTER: (sAMAccountName=%s)
      BOOTIMUS_LDAP_GROUP_FILTER: cn=bootimus-admins
```

### Admin-Gruppenmitgliedschaft

Wenn `BOOTIMUS_LDAP_GROUP_FILTER` gesetzt ist, bekommen nur Nutzer, die Mitglied der passenden Gruppe sind, Admin-Zugriff. Die Gruppenmitgliedschaft wird geprüft über:

1. Das `memberOf`-Attribut am User-Objekt
2. Eine Gruppen-Suchanfrage, falls `memberOf` nicht verfügbar ist

Wenn `BOOTIMUS_LDAP_GROUP_FILTER` **nicht gesetzt** ist, bekommen alle LDAP-Nutzer Admin-Zugriff.

### Login per API mit LDAP

```bash
# auth_method: "ldap" angeben
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"jdoe","password":"ldap-password","auth_method":"ldap"}' | jq -r '.data.token')
```

## Konfigurations-Referenz

### CLI-Flags

| Flag | Default | Beschreibung |
|------|---------|-------------|
| `--ldap-host` | *(leer)* | LDAP-Server-Hostname (aktiviert LDAP-Auth) |
| `--ldap-port` | `389` | LDAP-Server-Port |
| `--ldap-tls` | `false` | LDAPS verwenden (TLS beim Verbindungsaufbau) |
| `--ldap-starttls` | `false` | StartTLS nach Verbindungsaufbau verwenden |
| `--ldap-skip-verify` | `false` | TLS-Zertifikatsprüfung überspringen |
| `--ldap-bind-dn` | *(leer)* | Service-Konto-DN für die Nutzersuche |
| `--ldap-bind-password` | *(leer)* | Service-Konto-Passwort |
| `--ldap-base-dn` | *(leer)* | Base-DN für die Nutzersuche |
| `--ldap-user-filter` | `(sAMAccountName=%s)` | Nutzer-Suchfilter (`%s` = Username) |
| `--ldap-group-filter` | *(leer)* | Gruppen-CN für Admin-Zugriff |
| `--ldap-group-base-dn` | *(leer)* | Base-DN für die Gruppensuche (Default: Base-DN) |

### Environment-Variablen

Alle Flags lassen sich über Environment-Variablen mit dem Präfix `BOOTIMUS_` setzen:

| Variable | Entspricht |
|----------|---------|
| `BOOTIMUS_LDAP_HOST` | `--ldap-host` |
| `BOOTIMUS_LDAP_PORT` | `--ldap-port` |
| `BOOTIMUS_LDAP_TLS` | `--ldap-tls` |
| `BOOTIMUS_LDAP_STARTTLS` | `--ldap-starttls` |
| `BOOTIMUS_LDAP_SKIP_VERIFY` | `--ldap-skip-verify` |
| `BOOTIMUS_LDAP_BIND_DN` | `--ldap-bind-dn` |
| `BOOTIMUS_LDAP_BIND_PASSWORD` | `--ldap-bind-password` |
| `BOOTIMUS_LDAP_BASE_DN` | `--ldap-base-dn` |
| `BOOTIMUS_LDAP_USER_FILTER` | `--ldap-user-filter` |
| `BOOTIMUS_LDAP_GROUP_FILTER` | `--ldap-group-filter` |
| `BOOTIMUS_LDAP_GROUP_BASE_DN` | `--ldap-group-base-dn` |

### Konfigurationsdatei (bootimus.yaml)

```yaml
ldap:
  host: dc.example.com
  port: 389
  tls: false
  starttls: true
  bind_dn: cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com
  bind_password: service-account-password
  base_dn: dc=example,dc=com
  user_filter: (sAMAccountName=%s)
  group_filter: cn=bootimus-admins
```

## Fehlersuche

### LDAP-Verbindung fehlgeschlagen

Konnektivität und TLS-Einstellungen prüfen:
```bash
# LDAP-Verbindung testen
ldapsearch -H ldap://dc.example.com -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"

# LDAPS testen
ldapsearch -H ldaps://dc.example.com:636 -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"
```

### Nutzer nicht gefunden

Prüfe, ob der User-Filter Ergebnisse liefert:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" dn
```

Gängige Filter:
- Active Directory: `(sAMAccountName=%s)`
- OpenLDAP: `(uid=%s)`
- E-Mail-basiert: `(mail=%s)`

### LDAP-Nutzer ist kein Admin

Gruppenmitgliedschaft prüfen:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" memberOf
```

### Token abgelaufen

JWT-Token sind 24 Stunden gültig. Nach Ablauf wird automatisch die Login-Seite gezeigt. Token werden außerdem ungültig, wenn der Server neu startet (das Signing-Secret wird neu erzeugt).

### Lokaler Admin ausgesperrt

Admin-Passwort zurücksetzen:
```bash
./bootimus serve --reset-admin-password
```

Oder, wenn das Starten des Servers unpraktisch ist (z. B. Portkonflikte), das
Passwort direkt in der Datenbank setzen:
```bash
./bootimus user set-password admin
```

Das funktioniert immer, unabhängig von der LDAP-Konfiguration.
