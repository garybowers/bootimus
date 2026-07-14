# Distro-Profile-Leitfaden

Bootimus nutzt Distro-Profile, um ISO-Typen zu erkennen und die richtigen Boot-Parameter zu erzeugen. Profile sind datengetrieben — du kannst Unterstützung für neue Distributionen hinzufügen, ohne Code zu ändern.

## Inhaltsverzeichnis

- [Überblick](#überblick)
- [Funktionsweise](#funktionsweise)
- [Profile ansehen](#profile-ansehen)
- [Profile aktualisieren](#profile-aktualisieren)
- [Remote-Updates & Datenschutz](#remote-updates--datenschutz)
- [Eigene Profile erstellen](#eigene-profile-erstellen)
- [Profilfelder](#profilfelder)
- [Platzhalter](#platzhalter)
- [Beispiele](#beispiele)
- [Fehlersuche](#fehlersuche)

## Überblick

Distro-Profile definieren:
- **Wie erkannt wird**, um welche Distro es sich bei einem ISO handelt (Filename-Pattern-Matching)
- **Wo Kernel, initrd und squashfs** im ISO zu finden sind
- **Welche Boot-Parameter** beim PXE-Boot verwendet werden
- **Welcher Auto-Install-Typ** unterstützt wird (preseed, kickstart, autoinstall etc.)

### Profiltypen

| Typ | Beschreibung |
|------|-------------|
| **Built-in** | Mit Bootimus mitgeliefert, aus dem zentralen Repository aktualisiert |
| **Custom** | Vom Nutzer erstellt, wird nie durch Updates überschrieben |

Custom-Profile haben beim Matching von ISO-Dateinamen immer Vorrang vor Built-in-Profilen.

## Funktionsweise

1. Wenn ein ISO hochgeladen oder extrahiert wird, gleicht Bootimus den Dateinamen gegen Profil-Patterns ab
2. Die Kernel-/initrd-Pfade des passenden Profils werden genutzt, um Boot-Dateien im ISO zu finden
3. Die Boot-Parameter des Profils werden zum Default (editierbar in den Image-Eigenschaften)
4. Beim Boot werden Platzhalter in den Parametern zu echten URLs aufgelöst

### Profil-Lebenszyklus

```
Build-Zeit:    distro-profiles.json im Binary eingebettet
                        ↓
Erster Start:  Profile in die Datenbank geseedet
                        ↓
"Auf Updates prüfen":  Neueste Profile von GitHub geholt
                        ↓
Nutzer legt an:   Custom-Profile in der DB gespeichert (nie überschrieben)
```

## Profile ansehen

Navigiere im Admin-Panel zu **Boot > Distro-Profile**, um alle geladenen Profile mit ihren Filename-Patterns, Boot-Parametern, Typ (Built-in/Custom) und Version zu sehen.

## Profile aktualisieren

Das Aktualisieren von Profilen ist immer eine **explizite, bedarfsgesteuerte Aktion** — Bootimus kontaktiert den Remote-Katalog niemals von sich aus. Bis du ein Update auslöst, werden die zur Build-Zeit ins Binary eingebetteten Profile verwendet. Unter [Remote-Updates & Datenschutz](#remote-updates--datenschutz) findest du genau, was kontaktiert wird und wie du es deaktivierst.

Wenn du ein Update auslöst:

- Neue Profile werden hinzugefügt
- Bestehende Built-in-Profile werden auf die neueste Version aktualisiert
- Custom-Profile werden nie verändert

Es gibt drei Wege, es auszulösen:

### Über das Admin-UI

Klicke im Tab **Boot > Distro-Profile** auf **"Auf Updates prüfen"**.

### Über die CLI

```bash
bootimus profiles update
```

Dies nutzt dieselbe Datenbank-Konfiguration wie `serve` (PostgreSQL, wenn `db.host` gesetzt ist, andernfalls die lokale SQLite-Datenbank unter `data_dir`). Es beachtet `--disable-remote-profiles` und beendet sich ohne Netzwerkkontakt, wenn dieses Flag gesetzt ist.

### Per API

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/profiles/update
```

Antwort:
```json
{
  "success": true,
  "message": "Updated to version 0.1.21 (2 added, 5 updated)"
}
```

## Remote-Updates & Datenschutz

Bootimus ist selbst gehostet und funkt nicht im Hintergrund nach Hause. Der einzige Zeitpunkt, zu dem es für Profile einen externen Dienst kontaktiert, ist, wenn **du** über eine der oben genannten Methoden explizit ein Update auslöst.

**Kontaktierter Endpunkt (nur bei einem expliziten Update):**

```
https://raw.githubusercontent.com/garybowers/bootimus/main/distro-profiles.json
```

Der entsprechende Tools-Katalog ("Auf Updates prüfen" im Tab **Tools** / `POST /api/tools/update`) verhält sich genauso und kontaktiert:

```
https://raw.githubusercontent.com/garybowers/bootimus/main/tools-profiles.json
```

Dies sind einfache, nicht authentifizierte `GET`-Anfragen an GitHubs statischen Datei-Host. Bootimus sendet dabei keine Systeminformationen, Kennungen oder Nutzungsdaten mit — es lädt lediglich die JSON-Datei herunter. Beachte, dass GitHub — wie bei jeder HTTP-Anfrage — deine Quell-IP-Adresse und die üblichen Anfrage-Metadaten sieht.

### Remote-Updates deaktivieren

Um sicherzustellen, dass Bootimus niemals den Remote-Katalog kontaktiert — für air-gapped Deployments oder aus Prinzip —, starte es mit:

```bash
bootimus serve --disable-remote-profiles
```

oder setze den entsprechenden Config-/Env-Wert:

```yaml
# bootimus.yaml
disable_remote_profiles: true
```

```bash
# environment variable
BOOTIMUS_DISABLE_REMOTE_PROFILES=true
```

Wenn deaktiviert, werden die eingebetteten Profile beim ersten Start weiterhin aus dem Binary geseedet, sodass Bootimus offline voll funktionsfähig ist. Der Button "Auf Updates prüfen", der Endpunkt `/api/profiles/update` und `bootimus profiles update` verweigern dann allesamt die Ausführung.

## Eigene Profile erstellen

### Per Web-Oberfläche

1. Gehe zu **Boot > Distro-Profile**
2. Klicke auf **"+ Custom-Profil hinzufügen"**
3. Fülle die Profilfelder aus
4. Klicke auf **"Profil erstellen"**

### Per API

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/profiles/save \
  -H "Content-Type: application/json" \
  -d '{
    "profile_id": "my-distro",
    "display_name": "My Custom Distro",
    "family": "debian",
    "filename_patterns": ["mydistro", "my-distro"],
    "kernel_paths": ["/live/vmlinuz", "/boot/vmlinuz"],
    "initrd_paths": ["/live/initrd.img", "/boot/initrd"],
    "squashfs_paths": ["/live/filesystem.squashfs"],
    "default_boot_params": "boot=live initrd=initrd ip=dhcp",
    "boot_params_with_squashfs": "boot=live initrd=initrd fetch={{SQUASHFS}}",
    "auto_install_type": "preseed"
  }'
```

### Custom-Profile löschen

Nur Custom-Profile können gelöscht werden. Built-in-Profile werden beim nächsten Update wiederhergestellt.

```bash
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/profiles/delete?id=my-distro"
```

## Profilfelder

| Feld | Pflicht | Beschreibung |
|-------|----------|-------------|
| `profile_id` | Ja | Eindeutige Kennung (z.B. `ubuntu`, `my-distro`) |
| `display_name` | Ja | Menschlich lesbarer Name, der im UI angezeigt wird |
| `family` | Nein | Distro-Familie (z.B. `debian`, `arch`, `redhat`) — zur Gruppierung |
| `filename_patterns` | Ja | Teilstrings, die in ISO-Dateinamen gematcht werden (case-insensitive) |
| `kernel_paths` | Nein | Pfade, die für den Kernel im ISO probiert werden (z.B. `/casper/vmlinuz`) |
| `initrd_paths` | Nein | Pfade, die für das initrd im ISO probiert werden |
| `squashfs_paths` | Nein | Pfade, die für das squashfs-Root-Dateisystem probiert werden |
| `default_boot_params` | Nein | Standard-Kernel-Boot-Parameter (mit Platzhalter-Unterstützung) |
| `boot_params_with_squashfs` | Nein | Alternative Boot-Parameter, wenn ein squashfs erkannt wird |
| `auto_install_type` | Nein | Auto-Install-Format: `preseed`, `kickstart`, `autoinstall`, `autounattend` |
| `boot_method` | Nein | Boot-Methode überschreiben (z.B. `wimboot` für Windows) |

## Platzhalter

Boot-Parameter unterstützen diese Platzhalter, die zum Boot-Zeitpunkt aufgelöst werden:

| Platzhalter | Wird aufgelöst zu | Beispiel |
|-------------|-------------|---------|
| `{{BASE_URL}}` | Server-HTTP-URL | `http://192.168.1.10:8080` |
| `{{CACHE_DIR}}` | Verzeichnis der extrahierten Dateien | `ubuntu-24.04-server-amd64` |
| `{{FILENAME}}` | ISO-Dateiname (URL-encoded) | `ubuntu-24.04-server-amd64.iso` |
| `{{SQUASHFS}}` | Vollständige URL zur squashfs-Datei | `http://192.168.1.10:8080/boot/ubuntu.../casper/filesystem.squashfs` |

### Beispiel mit Platzhaltern

```
boot=live initrd=initrd fetch={{SQUASHFS}} ip=dhcp
```

Wird aufgelöst zu:
```
boot=live initrd=initrd fetch=http://192.168.1.10:8080/boot/debian-live-13/live/filesystem.squashfs ip=dhcp
```

## Beispiele

### Debian-basiertes Live-ISO

```json
{
  "profile_id": "my-debian-live",
  "display_name": "My Debian Live Spin",
  "family": "debian",
  "filename_patterns": ["my-debian"],
  "kernel_paths": ["/live/vmlinuz"],
  "initrd_paths": ["/live/initrd.img"],
  "squashfs_paths": ["/live/filesystem.squashfs"],
  "default_boot_params": "initrd=initrd boot=live priority=critical",
  "boot_params_with_squashfs": "initrd=initrd boot=live priority=critical fetch={{SQUASHFS}}"
}
```

### Arch-basierte Distro

```json
{
  "profile_id": "my-arch-spin",
  "display_name": "My Arch Spin",
  "family": "arch",
  "filename_patterns": ["myarch"],
  "kernel_paths": ["/arch/boot/x86_64/vmlinuz-linux", "/boot/vmlinuz-linux"],
  "initrd_paths": ["/arch/boot/x86_64/initramfs-linux.img", "/boot/initramfs-linux.img"],
  "squashfs_paths": ["/arch/x86_64/airootfs.sfs"],
  "default_boot_params": "archisobasedir=arch archiso_http_srv={{BASE_URL}}/boot/{{CACHE_DIR}}/iso/ ip=dhcp"
}
```

### RHEL-basierter Installer

```json
{
  "profile_id": "my-rhel-clone",
  "display_name": "My RHEL Clone",
  "family": "redhat",
  "filename_patterns": ["myrhel"],
  "kernel_paths": ["/images/pxeboot/vmlinuz"],
  "initrd_paths": ["/images/pxeboot/initrd.img"],
  "default_boot_params": "root=live:{{BASE_URL}}/isos/{{FILENAME}} rd.live.image inst.repo={{BASE_URL}}/boot/{{CACHE_DIR}}/iso/ rd.neednet=1 ip=dhcp",
  "auto_install_type": "kickstart"
}
```

## Fehlersuche

### ISO wird nicht als korrekte Distro erkannt

Prüfe, ob der ISO-Dateiname zu einem Profil-Pattern passt:

1. Gehe in den Tab **Distro-Profile**
2. Sieh dir die Spalte "Filename-Patterns" an
3. Wenn kein Pattern auf deinen ISO-Dateinamen passt, lege ein Custom-Profil an

### Boot-Parameter falsch nach Extraktion

1. Öffne die **Eigenschaften** des Images
2. Klicke neben Boot-Parameter auf **"Re-detect"**
3. Oder editiere die Boot-Parameter manuell — sie unterstützen Platzhalter

### "Auf Updates prüfen" fehlgeschlagen

Das Update holt von GitHub. Prüfe:
- Server hat Internetzugang
- `raw.githubusercontent.com` ist nicht blockiert
- Versuche es später nochmal, falls GitHub down ist

### Custom-Profil matcht nicht

Custom-Profile haben Vorrang vor Built-in-Profilen. Stelle sicher, dass:
- Die `filename_patterns` Teilstrings enthalten, die auf deinen ISO-Dateinamen passen (case-insensitive)
- Die Profil-ID eindeutig ist
- Das Profil erfolgreich gespeichert wurde

### Profile beitragen

Um ein Profil zur offiziellen Liste für alle Nutzer hinzuzufügen:
1. Forke das [Bootimus-Repository](https://github.com/garybowers/bootimus)
2. Editiere `distro-profiles.json` im Repo-Root
3. Füge dein Profil zum `profiles`-Array hinzu
4. Reiche einen Pull Request ein

So bekommen alle Bootimus-Nutzer das neue Profil über "Auf Updates prüfen".
