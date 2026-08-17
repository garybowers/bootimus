#  Guide de configuration DHCP

Guide complet pour configurer divers serveurs DHCP avec Bootimus pour le boot réseau PXE.

##  Table des matières

- [proxyDHCP intégré (mode standalone)](#proxydhcp-intégré-mode-standalone)
- [Vue d'ensemble](#vue-densemble)
- [ISC DHCP Server](#isc-dhcp-server)
- [Dnsmasq](#dnsmasq)
- [MikroTik RouterOS](#mikrotik-routeros)
- [Ubiquiti EdgeRouter](#ubiquiti-edgerouter)
- [pfSense](#pfsense)
- [OPNsense](#opnsense)
- [Windows Server DHCP](#windows-server-dhcp)
- [Dépannage](#dépannage)
- [PiHole](#pi-hole-dnsmasq)

## proxyDHCP intégré (mode standalone)

Bootimus inclut un répondeur proxyDHCP intégré (RFC 4578). Quand il est activé, Bootimus répond lui-même aux options DHCP spécifiques au PXE — **ton serveur DHCP existant n'a besoin d'aucune configuration PXE**. Il continue à distribuer les IPs comme d'habitude ; Bootimus répond uniquement aux clients PXE avec `next-server`, le bootfile et la vendor class PXE, et n'offre jamais d'adresse IP.

C'est la façon la plus simple de faire tourner Bootimus dans n'importe quel environnement où tu ne possèdes pas le serveur DHCP principal, ou où tu ne veux pas y toucher.

### Fonctionnement

1. Le client diffuse un `DHCPDISCOVER`.
2. Le serveur DHCP existant du LAN répond avec un lease IP (aucune info PXE nécessaire).
3. Bootimus répond sur le même broadcast avec uniquement les infos de boot PXE — pas d'IP.
4. Le PXE ROM du client merge les deux réponses : IP depuis le DHCP principal, bootfile depuis Bootimus.

Comme Bootimus n'offre jamais d'IP, il n'y a aucun conflit avec le serveur DHCP existant et aucun pool de leases à coordonner.

### Activer

```bash
# CLI flag
bootimus serve --proxy-dhcp

# Environment variable
BOOTIMUS_PROXY_DHCP_ENABLED=true

# YAML config
proxy_dhcp:
  enabled: true
  bootfile_bios: undionly.kpxe           # legacy BIOS PXE
  bootfile_uefi: bootimus.efi            # UEFI x64
  bootfile_arm64: bootimus-arm64.efi     # UEFI ARM64
```

Désactivé par défaut pour que les installs existantes qui font déjà tourner dnsmasq/ISC-DHCP/etc. ne soient pas surprises.

### Prérequis et mises en garde

- **Bind sur UDP/67 et UDP/4011.** UDP/67 nécessite `CAP_NET_BIND_SERVICE` ou root ; UDP/4011 est le port de découverte du PXE boot-server que certains ROMs UEFI (notamment AMI/Supermicro) recontactent après l'offer initial. Dans Docker, l'image par défaut tourne déjà en root donc pas besoin de capability supplémentaire ; ajoute `--cap-add NET_BIND_SERVICE` uniquement pour les setups rootless.
- **Même broadcast domain.** Le proxyDHCP s'appuie sur le fait de voir les broadcasts DHCP des clients. Ça marche sur un LAN à plat ou un seul VLAN. Si ton réseau utilise un relais DHCP (`ip helper-address`) pour faire passer le DHCP entre VLANs, le relais forwarde vers ton DHCP principal mais pas vers Bootimus — ajoute Bootimus comme cible de relais supplémentaire, ou garde les cibles sur le même VLAN que Bootimus.
- **Réseau Docker.** Utilise `macvlan`, `ipvlan`, ou `network_mode: host` pour que le conteneur soit un participant de première classe sur le broadcast domain. Un réseau bridge par défaut ne marchera pas — les broadcasts restent coincés dans `docker0`.
- **Deux serveurs proxyDHCP sur le même LAN, c'est légal, mais le debug est un cauchemar.** Si tu actives le proxyDHCP intégré de Bootimus, désactive tout proxyDHCP dnsmasq existant qui annonce du PXE sur le même réseau.

### Exemple docker-compose

```yaml
services:
  bootimus:
    image: garybowers/bootimus:latest
    environment:
      BOOTIMUS_PROXY_DHCP_ENABLED: "true"
      BOOTIMUS_SERVER_ADDR: 10.76.42.41    # the container's macvlan IP
    ports:
      - "67:67/udp"                         # proxyDHCP
      - "4011:4011/udp"                     # PXE boot-server discovery
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

### Vérifier

Les logs du conteneur devraient afficher :

```
proxyDHCP: listening on UDP/67 + UDP/4011, advertising next-server=10.76.42.41 (BIOS=undionly.kpxe, UEFI=bootimus.efi, ARM64=bootimus-arm64.efi)
```

Et des lignes par client quand un boot PXE se produit :

```
proxyDHCP: DISCOVER -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
proxyDHCP: REQUEST  -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
TFTP: Client requesting file: bootimus.efi
```

Le panneau **Server Information** de l'UI admin affiche également l'état actuel du proxyDHCP.

---

## Vue d'ensemble

Pour activer le boot réseau PXE, ton serveur DHCP doit être configuré pour :

1. **Fournir des adresses IP** aux clients (DHCP standard)
2. **Pointer vers le serveur de boot** (`next-server` ou option DHCP 66)
3. **Spécifier le nom de fichier du bootloader** (option DHCP 67)
4. **Détecter iPXE** et chaîner vers le menu HTTP (optionnel mais recommandé)

**Remplace `192.168.1.10` par l'IP de ton serveur Bootimus dans tous les exemples ci-dessous.**

### Noms de fichiers bootloader

| Type de client | Nom de fichier DHCP | Notes |
|-------------|---------------|-------|
| UEFI (x86_64) | `bootimus.efi` (ou `ipxe.efi`) | iPXE custom-buildé avec script embarqué |
| UEFI (ARM64) | `bootimus-arm64.efi` (ou `ipxe-arm64.efi`) | iPXE custom-buildé avec script embarqué |
| BIOS legacy | `undionly.kpxe` | Bootloader PXE standard |

> **Secure Boot :** pour les clients avec Secure Boot activé, active le set de bootloaders intégré `secureboot-official` et annonce `ipxe-shimx64.efi` (x86_64) ou `ipxe-shimaa64.efi` (ARM64) comme bootfile UEFI à la place. Le proxyDHCP intégré le fait automatiquement quand le set est actif. Voir le [guide Secure Boot](secure-boot.md).

### Flux de boot

```
Client → DHCP Request
      ← DHCP Offer (IP, next-server, bootloader filename)
Client → TFTP Request for bootloader (bootimus.efi or undionly.kpxe)
      ← Bootloader downloaded
Client → HTTP Request for menu.ipxe
      ← Boot menu displayed
Client → Boot selected ISO
```

## ISC DHCP Server

ISC DHCP est le serveur DHCP standard sur la plupart des distributions Linux.

### Configuration

Édite `/etc/dhcp/dhcpd.conf` :

```conf
# Basic DHCP configuration
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    option routers 192.168.1.1;
    option domain-name-servers 8.8.8.8, 8.8.4.4;

    # PXE boot server
    next-server 192.168.1.10;  # Bootimus server IP

    # Detect if client is already running iPXE
    if exists user-class and option user-class = "iPXE" {
        # Client has iPXE, chain to HTTP menu
        filename "http://192.168.1.10:8080/menu.ipxe";
    }
    # UEFI systems (x86_64)
    elsif option arch = 00:07 {
        filename "ipxe.efi";
    }
    # UEFI systems (alternative)
    elsif option arch = 00:09 {
        filename "ipxe.efi";
    }
    # Legacy BIOS
    else {
        filename "undionly.kpxe";
    }
}
```

### Avancé : configuration de boot par client

```conf
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    next-server 192.168.1.10;

    # Specific client configuration
    host lab-server-1 {
        hardware ethernet 00:11:22:33:44:55;
        fixed-address 192.168.1.50;
        filename "ipxe.efi";
    }

    # Default configuration for other clients
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

### Redémarrer le service

```bash
# Test configuration
sudo dhcpd -t -cf /etc/dhcp/dhcpd.conf

# Restart DHCP service
sudo systemctl restart isc-dhcp-server

# Check status
sudo systemctl status isc-dhcp-server

# View logs
sudo journalctl -u isc-dhcp-server -f
```

## Dnsmasq

Dnsmasq est un serveur DHCP et DNS léger, populaire sur les systèmes embarqués et les routeurs.

### Configuration

Édite `/etc/dnsmasq.conf` :

```conf
# DHCP range
dhcp-range=192.168.1.100,192.168.1.200,12h

# Default gateway
dhcp-option=3,192.168.1.1

# DNS servers
dhcp-option=6,8.8.8.8,8.8.4.4

# PXE boot configuration
dhcp-boot=tag:!ipxe,undionly.kpxe,192.168.1.10
dhcp-boot=tag:ipxe,http://192.168.1.10:8080/menu.ipxe

# UEFI support
dhcp-match=set:efi-x86_64,option:client-arch,7
dhcp-match=set:efi-x86_64,option:client-arch,9
dhcp-boot=tag:efi-x86_64,tag:!ipxe,ipxe.efi,192.168.1.10

# Legacy BIOS support
dhcp-match=set:bios,option:client-arch,0
dhcp-boot=tag:bios,tag:!ipxe,undionly.kpxe,192.168.1.10

# Enable TFTP server (optional, if using dnsmasq as TFTP server)
# enable-tftp
# tftp-root=/var/lib/tftpboot
```

### Configuration minimale

Si tu veux juste un PXE basique sans détection d'iPXE :

```conf
dhcp-range=192.168.1.100,192.168.1.200,12h
dhcp-option=3,192.168.1.1
dhcp-option=6,8.8.8.8

# TFTP server and boot file
dhcp-boot=undionly.kpxe,192.168.1.10
```

### Redémarrer le service

```bash
# Test configuration
sudo dnsmasq --test

# Restart service
sudo systemctl restart dnsmasq

# Check status
sudo systemctl status dnsmasq

# View logs
sudo journalctl -u dnsmasq -f
```

## MikroTik RouterOS

Les routeurs MikroTik sont populaires pour le boot réseau grâce à leur flexibilité et leurs performances.

### Via l'interface web (WebFig)

#### 1. Définir les options DHCP
* Va dans **IP** > **DHCP Server** > **Options**.
* Clique **Add New** pour chacun des éléments suivants (assure-toi que la **Value** inclut des **single quotes**) :
    * **Option 66 (Server)** : Name: `tftp-server` | Code: `66` | Value: `'<BOOT_SERVER_IP>'`.
    * **Option 67 (BIOS)** : Name: `boot-bios` | Code: `67` | Value: `'undionly.kpxe'`.
    * **Option 67 (UEFI)** : Name: `boot-uefi` | Code: `67` | Value: `'ipxe.efi'`.

#### 2. Créer les Option Sets
* Va dans **IP** > **DHCP Server** > **Option Sets**.
* **BIOS Set** : Clique **Add New**, Name: `set-bios`, puis ajoute `tftp-server` et `boot-bios`.
* **UEFI Set** : Clique **Add New**, Name: `set-uefi`, puis ajoute `tftp-server` et `boot-uefi`.

#### 3. Configurer l'Option Matcher (logique de détection)
* Va dans **IP** > **DHCP Server** > **Option Matcher**.
* **BIOS Entry** : Name: `match-bios` | Code: `93` | Value: `0x0000` | Option Set: `set-bios` | Server: `<DHCP_SERVER_NAME>`.
* **UEFI Entry** : Name: `match-uefi-7` | Code: `93` | Value: `0x0007` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.
* **UEFI Alt Entry** : Name: `match-uefi-9` | Code: `93` | Value: `0x0009` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.

#### 4. Configuration réseau DHCP
* Va dans **IP** > **DHCP Server** > **Networks**.
* Ouvre l'entrée de ton sous-réseau (par ex. `192.168.88.0/24`).
* **Next Server** : entre ton `<BOOT_SERVER_IP>`.
* **Boot File Name** : **LAISSE VIDE** (l'Option Matcher injecte dynamiquement le nom de fichier).

---

### Via la ligne de commande (CLI)



Remplace les placeholders `<BOOT_SERVER_IP>`, `<DHCP_SERVER_NAME>` et `<YOUR_SUBNET>` par tes détails spécifiques avant d'exécuter.

```routeros
# 1. Define DHCP Options
/ip dhcp-server option
add code=66 name=tftp-server value="'<BOOT_SERVER_IP>'"
add code=67 name=boot-bios value="'undionly.kpxe'"
add code=67 name=boot-uefi value="'ipxe.efi'"

# 2. Create Option Sets
/ip dhcp-server option sets
add name=set-bios options=tftp-server,boot-bios
add name=set-uefi options=tftp-server,boot-uefi

# 3. Create Option Matchers for Architecture Detection
/ip dhcp-server option-matcher
add code=93 name=match-bios option-set=set-bios server=<DHCP_SERVER_NAME> value=0x0000
add code=93 name=match-uefi-7 option-set=set-uefi server=<DHCP_SERVER_NAME> value=0x0007
add code=93 name=match-uefi-9 option-set=set-uefi server=<DHCP_SERVER_NAME> value=0x0009

# 4. Apply to DHCP Network
/ip dhcp-server network
set [find address="<YOUR_SUBNET>"] boot-file-name="" next-server=<BOOT_SERVER_IP>
```

## Ubiquiti EdgeRouter

Les EdgeRouters Ubiquiti utilisent EdgeOS (basé sur Vyatta/VyOS).

### Via l'interface web

1. Va dans **Services > DHCP Server**
2. Sélectionne ton serveur DHCP (par ex. `LAN`)
3. Sous **Actions**, clique **Edit**
4. Descends jusqu'à **PXE Settings** :
   - **Boot File** : `undionly.kpxe` (BIOS) ou `ipxe.efi` (UEFI)
   - **Boot Server** : `192.168.1.10`
5. Clique **Save**

### Via CLI

```bash
configure

# Set TFTP server for network boot
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-server 192.168.1.10
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-name undionly.kpxe

# Advanced: UEFI support
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "option arch code 93 = unsigned integer 16;"
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "if option arch = 00:07 { filename &quot;ipxe.efi&quot;; } else { filename &quot;undionly.kpxe&quot;; }"

commit
save
exit
```

**Note** : remplace `LAN` par le nom réel de ton réseau partagé s'il diffère.

### Vérifier la configuration

```bash
show service dhcp-server
show service dhcp-server leases
```

## pfSense

pfSense est une distribution de pare-feu et routeur open-source populaire.

### Étapes de configuration

1. Va dans **Services > DHCP Server**
2. Sélectionne l'interface (par ex. **LAN**)
3. Descends jusqu'à la section **Network Booting**
4. Configure :
   - **Enable Network Booting** :  coche
   - **Next Server** : `192.168.1.10`
   - **Default BIOS Filename** : `undionly.kpxe`
   - **UEFI 64-bit Filename** : `ipxe.efi`
5. Clique **Save**

### Avancé : options personnalisées

Pour la détection iPXE, ajoute des options DHCP personnalisées :

1. Va dans **Services > DHCP Server**
2. Sélectionne l'interface
3. Descends jusqu'à **Additional BOOTP/DHCP Options**
4. Ajoute les options :

```
# Option 60 (Class Identifier)
60 text "PXEClient"

# Option 66 (TFTP Server)
66 text "192.168.1.10"

# Option 67 (Bootfile Name)
67 text "undionly.kpxe"
```

### Mappings DHCP statiques

Pour des clients spécifiques :

1. **Services > DHCP Server > LAN**
2. Descends jusqu'à **DHCP Static Mappings**
3. Clique **Add**
4. Configure :
   - **MAC Address** : `00:11:22:33:44:55`
   - **IP Address** : `192.168.1.50`
   - **Filename** : `ipxe.efi`
   - **Root Path** : laisse vide
5. Clique **Save**

## OPNsense

OPNsense est un fork de pfSense avec une interface moderne.

### Étapes de configuration

1. Va dans **Services > DHCPv4 > [Interface]**
2. Descends jusqu'à **Network Booting**
3. Configure :
   - **Enable Network Booting** :  coche
   - **Next Server** : `192.168.1.10`
   - **Default BIOS Filename** : `undionly.kpxe`
   - **UEFI 64-bit Filename** : `ipxe.efi`
4. Clique **Save**
5. Clique **Apply Changes**

> **ARM64 / Raspberry Pi :** la GUI d'OPNsense n'a pas de champ de filename ARM64 et ne peut pas exprimer l'option vendor du firmware Raspberry Pi. Pour des Raspberry Pi ou des flottes multi-architectures, active le proxyDHCP intégré de Bootimus en parallèle d'OPNsense — il répond lui-même à ces clients. Voir le [guide Raspberry Pi](raspberry-pi.md).

### Configuration avancée

1. Va dans **Services > DHCPv4 > [Interface]**
2. Clique sur l'onglet **Additional Options**
3. Ajoute des options personnalisées similaires à pfSense

## Windows Server DHCP

Service DHCP de Windows Server.

### Étapes de configuration

1. Ouvre **DHCP Manager** (`dhcpmgmt.msc`)
2. Déplie ton serveur DHCP
3. Déplie **IPv4**
4. Clic droit sur **Scope** → **Scope Options**
5. Configure :
   - **066 Boot Server Host Name** : `192.168.1.10`
   - **067 Bootfile Name** : `undionly.kpxe`

### Avancé : détection UEFI et BIOS

1. Clic droit sur **Scope** → **Set Predefined Options**
2. Clique **Add**
3. Crée l'option code 60 (Vendor Class) :
   - **Code** : 60
   - **Name** : Vendor Class
   - **Data Type** : String
4. Crée des politiques pour UEFI/BIOS :
   - Clic droit sur **Policies** → **New Policy**
   - **Condition** : Vendor Class equals "PXEClient:Arch:00007" (UEFI)
   - **Options** : définis bootfile sur `ipxe.efi`
   - Répète pour BIOS (Arch:00000) avec `undionly.kpxe`

### Configuration PowerShell

```powershell
# Set DHCP scope options
Set-DhcpServerv4OptionValue -ScopeId 192.168.1.0 -OptionId 66 -Value "192.168.1.10"
Set-DhcpServerv4OptionValue -ScopeId 192.168.1.0 -OptionId 67 -Value "undionly.kpxe"

# Create policy for UEFI
Add-DhcpServerv4Policy -Name "UEFI" -Condition OR -VendorClass EQ "PXEClient:Arch:00007"
Set-DhcpServerv4OptionValue -PolicyName "UEFI" -OptionId 67 -Value "ipxe.efi"

# Create policy for BIOS
Add-DhcpServerv4Policy -Name "BIOS" -Condition OR -VendorClass EQ "PXEClient:Arch:00000"
Set-DhcpServerv4OptionValue -PolicyName "BIOS" -OptionId 67 -Value "undionly.kpxe"
```

## Dépannage

### Le client ne reçoit pas d'offer DHCP

```bash
# Check DHCP server logs
sudo journalctl -u isc-dhcp-server -f   # ISC DHCP
sudo journalctl -u dnsmasq -f           # Dnsmasq

# Verify DHCP server is running
sudo systemctl status isc-dhcp-server
sudo systemctl status dnsmasq

# Check network connectivity
ping 192.168.1.10

# Capture DHCP traffic
sudo tcpdump -i eth0 port 67 or port 68
```

### Le client ne télécharge pas le bootloader

```bash
# Verify Bootimus TFTP server is running
sudo netstat -ulnp | grep :69

# Test TFTP manually
tftp 192.168.1.10
> get undionly.kpxe
> quit

# Check Bootimus logs
docker logs bootimus | grep TFTP
```

### iPXE charge mais pas de menu

```bash
# Verify HTTP server is running
curl http://192.168.1.10:8080/menu.ipxe

# Check if ISOs are available
curl -u admin:password http://192.168.1.10:8081/api/images

# Verify client has network access to HTTP port
telnet 192.168.1.10 8080

# Check Bootimus logs
docker logs bootimus -f
```

### Mauvais bootloader (UEFI vs BIOS)

```bash
# Check client firmware mode in DHCP logs
sudo journalctl -u isc-dhcp-server | grep -i "arch"

# UEFI clients send option 93 with value 00:07 or 00:09
# BIOS clients send option 93 with value 00:00

# Verify DHCP configuration handles architecture detection
```

### Le client boote mais affiche « No Bootable Device »

Causes possibles :
1. **Option DHCP 67 incorrecte** : doit être `undionly.kpxe` ou `ipxe.efi`
2. **Next-server non défini** : l'option DHCP 66 doit pointer vers Bootimus
3. **Port TFTP bloqué** : pare-feu bloquant le port 69
4. **Échec du chainloading iPXE** : port HTTP 8080 non accessible

**Solution** :
```bash
# Verify all ports are accessible
sudo ufw allow 69/udp    # TFTP
sudo ufw allow 8080/tcp  # HTTP boot
sudo ufw allow 8081/tcp  # Admin (optional)

# Check Bootimus is listening
sudo netstat -tulpn | grep -E '69|8080|8081'
```

### Conflits de serveurs DHCP

Si tu as plusieurs serveurs DHCP sur le réseau :

```bash
# Find all DHCP servers
sudo nmap --script broadcast-dhcp-discover

# Disable conflicting DHCP servers
# Or configure DHCP relay/helper if needed
```

## Étapes suivantes

-  Configure la [Gestion des images](images.md) pour ajouter des ISOs
-  Mets en place la [Console d'administration](admin.md) pour la gestion
-  Configure la [Gestion des clients](clients.md) pour le contrôle d'accès


## Pi-hole (dnsmasq)

Pi-hole utilise `dnsmasq` pour son moteur DHCP, ce qui permet une détection granulaire de l'architecture via des fichiers de configuration.

### Via l'interface web

#### 1. Activer le DHCP
* Va dans **Settings** > **DHCP**.
* Coche **DHCP server enabled**.
* Définis ta **plage IP**, ton **Gateway** et la **durée de lease**.
* *Note : les options PXE détaillées ne sont pas disponibles dans l'UI web et doivent être configurées via CLI.*

---

### Via la ligne de commande (CLI)



Pour supporter BIOS et UEFI simultanément, tu dois créer un fichier de configuration personnalisé dans le répertoire `dnsmasq.d`.

#### 1. Créer le fichier de config
* Ouvre un terminal sur ton Pi-hole et crée le fichier :
    `sudo nano /etc/dnsmasq.d/07-pxe.conf`

#### 2. Définir la logique
* Colle le bloc suivant dans le fichier, en remplaçant `<BOOT_SERVER_IP>` par l'IP réelle de ton serveur :

```bash
# 1. Identify client architecture (Option 93)
dhcp-match=set:bios,option:client-arch,0
dhcp-match=set:efi-x64,option:client-arch,7
dhcp-match=set:efi-x64-alt,option:client-arch,9

# 2. Set the TFTP Server IP (Option 66)
dhcp-option=option:server-ip,<BOOT_SERVER_IP>

# 3. Assign filenames based on architecture (Option 67)
dhcp-boot=tag:bios,undionly.kpxe,,<BOOT_SERVER_IP>
dhcp-boot=tag:efi-x64,ipxe.efi,,<BOOT_SERVER_IP>
dhcp-boot=tag:efi-x64-alt,ipxe.efi,,<BOOT_SERVER_IP>

# 4. Optional: PXE Menu/Service definitions
pxe-service=tag:bios,x86PC,"Network Boot BIOS",undionly.kpxe,<BOOT_SERVER_IP>
pxe-service=tag:efi-x64,x86-64_EFI,"Network Boot UEFI",ipxe.efi,<BOOT_SERVER_IP>
pxe-service=tag:efi-x64-alt,x86-64_EFI,"Network Boot UEFI (Alt)",ipxe.efi,<BOOT_SERVER_IP>

```
