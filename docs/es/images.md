#  Guía de gestión de imágenes

Guía completa para gestionar imágenes ISO, extraer archivos de arranque y manejar casos especiales como el netboot de Debian/Ubuntu.

##  Tabla de contenidos

- [Añadir imágenes](#añadir-imágenes)
- [Sumas de verificación y avisos de actualización](#sumas-de-verificación-y-avisos-de-actualización)
- [Imágenes de kernel unificadas (UKI)](#imágenes-de-kernel-unificadas-uki)
- [OpenBSD y NetBSD](#openbsd-y-netbsd)
- [Medios WinPE y de rescate](#medios-winpe-y-de-rescate)
- [Extracción de kernel](#extracción-de-kernel)
- [Soporte netboot](#soporte-netboot)
- [Optimización de Ubuntu Desktop](#optimización-de-ubuntu-desktop)
- [Distribuciones soportadas](#distribuciones-soportadas)
- [Solución de problemas](#solución-de-problemas)

## Añadir imágenes

### Subir vía interfaz web

1. Navega al panel admin: `http://your-server:8081`
2. Haz click en el botón **"Upload ISO"**
3. Arrastra y suelta el archivo ISO o haz click para buscar
4. Opcionalmente añade descripción
5. Marca **"Public"** para hacerla disponible a todos los clientes
6. Haz click en **"Upload"**

**Límites de subida**: 10 GB por archivo

### Subir vía API

```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Descargar desde URL

Descarga ISOs directamente al servidor sin subida local:

**Vía interfaz web**:
1. Haz click en el botón **"Download from URL"**
2. Introduce la URL de descarga del ISO
3. Añade descripción
4. Haz click en **"Download"**

**Vía API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'
```

**Monitorizar progreso**:
```bash
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Sumas de verificación y avisos de actualización

**Verificación al descargar.** Cuando una ISO se obtiene a través de **Get Images** (el catálogo integrado), Bootimus busca un archivo de sumas de verificación publicado junto a ella (`SHA256SUMS`, `sha256sum.txt` y similares) y calcula el hash de la descarga mientras la transmite. Si no coincide, la descarga falla y el archivo se elimina, de modo que una imagen truncada o manipulada nunca llega al menú de arranque. Las descargas desde una URL cualquiera también se pueden verificar pasando un `sha256` o una `checksum_url` a `/api/images/download`:

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{"url": "https://releases.ubuntu.com/24.04.4/ubuntu-24.04.4-live-server-amd64.iso",
       "checksum_url": "https://releases.ubuntu.com/24.04.4/SHA256SUMS"}'
```

Las imágenes verificadas muestran una insignia **✓ Verified** en la lista de imágenes y el SHA-256 registrado en el panel de propiedades de la imagen. Las imágenes descargadas sin suma de verificación publicada, o subidas a mano, se marcan como **Unverified**; pega un SHA-256 o la URL de un archivo de sumas en el panel de propiedades y pulsa **Verify** para calcular el hash del archivo en segundo plano (`POST /api/images/verify?filename=…`, consulta la misma URL con `GET`).

**Versiones más nuevas.** Una vez al día (y en **Settings → Housekeeping → Check now**, o con `POST /api/catalogue/check`) Bootimus compara las imágenes instaladas con el catálogo. Una imagen descargada del catálogo se empareja por su distribución y línea de versión, así que `ubuntu-24.04.3-desktop-amd64.iso` se marca cuando aparece `24.04.4`; las imágenes añadidas antes de esta función se emparejan por la forma del nombre de archivo. Las imágenes marcadas llevan una insignia **Update available** y un botón **Download newer version** en el panel de propiedades, las herramientas muestran la misma insignia con **Download newer**, y el dashboard cuenta las actualizaciones pendientes. La comprobación usa el catálogo online cuando las actualizaciones remotas de perfiles están permitidas y recurre al catálogo incrustado en el binario cuando está activo `--disable-remote-profiles`. Desmarca **Check the catalogue daily** en Housekeeping para desactivarla.

### Organizar con carpetas

Los ISOs colocados en subdirectorios se agrupan automáticamente en el menú de arranque:

```
/data/isos/
├── ubuntu-24.04.iso              # ungrouped
├── linux/                        # "linux" group
│   ├── debian-12.iso
│   └── servers/                  # "servers" subgroup under "linux"
│       └── truenas-scale.iso
└── windows/                      # "windows" group
    └── win11.iso
```

Los grupos se auto-crean al arrancar y al escanear. También pueden gestionarse manualmente desde la pestaña Groups en la UI admin.

### Escanear ISOs existentes

Si copias ISOs manualmente al directorio de datos (incluyendo subdirectorios):

1. Copia los archivos ISO al directorio `/data/isos/` (o subdirectorios)
2. Haz click en el botón **"Scan for ISOs"** en el panel admin
3. Bootimus detecta y registra los nuevos ISOs y crea grupos desde las carpetas

**Vía API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Imágenes de kernel unificadas (UKI)

Una imagen de kernel unificada (UKI) es un único ejecutable EFI que agrupa un kernel Linux, su initrd y su línea de comandos del kernel. Bootimus arranca las UKI directamente, sin extracción.

**Añadir una UKI**: sube el archivo `.efi`, descárgalo desde una URL o cópialo en `/data/isos/` y escanea, exactamente igual que con una ISO. Bootimus solo registra los archivos `.efi` que son UKI (ejecutables EFI con una sección `.linux`); otros programas EFI se rechazan y los archivos dentro de la carpeta extraída de una ISO se ignoran.

**Parámetros de arranque**: la línea de comandos incrustada en la sección `.cmdline` de la UKI se copia en los parámetros de arranque de la imagen cuando se añade por primera vez, para que puedas verla y cambiarla en las propiedades de la imagen. Bootimus arranca la imagen con:

```
kernel http://<server>:8080/isos/<image>.efi <boot parameters>
```

Con Secure Boot desactivado, estos parámetros sustituyen a la línea de comandos incrustada, por eso esta aparece rellenada. Con Secure Boot activado, el systemd-stub de la mayoría de las UKI los ignora y usa siempre la línea de comandos incrustada.

**Solo UEFI**: las UKI son programas EFI, así que los clientes con BIOS legacy no pueden arrancarlas. Si eliges una en un cliente BIOS, aparece un mensaje que indica que necesita UEFI.

## OpenBSD y NetBSD

OpenBSD y NetBSD no arrancan con un kernel e initrd al estilo Linux. Cada uno trae su propio cargador de red de segunda etapa, y ese cargador solo habla TFTP: obtiene su archivo de configuración y el kernel de instalación del directorio raíz del servidor TFTP. Bootimus soporta ambos extrayendo esos archivos de la ISO de instalación y sirviéndolos por su servidor TFTP, así que el puerto TFTP debe ser accesible desde los clientes (ya lo es para el propio PXE).

**Añadir una imagen**: sube `install77.iso` (o `cd77.iso`) para OpenBSD, o `NetBSD-10.1-amd64.iso` para NetBSD, y ejecuta **Extract Kernel/Initrd** como con cualquier otra ISO. La extracción copia los cargadores y el kernel de instalación en una carpeta `bsd/` junto a la ISO y extrae el árbol completo de la ISO para que el instalador pueda obtener sus sets desde `http://<server>:8080/boot/<image>/iso/`.

| Distribución | Cargadores tomados de la ISO | Kernel |
|--------------|------------------------------|--------|
| OpenBSD | `pxeboot` (BIOS), `BOOTX64.EFI`, `BOOTIA32.EFI`, `BOOTAA64.EFI` (UEFI) desde `/<version>/<arch>/` | `bsd.rd` |
| NetBSD | `pxeboot_ia32.bin` (BIOS) desde `/<arch>/installation/misc/`, `bootx64.efi` (UEFI) cuando la ISO lo incluye | `netbsd-INSTALL.gz` |

**Cómo funciona el arranque**: la entrada del menú encadena a un pequeño script iPXE en el puerto HTTP, que registra que este cliente (por dirección IP) eligió esa imagen y luego encadena el cargador correspondiente por TFTP desde `bsd/<image>/`. Durante los 15 minutos siguientes, las peticiones desde esa IP de los archivos que los cargadores buscan en la raíz TFTP (`etc/boot.conf`, `etc/random.seed`, `bsd.rd`, `bsd` para OpenBSD; `boot.cfg`, `EFI/NetBSD/boot.cfg`, `netbsd` y `netbsd-INSTALL.gz` para NetBSD) se responden desde esa imagen. El archivo de configuración se genera al vuelo y apunta al cargador hacia el kernel bajo `bsd/<image>/`, así que las rutas explícitas también funcionan. Si la ISO no tiene el cargador para el firmware del cliente, el cliente ve un mensaje que lo indica y vuelve al menú.

**Consola serie y otros ajustes del cargador**: pon las líneas de configuración adicionales del cargador en los **Boot Parameters** de la imagen, separadas por punto y coma. Para OpenBSD se añaden al `boot.conf` generado, por ejemplo `stty com0 115200;set tty com0`. Para NetBSD se añaden al `boot.cfg` generado, por ejemplo `consdev=com0`.

**Limitaciones conocidas**:

- Este soporte es nuevo y todavía no se ha verificado en hardware real. Por favor, informa de qué funciona y qué no.
- El `pxeboot_ia32.bin` de NetBSD realiza su propio intercambio DHCP tras cargarse y lee el campo `filename` de DHCP para decidir entre NFS y TFTP. Sin un prefijo `tftp:` ahí, usa NFS por defecto, que Bootimus no proporciona para este fin. Si el cargador informa de que no puede abrir `boot.cfg` o el kernel, escribe `boot tftp:bsd/<image>/netbsd-INSTALL.gz` en su prompt, o da a los clientes NetBSD `filename "tftp:boot.cfg";` en tu servidor DHCP. Los cargadores de OpenBSD siempre usan TFTP y no necesitan este paso.
- La asociación por cliente se basa en la dirección IP, así que dos clientes detrás de la misma dirección NAT que arranquen imágenes BSD distintas en 15 minutos recibirían la misma configuración.
- La autoinstalación de los instaladores BSD todavía no está conectada. Los instaladores pueden obtener igualmente los sets del árbol de la ISO extraído por HTTP.

## Medios WinPE y de rescate

Bootimus arranca los instaladores de Windows mediante [wimboot](https://ipxe.org/wimboot), y esa misma vía cubre ahora los medios de rescate basados en WinPE, como el WinPE de Sergei Strelec, Active@ Boot Disk y Hiren's BootCD PE.

**Diseños soportados**: cualquier ISO que lleve un `BCD` (bajo `boot/` o `efi/microsoft/boot/`), `boot/boot.sdi` y al menos un archivo `.wim`. Los medios de instalación tienen además `sources/install.wim` o `install.esd`; los medios sin uno se clasifican con el perfil **WinPE / Rescue Media** (`winpe`) en lugar de **Windows**, de modo que no se les aplica ningún tratamiento de autoinstalación ni SMB. Se encuentran los archivos WIM en cualquier lugar de los cuatro primeros niveles de directorios, así que `SSTR/*.wim` de Strelec y diseños similares funcionan sin renombrar nada.

**Elegir el WIM**: tras la extracción, el panel de propiedades de la imagen lista todos los WIM encontrados con su tamaño. Elige el que debe cargar wimboot y, cuando un WIM contiene varias imágenes, el índice de imagen. El índice solo se añade a la línea de wimboot cuando es mayor que uno, y un `index=` explícito en los parámetros de arranque tiene prioridad. Lo mismo se puede establecer a través de la API:

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"wim_path":"SSTR/strelec10x64Eng.wim","wim_index":1}' \
  "http://localhost:8081/api/images/winpe?filename=WinPE_Strelec.iso"
```

**Cómo arranca**: solo se envía al cliente el WIM seleccionado; wimboot sintetiza el BCD y `boot.sdi` por sí mismo, lo que evita la pantalla en negro que producen algunos medios cuando se usa el BCD del DVD. Los parámetros de arranque se limitan a las opciones propias de wimboot (`rawbcd`, `rawwim`, `gui`, `linear`, `quiet`, `pause`, `index`, `initrdfile`), exactamente igual que para los instaladores de Windows.

**Limitaciones**:

- Los programas que el entorno de rescate espera encontrar en su propio medio de arranque (por ejemplo la carpeta `SSTR` de herramientas portables de Strelec) no están dentro del WIM, así que faltan tras un arranque PXE salvo que el propio entorno pueda obtenerlos por red. El escritorio WinPE básico y las herramientas integradas en el WIM funcionan.
- Los medios que necesitan sus propias entradas BCD (menús `bootmgr` personalizados, menús de selección multi-WIM) arrancan directamente en el WIM elegido en lugar de mostrar ese menú.
- Todavía no se pueden subir archivos `.wim` sueltos; ponlos en una ISO con un `BCD` y `boot.sdi`, o usa una ISO de rescate completa.

## Extracción de kernel

La mayoría de ISOs modernos soportan arranque HTTP directo vía el comando `sanboot` de iPXE, que descarga y arranca el ISO entero. Sin embargo, extraer el kernel y el initrd proporciona beneficios significativos:

###  Beneficios de la extracción de kernel

- **Tiempos de arranque más rápidos**: Solo descarga kernel/initrd (~100 MB) en vez del ISO entero (1-10 GB)
- **Menor ancho de banda**: Crítico para redes con múltiples clientes
- **Mejor compatibilidad**: Algunos ISOs no soportan `sanboot` correctamente
- **Instalación por red**: Usa archivos netboot para los instaladores de Debian/Ubuntu
- **UEFI Secure Boot**: La extracción detecta el shim firmado del ISO, permitiendo que los clientes con Secure Boot verifiquen el kernel — consulta la [guía de Secure Boot](secure-boot.md)

### Cómo extraer

**Vía interfaz web**:
1. Navega a la pestaña **Images**
2. Encuentra tu imagen ISO
3. Haz click en el botón **"Extract"**
4. Espera a que se complete la extracción

**Vía API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04-live-server-amd64.iso"}'
```

### Extracción manual

Si el extractor integrado no soporta tu ISO, puedes extraer los archivos de arranque manualmente y bootimus los detectará automáticamente.

1. Crea un directorio con el mismo nombre que el ISO (sin la extensión `.iso`):
   ```bash
   mkdir -p data/isos/my-custom-distro/
   ```

2. Coloca el kernel y el initrd en ese directorio con estos nombres exactos:
   ```
   data/isos/
   ├── my-custom-distro.iso
   └── my-custom-distro/
       ├── vmlinuz          # kernel
       └── initrd           # initrd/initramfs
   ```

3. Haz click en **"Scan for ISOs"** en el panel admin (o reinicia bootimus). La imagen se detectará automáticamente como extraída y se establecerá al método de arranque por kernel.

Esto también funciona para ISOs en subdirectorios:
```
data/isos/linux/my-custom-distro.iso
data/isos/linux/my-custom-distro/vmlinuz
data/isos/linux/my-custom-distro/initrd
```

### Qué se extrae

Bootimus detecta automáticamente la distribución y extrae:

- **Kernel**: `vmlinuz` (o `linux`, `bzImage`)
- **Initrd**: `initrd`, `initrd.gz`, `initrd.lz`
- **Squashfs** (Ubuntu/Debian live): `filesystem.squashfs`
- **Metadatos de distribución**: tipo de OS, parámetros de arranque

**Ubicación de archivos extraídos**:
```
/data/isos/
├── ubuntu-24.04.iso                    # Original ISO
└── ubuntu-24.04/                       # Extracted directory
    ├── vmlinuz                         # Kernel
    ├── initrd                          # Initrd
    └── casper/
        └── filesystem.squashfs         # Squashfs filesystem
```

### Selección automática del método de arranque

Tras la extracción, Bootimus selecciona automáticamente el método de arranque óptimo:

| Distribución | Método de arranque | Descarga |
|--------------|-------------|-----------|
| Ubuntu Desktop (extraído) | `fetch=` | ~2.8 GB (solo squashfs) |
| Ubuntu Desktop (no extraído) | `url=` | ~18 GB (ISO × 3) |
| Ubuntu Server (netboot) | Netboot | ~50 MB (archivos netboot) |
| Debian Installer (netboot) | Netboot | ~30 MB (archivos netboot) |
| Arch Linux | HTTP boot | ~100 MB (kernel/initrd) |
| Fedora/RHEL | HTTP boot | ~150 MB (kernel/initrd + stage2) |

## Soporte netboot

Algunos ISOs instaladores (Debian, Ubuntu Server) no contienen un OS completo - están diseñados para descargar paquetes durante la instalación. Para estos, Bootimus soporta descargar archivos netboot oficiales.

###  Detectar requisito de netboot

Cuando extraes un ISO instalador de Debian o Ubuntu Server, Bootimus detecta que requiere netboot:

**Indicadores**:
- El ISO contiene directorio `/install/` (no `/casper/`)
- Tipo instalador (no live/desktop)
- Tamaño pequeño de ISO (< 1 GB)

**El panel admin muestra**:
-  Badge "Netboot Required"
- 📥 Botón "Download Netboot"

### Descargar archivos netboot

**Vía interfaz web**:
1. Navega a la pestaña **Images**
2. Encuentra el ISO instalador con el badge "Netboot Required"
3. Haz click en el botón **"Download Netboot"**
4. Espera a la descarga y extracción

**Vía API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

### ¿Qué son los archivos netboot?

Los archivos netboot son archivos de arranque mínimos oficiales proporcionados por las distribuciones:

**Netboot de Debian**:
- Fuente: `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`
- Tamaño: ~30 MB
- Contiene: `vmlinuz`, `initrd.gz`, archivos del instalador

**Netboot de Ubuntu**:
- Fuente: `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`
- Tamaño: ~50 MB
- Contiene: `vmlinuz`, `initrd.gz`, archivos del instalador

### Cómo funciona netboot

1. **El cliente arranca**: Descarga el kernel/initrd netboot (~50 MB)
2. **El instalador arranca**: El initrd netboot arranca el instalador de red
3. **Descarga de paquetes**: El instalador descarga paquetes desde los mirrors de Ubuntu/Debian
4. **Instalación**: OS instalado directamente desde los repositorios de internet

**Beneficios**:
-  Siempre obtienes los últimos paquetes (no paquetes obsoletos del ISO)
-  Mínimo ancho de banda al servidor PXE (sin descarga de ISO)
-  Menores requisitos de almacenamiento
-  Archivos de arranque oficiales y firmados

### Netboot de Debian Installer

**ISOs soportados**:
- `debian-*-netinst.iso` - Instalador de red
- ISOs pequeños del instalador Debian con directorio `/install/`

**Detección**:
```
ISO structure:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**URL netboot**: `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`

**Parámetros de arranque**: `priority=critical ip=dhcp`

### Netboot de Ubuntu Server

**ISOs soportados**:
- `ubuntu-*-live-server-*.iso` - Instalador live server con directorio `/install/`
- Instaladores antiguos de Ubuntu server

**Detección**:
```
ISO structure:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**URL netboot**: `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`

**Parámetros de arranque**: `ip=dhcp`

###  Importante: Ubuntu Desktop vs Server

Hay **dos tipos** de ISOs de Ubuntu con métodos de arranque diferentes:

| Tipo | Patrón de nombre de ISO | Directorio | Método de arranque | ¿Netboot? |
|------|------------------|-----------|-------------|----------|
| **Desktop/Live** | `ubuntu-*-desktop-*.iso` | `/casper/` | `fetch=` o `url=` |  No |
| **Server Installer** | `ubuntu-*-live-server-*.iso` (con `/install/`) | `/install/` | Netboot |  Sí |

**Ubuntu Desktop** (`/casper/`):
- Contiene OS live completo
- Usa arranque casper con `fetch=` o `url=`
- Extrae el kernel para usar `fetch=` (descarga solo el squashfs)
- Sin soporte netboot

**Ubuntu Server Installer** (`/install/`):
- Instalador de red mínimo
- Requiere archivos netboot
- Descarga paquetes durante la instalación
- Mucho más eficiente

## Optimización de Ubuntu Desktop

Los ISOs de Ubuntu Desktop usan el sistema de arranque live casper. Sin optimización, descargan el ISO entero **tres veces** (~18 GB para un ISO de 6 GB).

###  Problema: triple descarga del ISO

**Comportamiento por defecto** (sin extracción):
```
Boot parameter: url=http://server/ubuntu.iso

Result:
- Download 1: Kernel verifies ISO (6GB)
- Download 2: Initrd verifies ISO (6GB)
- Download 3: Casper mounts ISO (6GB)
Total: ~18GB downloaded
```

###  Solución 1: extraer y usar el parámetro `fetch=`

**Tras la extracción**:
```
Boot parameter: fetch=http://server/ubuntu/casper/filesystem.squashfs

Result:
- Download: Only squashfs (~2.8GB)
Total: ~2.8GB downloaded
```

**Cómo habilitar**:
1. Extrae kernel/initrd del ISO
2. Bootimus usa automáticamente el parámetro `fetch=`
3. Solo se descarga el squashfs (no el ISO entero)

**Ahorro**: 85% de reducción (18 GB → 2.8 GB)

###  Solución 2: usar Ubuntu Server netboot en su lugar

Para despliegues de servidor, usa el instalador de Ubuntu Server con netboot:

**Enfoque netboot**:
```
1. Upload ubuntu-server.iso
2. Extract kernel/initrd
3. Download netboot files
4. Boot with netboot (~50MB download)
5. Install from Ubuntu repositories
```

**Ahorro**: 99% de reducción (18 GB → 50 MB)

### Referencia de parámetros de arranque

**Ubuntu Desktop (casper)**:
```bash
# Default (no extraction) - downloads ISO 3 times
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp url=http://server/ubuntu.iso

# Optimised (with extraction) - downloads squashfs once
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp fetch=http://server/ubuntu/casper/filesystem.squashfs
```

**Ubuntu Server (netboot)**:
```bash
# Netboot - minimal download
ip=dhcp
```

## Distribuciones soportadas

### Totalmente probadas

| Distribución | Extracción de kernel | Netboot | Notas |
|--------------|-------------------|---------|-------|
| **Arch Linux** |  Sí |  N/A | `/arch/boot/x86_64/vmlinuz-linux` |
| **Fedora Workstation** |  Sí |  N/A | `/isolinux/vmlinuz` |
| **Rocky Linux** |  Sí |  N/A | `/isolinux/vmlinuz` |
| **Debian (installer)** |  Sí |  Sí | `/install/vmlinuz` + netboot |
| **Debian Live** |  Sí |  No | `/live/vmlinuz` |
| **Ubuntu Desktop** |  Sí |  No | `/casper/vmlinuz` + optimización fetch |
| **Ubuntu Server** |  Sí |  Sí | `/install/vmlinuz` + netboot |
| **Pop!_OS** |  Sí |  No | `/casper/vmlinuz` |
| **TrueNAS SCALE** |  Sí |  No | 25.10.2.1: instalación y arranque desde disco en QEMU UEFI; descarga de la ISO completa |
| **Proxmox VE** |  Sí |  No | `/boot/linux26` + `/boot/initrd.img` |
| **openSUSE** |  Sí |  N/A | `/boot/x86_64/loader/linux` |
| **NixOS** |  N/A |  N/A | Sanboot |

### Patrones de detección

Bootimus detecta distribuciones escaneando patrones de archivos específicos:

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

**Ubuntu Desktop (casper)**:
```
/casper/vmlinuz or /casper/vmlinuz.efi
/casper/initrd or /casper/initrd.gz or /casper/initrd.lz
/casper/filesystem.squashfs
```

**Ubuntu Server Installer**:
```
/install/vmlinuz or /install.amd/vmlinuz
/install/initrd.gz or /install.amd/initrd.gz
```

**Debian Installer**:
```
/install/vmlinuz or /install.amd/vmlinuz
/install/initrd.gz or /install.amd/initrd.gz
```

**TrueNAS SCALE**:
```
/vmlinuz
/initrd.img
/live/filesystem.squashfs
/TrueNAS-SCALE.update
```

Bootimus reconoce SCALE por el archivo `TrueNAS-SCALE.update` y selecciona un perfil específico. Extraiga el kernel/initrd como de costumbre: el perfil descarga la ISO completa y la monta en `/cdrom` para el instalador. La ISO no se modifica. El instalador debe incluir systemd 254 o posterior.

Verificado con SCALE 25.10.2.1 en QEMU UEFI con 8 GiB de RAM: instalación en un disco virtual nuevo y arranque posterior desde él. La ISO completa permanece en RAM durante la instalación. Otras versiones, hardware físico y Secure Boot no se han probado. Este perfil no cubre TrueNAS CORE, basado en FreeBSD.

Vuelva a extraer las imágenes ya extraídas para sustituir los antiguos parámetros de Debian. Después revise los parámetros de arranque personalizados.

**Proxmox VE**:
```
/boot/linux26
/boot/initrd.img
```

## Solución de problemas

### Falló la extracción

**Síntomas**: error "Extraction failed" en el panel admin

**Causas comunes**:
1. **ISO corrupto**: Re-descarga el ISO
2. **ISO no soportado**: Comprueba si la distribución está soportada
3. **Espacio en disco**: Asegura espacio suficiente para la extracción
4. **Permisos**: Revisa los permisos de archivo en el directorio de datos

**Debugueo**:
```bash
# Check extraction logs
docker logs bootimus | grep -i extract

# Verify ISO integrity
sha256sum ubuntu.iso

# Check disk space
df -h /data/isos/

# Test manual mount
sudo mount -o loop ubuntu.iso /mnt
ls /mnt/casper/
sudo umount /mnt
```

### Falló la descarga netboot

**Síntomas**: error "Netboot download failed"

**Causas comunes**:
1. **Conectividad de red**: No se puede alcanzar los mirrors de Debian/Ubuntu
2. **URL cambió**: La URL del mirror puede haberse actualizado
3. **Falló la extracción del tarball**: Descarga corrupta

**Soluciones**:
```bash
# Test mirror connectivity
curl -I http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz

# Check server logs
docker logs bootimus | grep -i netboot

# Manually verify netboot URL
wget http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz
tar -tzf netboot.tar.gz | grep vmlinuz
```

### El menú de arranque muestra el tipo de imagen incorrecto

**Síntomas**: La imagen muestra el badge "[kernel]" pero no arranca con el método kernel

**Causa**: Base de datos y filesystem desincronizados

**Solución**:
```bash
# Re-extract kernel/initrd
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'

# Or re-scan ISOs
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### El cliente descarga el ISO múltiples veces

**Síntomas**: Ubuntu Desktop ISO se descarga 3 veces

**Causa**: Usando el parámetro `url=` sin extracción

**Solución**:
1. Extrae kernel/initrd del ISO
2. Bootimus usará automáticamente el parámetro `fetch=`
3. Solo se descarga el squashfs (no el ISO entero)

**Verificar**:
```bash
# Check if extracted
ls -la /data/isos/ubuntu-24.04/casper/filesystem.squashfs

# Check server logs during boot
docker logs -f bootimus
# Look for: "fetch=..." instead of "url=..."
```

### Netboot requerido pero sin botón de descarga

**Síntomas**: La imagen muestra "Netboot Required" pero sin botón de descarga

**Causa**: URL netboot no configurada o falló la detección

**Solución**:
```bash
# Check image details
curl -u admin:password http://localhost:8081/api/images | jq '.data[] | select(.filename=="debian-13.2.0-amd64-netinst.iso")'

# Verify netboot_required and netboot_url fields
# If netboot_url is empty, the ISO detection may have failed

# Try re-extracting
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

## Siguientes pasos

-  Mira la [Guía de la consola admin](admin.md) para gestionar imágenes
-  Lee la [Guía de despliegue](deployment.md) para configuración de almacenamiento
-  Configura el [servidor DHCP](dhcp.md) para arranque PXE
-  Configura la [Gestión de clientes](clients.md) para control de acceso
