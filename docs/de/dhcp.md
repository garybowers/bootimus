#  DHCP-Konfigurations-Leitfaden

Kompletter Leitfaden zur Konfiguration verschiedener DHCP-Server für PXE-Netzwerk-Boot mit Bootimus.

##  Inhaltsverzeichnis

- [Eingebautes proxyDHCP (Standalone-Modus)](#eingebautes-proxydhcp-standalone-modus)
- [Überblick](#überblick)
- [ISC DHCP Server](#isc-dhcp-server)
- [Dnsmasq](#dnsmasq)
- [MikroTik RouterOS](#mikrotik-routeros)
- [Ubiquiti EdgeRouter](#ubiquiti-edgerouter)
- [pfSense](#pfsense)
- [OPNsense](#opnsense)
- [Windows Server DHCP](#windows-server-dhcp)
- [Fehlersuche](#fehlersuche)
- [Pi-hole](#pi-hole-dnsmasq)

## Eingebautes proxyDHCP (Standalone-Modus)

Bootimus bringt einen eingebauten proxyDHCP-Responder mit (RFC 4578). Aktiviert beantwortet Bootimus die PXE-spezifischen DHCP-Optionen selbst — **dein bestehender DHCP-Server braucht keinerlei PXE-Konfiguration**. Er verteilt weiter IPs wie gehabt; Bootimus antwortet PXE-Clients nur mit `next-server`, Bootfile und der PXE-Vendor-Klasse, und bietet selbst nie eine IP-Adresse an.

Das ist der einfachste Weg, Bootimus in jeder Umgebung zu betreiben, in der dir der Haupt-DHCP-Server nicht gehört — oder du ihn nicht anfassen willst.

### Funktionsweise

1. Client broadcasted `DHCPDISCOVER`.
2. Der bestehende DHCP-Server im LAN antwortet mit einem IP-Lease (keine PXE-Info nötig).
3. Bootimus antwortet auf demselben Broadcast nur mit PXE-Boot-Info — keine IP.
4. Das PXE-ROM des Clients merged beide Antworten: IP vom Haupt-DHCP, Bootfile von Bootimus.

Weil Bootimus nie eine IP anbietet, gibt es keinen Konflikt mit dem bestehenden DHCP-Server und keinen Lease-Pool zu koordinieren.

### Aktivieren

```bash
# CLI-Flag
bootimus serve --proxy-dhcp

# Environment-Variable
BOOTIMUS_PROXY_DHCP_ENABLED=true

# YAML-Config
proxy_dhcp:
  enabled: true
  bootfile_bios: undionly.kpxe           # Legacy BIOS PXE
  bootfile_uefi: bootimus.efi            # UEFI x64
  bootfile_arm64: bootimus-arm64.efi     # UEFI ARM64
```

Standardmäßig aus, damit bestehende Installationen, die schon dnsmasq/ISC-DHCP/etc. nutzen, nicht überrascht werden.

### Voraussetzungen und Vorbehalte

- **Bindet UDP/67 und UDP/4011.** UDP/67 braucht `CAP_NET_BIND_SERVICE` oder Root; UDP/4011 ist der PXE-Boot-Server-Discovery-Port, auf den manche UEFI-ROMs (vor allem AMI/Supermicro) nach dem ersten Offer noch ansprechen. In Docker läuft das Default-Image schon als Root, also keine Extra-Capability nötig; `--cap-add NET_BIND_SERVICE` nur bei Rootless-Setups.
- **Gleiche Broadcast-Domäne.** proxyDHCP ist darauf angewiesen, die DHCP-Broadcasts der Clients zu sehen. Das funktioniert auf einem flachen LAN oder einem einzelnen VLAN. Wenn dein Netz einen DHCP-Relay (`ip helper-address`) nutzt, um DHCP über VLANs zu forwarden, leitet der Relay zum Haupt-DHCP, aber nicht zu Bootimus weiter — füge Bootimus als zusätzliches Relay-Ziel hinzu, oder halte Ziele im selben VLAN wie Bootimus.
- **Docker-Networking.** Nutze `macvlan`, `ipvlan` oder `network_mode: host`, damit der Container ein vollwertiger Teilnehmer auf der Broadcast-Domäne ist. Ein Default-Bridge-Netzwerk funktioniert nicht — Broadcasts hängen in `docker0` fest.
- **Zwei proxyDHCP-Server im selben LAN sind erlaubt, aber das Debugging ist ein Albtraum.** Wenn du das eingebaute proxyDHCP von Bootimus aktivierst, deaktiviere alle bestehenden dnsmasq-proxyDHCPs, die im selben Netz PXE ausspielen.

### docker-compose-Beispiel

```yaml
services:
  bootimus:
    image: garybowers/bootimus:latest
    environment:
      BOOTIMUS_PROXY_DHCP_ENABLED: "true"
      BOOTIMUS_SERVER_ADDR: 10.76.42.41    # die Macvlan-IP des Containers
    ports:
      - "67:67/udp"                         # proxyDHCP
      - "4011:4011/udp"                     # PXE Boot-Server-Discovery
      - "69:69/udp"                         # TFTP
      - "8080:8080/tcp"
      - "8081:8081/tcp"
    networks:
      lan:
        ipv4_address: 10.76.42.41
networks:
  lan:
    driver: macvlan
    driver_opts:
      parent: eth0
    ipam:
      config:
        - subnet: 10.76.42.0/24
```

### Verifizieren

Die Container-Logs sollten zeigen:

```
proxyDHCP: listening on UDP/67 + UDP/4011, advertising next-server=10.76.42.41 (BIOS=undionly.kpxe, UEFI=bootimus.efi, ARM64=bootimus-arm64.efi)
```

Und Per-Client-Zeilen, wenn ein PXE-Boot passiert:

```
proxyDHCP: DISCOVER -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
proxyDHCP: REQUEST  -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
TFTP: Client requesting file: bootimus.efi
```

Das Panel **Server Information** im Admin-UI zeigt ebenfalls den aktuellen proxyDHCP-Status.

---

## Überblick

Um PXE-Netzwerk-Boot zu aktivieren, muss dein DHCP-Server konfiguriert sein, um:

1. **IP-Adressen zu verteilen** an Clients (Standard-DHCP)
2. **Auf den Boot-Server zu zeigen** (`next-server` oder DHCP-Option 66)
3. **Den Bootloader-Dateinamen anzugeben** (DHCP-Option 67)
4. **iPXE zu erkennen** und ins HTTP-Menü zu chainen (optional, aber empfohlen)

**Ersetze `192.168.1.10` in allen Beispielen unten durch die IP-Adresse deines Bootimus-Servers.**

### Bootloader-Dateinamen

| Client-Typ | DHCP-Dateiname | Hinweise |
|-------------|---------------|-------|
| UEFI (x86_64) | `bootimus.efi` (oder `ipxe.efi`) | Custom-Build von iPXE mit eingebettetem Skript |
| UEFI (ARM64) | `bootimus-arm64.efi` (oder `ipxe-arm64.efi`) | Custom-Build von iPXE mit eingebettetem Skript |
| Legacy BIOS | `undionly.kpxe` | Standard-PXE-Bootloader |

> **Secure Boot:** aktiviere für Clients mit eingeschaltetem Secure Boot das eingebaute Bootloader-Set `secureboot-official` und advertise stattdessen `ipxe-shimx64.efi` (x86_64) bzw. `ipxe-shimaa64.efi` (ARM64) als UEFI-Bootfile. Das eingebaute proxyDHCP macht das automatisch, wenn das Set aktiv ist. Siehe den [Secure-Boot-Leitfaden](secure-boot.md).

### Boot-Ablauf

```
Client → DHCP Request
      ← DHCP Offer (IP, next-server, Bootloader-Dateiname)
Client → TFTP-Request für Bootloader (bootimus.efi oder undionly.kpxe)
      ← Bootloader heruntergeladen
Client → HTTP-Request für menu.ipxe
      ← Boot-Menü angezeigt
Client → Gewähltes ISO booten
```

## ISC DHCP Server

ISC DHCP ist der Standard-DHCP-Server auf den meisten Linux-Distributionen.

### Konfiguration

`/etc/dhcp/dhcpd.conf` editieren:

```conf
# Basis-DHCP-Konfiguration
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    option routers 192.168.1.1;
    option domain-name-servers 8.8.8.8, 8.8.4.4;

    # PXE-Boot-Server
    next-server 192.168.1.10;  # Bootimus-Server-IP

    # Prüfen, ob der Client bereits iPXE fährt
    if exists user-class and option user-class = "iPXE" {
        # Client hat iPXE, ins HTTP-Menü chainen
        filename "http://192.168.1.10:8080/menu.ipxe";
    }
    # UEFI-Systeme (x86_64)
    elsif option arch = 00:07 {
        filename "ipxe.efi";
    }
    # UEFI-Systeme (alternativ)
    elsif option arch = 00:09 {
        filename "ipxe.efi";
    }
    # Legacy BIOS
    else {
        filename "undionly.kpxe";
    }
}
```

### Fortgeschritten: Per-Client-Boot-Konfiguration

```conf
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    next-server 192.168.1.10;

    # Spezifische Client-Konfiguration
    host lab-server-1 {
        hardware ethernet 00:11:22:33:44:55;
        fixed-address 192.168.1.50;
        filename "ipxe.efi";
    }

    # Default-Konfiguration für andere Clients
    if exists user-class and option user-class = "iPXE" {
        filename "http://192.168.1.10:8080/menu.ipxe";
    }
    elsif option arch = 00:07 or option arch = 00:09 {
        filename "ipxe.efi";
    }
    else {
        filename "undionly.kpxe";
    }
}
```

### Service neu starten

```bash
# Konfiguration testen
sudo dhcpd -t -cf /etc/dhcp/dhcpd.conf

# DHCP-Service neu starten
sudo systemctl restart isc-dhcp-server

# Status prüfen
sudo systemctl status isc-dhcp-server

# Logs ansehen
sudo journalctl -u isc-dhcp-server -f
```

## Dnsmasq

Dnsmasq ist ein leichtgewichtiger DHCP- und DNS-Server, beliebt auf Embedded-Systemen und Routern.

### Konfiguration

`/etc/dnsmasq.conf` editieren:

```conf
# DHCP-Range
dhcp-range=192.168.1.100,192.168.1.200,12h

# Default-Gateway
dhcp-option=3,192.168.1.1

# DNS-Server
dhcp-option=6,8.8.8.8,8.8.4.4

# PXE-Boot-Konfiguration
dhcp-boot=tag:!ipxe,undionly.kpxe,192.168.1.10
dhcp-boot=tag:ipxe,http://192.168.1.10:8080/menu.ipxe

# UEFI-Unterstützung
dhcp-match=set:efi-x86_64,option:client-arch,7
dhcp-match=set:efi-x86_64,option:client-arch,9
dhcp-boot=tag:efi-x86_64,tag:!ipxe,ipxe.efi,192.168.1.10

# Legacy-BIOS-Unterstützung
dhcp-match=set:bios,option:client-arch,0
dhcp-boot=tag:bios,tag:!ipxe,undionly.kpxe,192.168.1.10

# TFTP-Server aktivieren (optional, falls dnsmasq als TFTP-Server fungieren soll)
# enable-tftp
# tftp-root=/var/lib/tftpboot
```

### Minimale Konfiguration

Wenn du nur Basis-PXE ohne iPXE-Erkennung willst:

```conf
dhcp-range=192.168.1.100,192.168.1.200,12h
dhcp-option=3,192.168.1.1
dhcp-option=6,8.8.8.8

# TFTP-Server und Boot-Datei
dhcp-boot=undionly.kpxe,192.168.1.10
```

### Service neu starten

```bash
# Konfiguration testen
sudo dnsmasq --test

# Service neu starten
sudo systemctl restart dnsmasq

# Status prüfen
sudo systemctl status dnsmasq

# Logs ansehen
sudo journalctl -u dnsmasq -f
```

## MikroTik RouterOS

MikroTik-Router sind wegen ihrer Flexibilität und Performance beim Netzwerk-Boot beliebt.

### Per Web-Oberfläche (WebFig)

#### 1. DHCP-Optionen definieren
* Zu **IP** > **DHCP Server** > **Options** navigieren.
* Für jeden der folgenden Einträge **Add New** klicken (der **Value** muss **einfache Anführungszeichen** enthalten):
    * **Option 66 (Server)**: Name: `tftp-server` | Code: `66` | Value: `'<BOOT_SERVER_IP>'`.
    * **Option 67 (BIOS)**: Name: `boot-bios` | Code: `67` | Value: `'undionly.kpxe'`.
    * **Option 67 (UEFI)**: Name: `boot-uefi` | Code: `67` | Value: `'ipxe.efi'`.

#### 2. Option Sets anlegen
* Zu **IP** > **DHCP Server** > **Option Sets** navigieren.
* **BIOS-Set**: **Add New** klicken, Name: `set-bios`, dann `tftp-server` und `boot-bios` hinzufügen.
* **UEFI-Set**: **Add New** klicken, Name: `set-uefi`, dann `tftp-server` und `boot-uefi` hinzufügen.

#### 3. Option Matcher konfigurieren (Erkennungslogik)
* Zu **IP** > **DHCP Server** > **Option Matcher** navigieren.
* **BIOS-Entry**: Name: `match-bios` | Code: `93` | Value: `0x0000` | Option Set: `set-bios` | Server: `<DHCP_SERVER_NAME>`.
* **UEFI-Entry**: Name: `match-uefi-7` | Code: `93` | Value: `0x0007` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.
* **UEFI-Alt-Entry**: Name: `match-uefi-9` | Code: `93` | Value: `0x0009` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.

#### 4. DHCP-Netzwerk-Konfiguration
* Zu **IP** > **DHCP Server** > **Networks** navigieren.
* Eintrag für dein Subnetz öffnen (z.B. `192.168.88.0/24`).
* **Next Server**: Trage deine `<BOOT_SERVER_IP>` ein.
* **Boot File Name**: **LEER LASSEN** (Der Option Matcher injiziert den Dateinamen dynamisch).

---

### Per Command Line (CLI)



Ersetze die Platzhalter `<BOOT_SERVER_IP>`, `<DHCP_SERVER_NAME>` und `<YOUR_SUBNET>` vor der Ausführung durch deine konkreten Werte.

```routeros
# 1. DHCP-Optionen definieren
/ip dhcp-server option
add code=66 name=tftp-server value="'<BOOT_SERVER_IP>'"
add code=67 name=boot-bios value="'undionly.kpxe'"
add code=67 name=boot-uefi value="'ipxe.efi'"

# 2. Option Sets anlegen
/ip dhcp-server option sets
add name=set-bios options=tftp-server,boot-bios
add name=set-uefi options=tftp-server,boot-uefi

# 3. Option Matcher für Architektur-Erkennung anlegen
/ip dhcp-server option-matcher
add code=93 name=match-bios option-set=set-bios server=<DHCP_SERVER_NAME> value=0x0000
add code=93 name=match-uefi-7 option-set=set-uefi server=<DHCP_SERVER_NAME> value=0x0007
add code=93 name=match-uefi-9 option-set=set-uefi server=<DHCP_SERVER_NAME> value=0x0009

# 4. Auf DHCP-Netzwerk anwenden
/ip dhcp-server network
set [find address="<YOUR_SUBNET>"] boot-file-name="" next-server=<BOOT_SERVER_IP>
```

## Ubiquiti EdgeRouter

Ubiquiti-EdgeRouter nutzen EdgeOS (basiert auf Vyatta/VyOS).

### Per Web-UI

1. Zu **Services > DHCP Server** navigieren
2. Deinen DHCP-Server auswählen (z.B. `LAN`)
3. Unter **Actions** auf **Edit** klicken
4. Zu **PXE Settings** scrollen:
   - **Boot File**: `undionly.kpxe` (BIOS) oder `ipxe.efi` (UEFI)
   - **Boot Server**: `192.168.1.10`
5. Auf **Save** klicken

### Per CLI

```bash
configure

# TFTP-Server für Netzwerk-Boot setzen
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-server 192.168.1.10
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-name undionly.kpxe

# Fortgeschritten: UEFI-Unterstützung
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "option arch code 93 = unsigned integer 16;"
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "if option arch = 00:07 { filename &quot;ipxe.efi&quot;; } else { filename &quot;undionly.kpxe&quot;; }"

commit
save
exit
```

**Hinweis**: `LAN` durch deinen tatsächlichen Shared-Network-Namen ersetzen, falls anders.

### Konfiguration verifizieren

```bash
show service dhcp-server
show service dhcp-server leases
```

## pfSense

pfSense ist eine beliebte Open-Source-Firewall- und Router-Distribution.

### Konfigurations-Schritte

1. Zu **Services > DHCP Server** navigieren
2. Interface auswählen (z.B. **LAN**)
3. Zu **Network Booting** scrollen
4. Konfigurieren:
   - **Enable Network Booting**:  Anhaken
   - **Next Server**: `192.168.1.10`
   - **Default BIOS Filename**: `undionly.kpxe`
   - **UEFI 64-bit Filename**: `ipxe.efi`
5. **Save** klicken

### Fortgeschritten: Custom-Optionen

Für iPXE-Erkennung custom DHCP-Optionen hinzufügen:

1. Zu **Services > DHCP Server** navigieren
2. Interface auswählen
3. Zu **Additional BOOTP/DHCP Options** scrollen
4. Optionen hinzufügen:

```
# Option 60 (Class Identifier)
60 text "PXEClient"

# Option 66 (TFTP Server)
66 text "192.168.1.10"

# Option 67 (Bootfile Name)
67 text "undionly.kpxe"
```

### Statische DHCP-Mappings

Für bestimmte Clients:

1. **Services > DHCP Server > LAN**
2. Zu **DHCP Static Mappings** scrollen
3. **Add** klicken
4. Konfigurieren:
   - **MAC Address**: `00:11:22:33:44:55`
   - **IP Address**: `192.168.1.50`
   - **Filename**: `ipxe.efi`
   - **Root Path**: Leer lassen
5. **Save** klicken

## OPNsense

OPNsense ist ein pfSense-Fork mit modernem Interface.

### Konfigurations-Schritte

1. Zu **Services > DHCPv4 > [Interface]** navigieren
2. Zu **Network Booting** scrollen
3. Konfigurieren:
   - **Enable Network Booting**:  Anhaken
   - **Next Server**: `192.168.1.10`
   - **Default BIOS Filename**: `undionly.kpxe`
   - **UEFI 64-bit Filename**: `ipxe.efi`
4. **Save** klicken
5. **Apply Changes** klicken

> **ARM64 / Raspberry Pi:** die OPNsense-GUI hat kein ARM64-Dateinamen-Feld und kann die Vendor-Option der Raspberry-Pi-Firmware nicht ausdrücken. Aktiviere für Raspberry Pis oder Flotten mit gemischten Architekturen Bootimus' eingebautes proxyDHCP parallel zu OPNsense — es beantwortet diese Clients selbst. Siehe den [Raspberry-Pi-Leitfaden](raspberry-pi.md).

### Erweiterte Konfiguration

1. Zu **Services > DHCPv4 > [Interface]** navigieren
2. Tab **Additional Options** öffnen
3. Custom-Optionen analog zu pfSense hinzufügen

## Windows Server DHCP

Windows-Server-DHCP-Dienst.

### Konfigurations-Schritte

1. **DHCP Manager** öffnen (`dhcpmgmt.msc`)
2. DHCP-Server ausklappen
3. **IPv4** ausklappen
4. Rechtsklick auf **Scope** → **Scope Options**
5. Konfigurieren:
   - **066 Boot Server Host Name**: `192.168.1.10`
   - **067 Bootfile Name**: `undionly.kpxe`

### Fortgeschritten: UEFI- und BIOS-Erkennung

1. Rechtsklick auf **Scope** → **Set Predefined Options**
2. **Add** klicken
3. Option-Code 60 (Vendor Class) anlegen:
   - **Code**: 60
   - **Name**: Vendor Class
   - **Data Type**: String
4. Policies für UEFI/BIOS anlegen:
   - Rechtsklick auf **Policies** → **New Policy**
   - **Bedingung**: Vendor Class equals "PXEClient:Arch:00007" (UEFI)
   - **Optionen**: Bootfile auf `ipxe.efi` setzen
   - Für BIOS wiederholen (Arch:00000) mit `undionly.kpxe`

### PowerShell-Konfiguration

```powershell
# DHCP-Scope-Optionen setzen
Set-DhcpServerv4OptionValue -ScopeId 192.168.1.0 -OptionId 66 -Value "192.168.1.10"
Set-DhcpServerv4OptionValue -ScopeId 192.168.1.0 -OptionId 67 -Value "undionly.kpxe"

# Policy für UEFI anlegen
Add-DhcpServerv4Policy -Name "UEFI" -Condition OR -VendorClass EQ "PXEClient:Arch:00007"
Set-DhcpServerv4OptionValue -PolicyName "UEFI" -OptionId 67 -Value "ipxe.efi"

# Policy für BIOS anlegen
Add-DhcpServerv4Policy -Name "BIOS" -Condition OR -VendorClass EQ "PXEClient:Arch:00000"
Set-DhcpServerv4OptionValue -PolicyName "BIOS" -OptionId 67 -Value "undionly.kpxe"
```

## Fehlersuche

### Client erhält kein DHCP-Offer

```bash
# DHCP-Server-Logs prüfen
sudo journalctl -u isc-dhcp-server -f   # ISC DHCP
sudo journalctl -u dnsmasq -f           # Dnsmasq

# Prüfen, ob der DHCP-Server läuft
sudo systemctl status isc-dhcp-server
sudo systemctl status dnsmasq

# Netzwerk-Konnektivität prüfen
ping 192.168.1.10

# DHCP-Traffic mitschneiden
sudo tcpdump -i eth0 port 67 or port 68
```

### Client lädt den Bootloader nicht

```bash
# Prüfen, ob der Bootimus-TFTP-Server läuft
sudo netstat -ulnp | grep :69

# TFTP manuell testen
tftp 192.168.1.10
> get undionly.kpxe
> quit

# Bootimus-Logs prüfen
docker logs bootimus | grep TFTP
```

### iPXE lädt, aber kein Menü

```bash
# Prüfen, ob der HTTP-Server läuft
curl http://192.168.1.10:8080/menu.ipxe

# Prüfen, ob ISOs verfügbar sind
curl -u admin:password http://192.168.1.10:8081/api/images

# Prüfen, ob der Client Netzwerkzugriff auf den HTTP-Port hat
telnet 192.168.1.10 8080

# Bootimus-Logs prüfen
docker logs bootimus -f
```

### Falscher Bootloader (UEFI vs. BIOS)

```bash
# Firmware-Modus des Clients in DHCP-Logs prüfen
sudo journalctl -u isc-dhcp-server | grep -i "arch"

# UEFI-Clients senden Option 93 mit Wert 00:07 oder 00:09
# BIOS-Clients senden Option 93 mit Wert 00:00

# Sicherstellen, dass die DHCP-Konfiguration Architektur-Erkennung macht
```

### Client bootet, zeigt aber "No Bootable Device"

Mögliche Ursachen:
1. **DHCP-Option 67 falsch**: Sollte `undionly.kpxe` oder `ipxe.efi` sein
2. **Next-Server nicht gesetzt**: DHCP-Option 66 muss auf Bootimus zeigen
3. **TFTP-Port blockiert**: Firewall blockt Port 69
4. **iPXE-Chainloading fehlgeschlagen**: HTTP-Port 8080 nicht erreichbar

**Lösung**:
```bash
# Alle Ports erreichbar machen
sudo ufw allow 69/udp    # TFTP
sudo ufw allow 8080/tcp  # HTTP-Boot
sudo ufw allow 8081/tcp  # Admin (optional)

# Prüfen, ob Bootimus lauscht
sudo netstat -tulpn | grep -E '69|8080|8081'
```

### DHCP-Server-Konflikte

Wenn du mehrere DHCP-Server im Netz hast:

```bash
# Alle DHCP-Server finden
sudo nmap --script broadcast-dhcp-discover

# Konfliktäre DHCP-Server abschalten
# Oder bei Bedarf DHCP-Relay/Helper konfigurieren
```

## Nächste Schritte

-  [Image-Verwaltung](images.md) konfigurieren, um ISOs hinzuzufügen
-  [Admin-Konsole](admin.md) zur Verwaltung einrichten
-  [Client-Verwaltung](clients.md) für Zugriffskontrolle konfigurieren


## Pi-hole (dnsmasq)

Pi-hole nutzt `dnsmasq` als DHCP-Engine, was über Konfigurationsdateien eine granulare Architektur-Erkennung erlaubt.

### Per Web-Oberfläche

#### 1. DHCP aktivieren
* Zu **Settings** > **DHCP** navigieren.
* **DHCP server enabled** anhaken.
* **IP-Range**, **Gateway** und **Lease duration** definieren.
* *Hinweis: Detaillierte PXE-Optionen sind im Web-UI nicht verfügbar und müssen per CLI konfiguriert werden.*

---

### Per Command Line (CLI)



Um BIOS und UEFI gleichzeitig zu unterstützen, musst du eine eigene Konfigurationsdatei im Verzeichnis `dnsmasq.d` anlegen.

#### 1. Config-Datei anlegen
* Terminal auf deinem Pi-hole öffnen und die Datei anlegen:
    `sudo nano /etc/dnsmasq.d/07-pxe.conf`

#### 2. Logik definieren
* Folgenden Block in die Datei einfügen und `<BOOT_SERVER_IP>` durch deine tatsächliche Server-IP ersetzen:

```bash
# 1. Client-Architektur erkennen (Option 93)
dhcp-match=set:bios,option:client-arch,0
dhcp-match=set:efi-x64,option:client-arch,7
dhcp-match=set:efi-x64-alt,option:client-arch,9

# 2. TFTP-Server-IP setzen (Option 66)
dhcp-option=option:server-ip,<BOOT_SERVER_IP>

# 3. Dateinamen je nach Architektur zuweisen (Option 67)
dhcp-boot=tag:bios,undionly.kpxe,,<BOOT_SERVER_IP>
dhcp-boot=tag:efi-x64,ipxe.efi,,<BOOT_SERVER_IP>
dhcp-boot=tag:efi-x64-alt,ipxe.efi,,<BOOT_SERVER_IP>

# 4. Optional: PXE-Menu/Service-Definitionen
pxe-service=tag:bios,x86PC,"Network Boot BIOS",undionly.kpxe,<BOOT_SERVER_IP>
pxe-service=tag:efi-x64,x86-64_EFI,"Network Boot UEFI",ipxe.efi,<BOOT_SERVER_IP>
pxe-service=tag:efi-x64-alt,x86-64_EFI,"Network Boot UEFI (Alt)",ipxe.efi,<BOOT_SERVER_IP>

```
