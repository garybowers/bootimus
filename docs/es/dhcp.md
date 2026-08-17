#  Guía de configuración DHCP

Guía completa para configurar varios servidores DHCP para que funcionen con Bootimus en arranque por red PXE.

##  Tabla de contenidos

- [proxyDHCP integrado (modo standalone)](#proxydhcp-integrado-modo-standalone)
- [Visión general](#visión-general)
- [ISC DHCP Server](#isc-dhcp-server)
- [Dnsmasq](#dnsmasq)
- [MikroTik RouterOS](#mikrotik-routeros)
- [Ubiquiti EdgeRouter](#ubiquiti-edgerouter)
- [pfSense](#pfsense)
- [OPNsense](#opnsense)
- [DHCP de Windows Server](#dhcp-de-windows-server)
- [Solución de problemas](#solución-de-problemas)
- [PiHole](#pi-hole-dnsmasq)

## proxyDHCP integrado (modo standalone)

Bootimus incluye un responder proxyDHCP integrado (RFC 4578). Cuando está habilitado, Bootimus responde a las opciones DHCP específicas de PXE por su cuenta — **tu servidor DHCP existente no necesita ninguna configuración PXE**. Sigue asignando IPs como siempre; Bootimus solo responde a clientes PXE con `next-server`, bootfile y la vendor class PXE, y nunca ofrece una dirección IP propia.

Esta es la forma más simple de correr Bootimus en cualquier entorno donde no controles el servidor DHCP principal, o donde no quieras tocarlo.

### Cómo funciona

1. El cliente hace broadcast de `DHCPDISCOVER`.
2. El servidor DHCP existente de la LAN responde con un lease de IP (no necesita info PXE).
3. Bootimus responde en el mismo broadcast solo con info de arranque PXE — sin IP.
4. La PXE ROM del cliente fusiona ambas respuestas: IP del DHCP principal, bootfile de Bootimus.

Como Bootimus nunca ofrece IP, no hay conflicto con el servidor DHCP existente ni pool de leases que coordinar.

### Habilitar

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

Apagado por defecto para que instalaciones existentes que ya corren dnsmasq/ISC-DHCP/etc. no se lleven una sorpresa.

### Requisitos y salvedades

- **Bindea UDP/67 y UDP/4011.** UDP/67 requiere `CAP_NET_BIND_SERVICE` o root; UDP/4011 es el puerto de discovery de boot-server PXE que algunas ROMs UEFI (notablemente AMI/Supermicro) consultan tras el offer inicial. En Docker, la imagen por defecto ya corre como root así que no se necesita capability extra; añade `--cap-add NET_BIND_SERVICE` solo para setups rootless.
- **Mismo dominio de broadcast.** proxyDHCP depende de ver los broadcasts DHCP del cliente. Funciona en una LAN plana o una sola VLAN. Si tu red usa un DHCP relay (`ip helper-address`) para reenviar DHCP entre VLANs, el relay reenvía a tu DHCP principal pero no a Bootimus — añade Bootimus como destino de relay adicional, o mantén los targets en la misma VLAN que Bootimus.
- **Networking en Docker.** Usa `macvlan`, `ipvlan` o `network_mode: host` para que el contenedor sea participante de primera clase en el dominio de broadcast. Una red bridge por defecto no funcionará — los broadcasts quedan atrapados dentro de `docker0`.
- **Dos servidores proxyDHCP en la misma LAN es legal, pero debuguearlo es una pesadilla.** Si habilitas el proxyDHCP integrado de Bootimus, deshabilita cualquier proxyDHCP de dnsmasq existente que anuncie PXE en la misma red.

### Ejemplo docker-compose

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

### Verificar

Los logs del contenedor deberían mostrar:

```
proxyDHCP: listening on UDP/67 + UDP/4011, advertising next-server=10.76.42.41 (BIOS=undionly.kpxe, UEFI=bootimus.efi, ARM64=bootimus-arm64.efi)
```

Y líneas por cliente cuando ocurre un arranque PXE:

```
proxyDHCP: DISCOVER -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
proxyDHCP: REQUEST  -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
TFTP: Client requesting file: bootimus.efi
```

El panel **Server Information** de la UI admin también muestra el estado actual de proxyDHCP.

---

## Visión general

Para habilitar el arranque por red PXE, tu servidor DHCP debe configurarse para:

1. **Proporcionar direcciones IP** a los clientes (DHCP estándar)
2. **Apuntar al servidor de arranque** (`next-server` o DHCP option 66)
3. **Especificar el filename del bootloader** (DHCP option 67)
4. **Detectar iPXE** y encadenar al menú HTTP (opcional pero recomendado)

**Reemplaza `192.168.1.10` con la dirección IP de tu servidor Bootimus en todos los ejemplos de abajo.**

### Nombres de archivo del bootloader

| Tipo de cliente | DHCP Filename | Notas |
|-------------|---------------|-------|
| UEFI (x86_64) | `bootimus.efi` (o `ipxe.efi`) | iPXE custom con script embebido |
| UEFI (ARM64) | `bootimus-arm64.efi` (o `ipxe-arm64.efi`) | iPXE custom con script embebido |
| BIOS legacy | `undionly.kpxe` | Bootloader PXE estándar |

> **Secure Boot:** para clientes con Secure Boot habilitado, activa el set de bootloaders integrado `secureboot-official` y anuncia `ipxe-shimx64.efi` (x86_64) o `ipxe-shimaa64.efi` (ARM64) como bootfile UEFI en su lugar. El proxyDHCP integrado lo hace automáticamente cuando el set está activo. Consulta la [guía de Secure Boot](secure-boot.md).

### Flujo de arranque

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

ISC DHCP es el servidor DHCP estándar en la mayoría de distribuciones Linux.

### Configuración

Edita `/etc/dhcp/dhcpd.conf`:

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

### Avanzado: configuración de arranque por cliente

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

### Reiniciar servicio

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

Dnsmasq es un servidor ligero de DHCP y DNS, popular en sistemas embebidos y routers.

### Configuración

Edita `/etc/dnsmasq.conf`:

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

### Configuración mínima

Si solo quieres PXE básico sin detección de iPXE:

```conf
dhcp-range=192.168.1.100,192.168.1.200,12h
dhcp-option=3,192.168.1.1
dhcp-option=6,8.8.8.8

# TFTP server and boot file
dhcp-boot=undionly.kpxe,192.168.1.10
```

### Reiniciar servicio

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

Los routers MikroTik son populares para arranque en red por su flexibilidad y rendimiento.

### Vía interfaz web (WebFig)

#### 1. Definir DHCP Options
* Navega a **IP** > **DHCP Server** > **Options**.
* Haz click en **Add New** para cada uno de los siguientes (asegúrate de que el **Value** incluye **comillas simples**):
    * **Option 66 (Server)**: Name: `tftp-server` | Code: `66` | Value: `'<BOOT_SERVER_IP>'`.
    * **Option 67 (BIOS)**: Name: `boot-bios` | Code: `67` | Value: `'undionly.kpxe'`.
    * **Option 67 (UEFI)**: Name: `boot-uefi` | Code: `67` | Value: `'ipxe.efi'`.

#### 2. Crear Option Sets
* Navega a **IP** > **DHCP Server** > **Option Sets**.
* **BIOS Set**: Click **Add New**, Name: `set-bios`, luego añade `tftp-server` y `boot-bios`.
* **UEFI Set**: Click **Add New**, Name: `set-uefi`, luego añade `tftp-server` y `boot-uefi`.

#### 3. Configurar Option Matcher (lógica de detección)
* Navega a **IP** > **DHCP Server** > **Option Matcher**.
* **Entrada BIOS**: Name: `match-bios` | Code: `93` | Value: `0x0000` | Option Set: `set-bios` | Server: `<DHCP_SERVER_NAME>`.
* **Entrada UEFI**: Name: `match-uefi-7` | Code: `93` | Value: `0x0007` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.
* **Entrada UEFI Alt**: Name: `match-uefi-9` | Code: `93` | Value: `0x0009` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.

#### 4. Configuración de red DHCP
* Navega a **IP** > **DHCP Server** > **Networks**.
* Abre la entrada para tu subnet (p. ej., `192.168.88.0/24`).
* **Next Server**: Introduce tu `<BOOT_SERVER_IP>`.
* **Boot File Name**: **DÉJALO VACÍO** (el Option Matcher inyecta dinámicamente el filename).

---

### Vía línea de comandos (CLI)



Reemplaza los placeholders `<BOOT_SERVER_IP>`, `<DHCP_SERVER_NAME>` y `<YOUR_SUBNET>` con tus detalles específicos antes de ejecutar.

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

Los EdgeRouters de Ubiquiti usan EdgeOS (basado en Vyatta/VyOS).

### Vía Web UI

1. Navega a **Services > DHCP Server**
2. Selecciona tu servidor DHCP (p. ej., `LAN`)
3. Bajo **Actions**, haz click en **Edit**
4. Desplázate a **PXE Settings**:
   - **Boot File**: `undionly.kpxe` (BIOS) o `ipxe.efi` (UEFI)
   - **Boot Server**: `192.168.1.10`
5. Haz click en **Save**

### Vía CLI

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

**Nota**: Reemplaza `LAN` con tu nombre real de shared network si es diferente.

### Verificar configuración

```bash
show service dhcp-server
show service dhcp-server leases
```

## pfSense

pfSense es una distribución de firewall y router open source popular.

### Pasos de configuración

1. Navega a **Services > DHCP Server**
2. Selecciona la interfaz (p. ej., **LAN**)
3. Desplázate hacia abajo a la sección **Network Booting**
4. Configura:
   - **Enable Network Booting**:  Marcar
   - **Next Server**: `192.168.1.10`
   - **Default BIOS Filename**: `undionly.kpxe`
   - **UEFI 64-bit Filename**: `ipxe.efi`
5. Haz click en **Save**

### Avanzado: opciones custom

Para detección de iPXE, añade opciones DHCP custom:

1. Navega a **Services > DHCP Server**
2. Selecciona interfaz
3. Desplázate a **Additional BOOTP/DHCP Options**
4. Añade opciones:

```
# Option 60 (Class Identifier)
60 text "PXEClient"

# Option 66 (TFTP Server)
66 text "192.168.1.10"

# Option 67 (Bootfile Name)
67 text "undionly.kpxe"
```

### Mapeos DHCP estáticos

Para clientes específicos:

1. **Services > DHCP Server > LAN**
2. Desplázate a **DHCP Static Mappings**
3. Haz click en **Add**
4. Configura:
   - **MAC Address**: `00:11:22:33:44:55`
   - **IP Address**: `192.168.1.50`
   - **Filename**: `ipxe.efi`
   - **Root Path**: Déjalo vacío
5. Haz click en **Save**

## OPNsense

OPNsense es un fork de pfSense con una interfaz moderna.

### Pasos de configuración

1. Navega a **Services > DHCPv4 > [Interface]**
2. Desplázate a **Network Booting**
3. Configura:
   - **Enable Network Booting**:  Marcar
   - **Next Server**: `192.168.1.10`
   - **Default BIOS Filename**: `undionly.kpxe`
   - **UEFI 64-bit Filename**: `ipxe.efi`
4. Haz click en **Save**
5. Haz click en **Apply Changes**

> **ARM64 / Raspberry Pi:** la GUI de OPNsense no tiene campo de filename ARM64 y no puede expresar la vendor option del firmware de la Raspberry Pi. Para Raspberry Pi o flotas de arquitectura mixta, habilita el proxyDHCP integrado de Bootimus junto a OPNsense — él mismo responde a esos clientes. Consulta la [guía de Raspberry Pi](raspberry-pi.md).

### Configuración avanzada

1. Navega a **Services > DHCPv4 > [Interface]**
2. Haz click en la pestaña **Additional Options**
3. Añade opciones custom similar a pfSense

## DHCP de Windows Server

Servicio DHCP de Windows Server.

### Pasos de configuración

1. Abre **DHCP Manager** (`dhcpmgmt.msc`)
2. Expande tu servidor DHCP
3. Expande **IPv4**
4. Click derecho en **Scope** → **Scope Options**
5. Configura:
   - **066 Boot Server Host Name**: `192.168.1.10`
   - **067 Bootfile Name**: `undionly.kpxe`

### Avanzado: detección UEFI y BIOS

1. Click derecho en **Scope** → **Set Predefined Options**
2. Haz click en **Add**
3. Crea option code 60 (Vendor Class):
   - **Code**: 60
   - **Name**: Vendor Class
   - **Data Type**: String
4. Crea policies para UEFI/BIOS:
   - Click derecho en **Policies** → **New Policy**
   - **Condition**: Vendor Class equals "PXEClient:Arch:00007" (UEFI)
   - **Options**: Establece bootfile a `ipxe.efi`
   - Repite para BIOS (Arch:00000) con `undionly.kpxe`

### Configuración con PowerShell

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

## Solución de problemas

### El cliente no recibe DHCP Offer

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

### El cliente no descarga el bootloader

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

### iPXE carga pero no hay menú

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

### Bootloader equivocado (UEFI vs BIOS)

```bash
# Check client firmware mode in DHCP logs
sudo journalctl -u isc-dhcp-server | grep -i "arch"

# UEFI clients send option 93 with value 00:07 or 00:09
# BIOS clients send option 93 with value 00:00

# Verify DHCP configuration handles architecture detection
```

### El cliente arranca pero muestra "No Bootable Device"

Causas posibles:
1. **DHCP option 67 incorrecta**: Debería ser `undionly.kpxe` o `ipxe.efi`
2. **Next-server no establecido**: La DHCP option 66 debe apuntar a Bootimus
3. **Puerto TFTP bloqueado**: Firewall bloqueando el puerto 69
4. **Falló el chainloading de iPXE**: Puerto HTTP 8080 no accesible

**Solución**:
```bash
# Verify all ports are accessible
sudo ufw allow 69/udp    # TFTP
sudo ufw allow 8080/tcp  # HTTP boot
sudo ufw allow 8081/tcp  # Admin (optional)

# Check Bootimus is listening
sudo netstat -tulpn | grep -E '69|8080|8081'
```

### Conflictos de servidor DHCP

Si tienes múltiples servidores DHCP en la red:

```bash
# Find all DHCP servers
sudo nmap --script broadcast-dhcp-discover

# Disable conflicting DHCP servers
# Or configure DHCP relay/helper if needed
```

## Siguientes pasos

-  Configura la [Gestión de imágenes](images.md) para añadir ISOs
-  Configura la [Consola admin](admin.md) para gestión
-  Configura la [Gestión de clientes](clients.md) para control de acceso


## Pi-hole (dnsmasq)

Pi-hole usa `dnsmasq` como motor DHCP, lo que permite detección granular de arquitectura usando archivos de configuración.

### Vía interfaz web

#### 1. Habilitar DHCP
* Navega a **Settings** > **DHCP**.
* Marca **DHCP server enabled**.
* Define tu **IP range**, **Gateway** y **Lease duration**.
* *Nota: Las opciones PXE detalladas no están disponibles en la Web UI y deben configurarse vía CLI.*

---

### Vía línea de comandos (CLI)



Para soportar BIOS y UEFI simultáneamente, debes crear un archivo de configuración custom en el directorio `dnsmasq.d`.

#### 1. Crear el archivo de config
* Abre un terminal en tu Pi-hole y crea el archivo:
    `sudo nano /etc/dnsmasq.d/07-pxe.conf`

#### 2. Definir la lógica
* Pega el siguiente bloque en el archivo, reemplazando `<BOOT_SERVER_IP>` con tu IP de servidor real:

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
