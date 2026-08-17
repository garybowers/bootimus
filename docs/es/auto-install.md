# Guía de auto-instalación

Lanza instalaciones desatendidas en Windows, Ubuntu, Debian y las distros de la familia Red Hat. Suelta un archivo de configuración una vez, adjúntalo a una imagen, y cada arranque PXE termina la instalación sin una sola pulsación de tecla.

## Tabla de contenidos

- [Visión general](#visión-general)
- [Formatos soportados](#formatos-soportados)
- [La biblioteca de archivos](#la-biblioteca-de-archivos)
- [Adjuntar archivos](#adjuntar-archivos)
- [Orden de resolución](#orden-de-resolución)
- [Placeholders](#placeholders)
- [Ejemplos](#ejemplos)
- [Notas sobre Windows](#notas-sobre-windows)
- [API REST](#api-rest)
- [Solución de problemas](#solución-de-problemas)

## Visión general

Los configs de auto-instalación se almacenan en una biblioteca por distro bajo `data/autoinstall/`. Puedes:

- **Gestionar archivos en la UI** bajo la pestaña **Auto-Install** — crear, editar, subir, descargar y borrar configs sin tocar el filesystem.
- **Definir un default** por imagen (cada cliente que arranque esa imagen recibe el mismo config).
- **Override por cliente** o **por grupo de cliente** cuando una máquina necesita un config diferente (hostname distinto, layout de disco, rol, etc.).
- **Plantillar** con placeholders como `{{HOSTNAME}}` e `{{IP}}` resueltos al servir, usando la identidad del cliente que arranca.

Bootimus sirve el script por HTTP en el endpoint de auto-instalación, y — para Windows — coloca `AutoUnattend.xml` en el share SMB de instalación para que `setup.exe /unattend:` lo recoja automáticamente.

## Formatos soportados

| Familia de distro | Formato | Extensión | Detectado como |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| Cualquier otro | raw | cualquiera | `generic` |

La extensión determina tanto la etiqueta en la UI como la cabecera `Content-Type` cuando se sirve el archivo.

## La biblioteca de archivos

Todos los archivos de auto-instalación viven bajo `data/autoinstall/<distro>/<filename>`:

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

El segmento `<distro>` debe coincidir con un ID de perfil de distro conocido (mira [Perfiles de distro](distro-profiles.md)). El directorio se crea automáticamente en el primer arranque.

### Añadir archivos vía la UI

Pestaña **Auto-Install** → **New File** abre el editor con un selector de distro, campo de nombre de archivo y un textarea amigable para sintaxis. **Upload File** acepta cualquier archivo local y lo suelta en la carpeta de distro elegida.

### Añadir archivos manualmente

Simplemente suéltalos:

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

Aparecen en la UI inmediatamente — sin reinicio, sin scan.

## Adjuntar archivos

Los archivos de auto-instalación no tienen efecto hasta que los conectes. Hay tres lugares donde adjuntar uno:

### Imagen (default)

Pestaña **Images** → abre las **Properties** de una imagen → sección **Auto-Install** → elige un archivo. Cada cliente que arranque esta imagen recibe este config a menos que algo más específico lo sobrescriba.

### Cliente (override por máquina)

Pestaña **Clients** → abre un cliente → desplegable **Auto-Install File**. Usa esto cuando una máquina específica necesita un config diferente (p. ej., un servidor de build vs el resto de la flota del escritorio).

### Grupo de clientes (override por flota)

Pestaña **Groups** → abre un grupo → **Auto-Install File**. Se aplica a cada cliente del grupo. Útil para escenarios tipo "todos los workstations del laboratorio 3".

## Orden de resolución

Cuando un cliente pide su archivo de auto-instalación, Bootimus recorre esta jerarquía:

```
1. Per-client override        (Client.AutoInstallFile)
2. Per-group override         (ClientGroup.AutoInstallFile, if client is in a group)
3. Image default              (Image.AutoInstallFile)
4. Inline legacy script       (Image.AutoInstallScript — pre-0.1.58 setups)
5. → 404 (no auto-install configured)
```

La primera coincidencia no vacía gana. El endpoint registra la fuente desde la que sirvió:

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## Placeholders

Estos tokens se sustituyen por cliente al servir:

| Token | Reemplazado con |
|-------|---------------|
| `{{MAC}}` | Dirección MAC del cliente (minúsculas, separada por dos puntos) |
| `{{CLIENT_NAME}}` | Nombre amigable de la tabla de Clients |
| `{{HOSTNAME}}` | Igual que `{{CLIENT_NAME}}` (alias por claridad en los configs) |
| `{{IP}}` | IP del cliente que emitió la petición |
| `{{SERVER_ADDR}}` | Dirección del servidor Bootimus |
| `{{IMAGE_NAME}}` | Nombre de display de la imagen que arranca |
| `{{IMAGE_FILENAME}}` | Filename del ISO de la imagen que arranca |

Los placeholders son sustitución de strings plana — sin escape. Cítalos apropiadamente para el formato objetivo (XML, YAML, etc.).

## Ejemplos

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

Parámetros de arranque (la imagen relevante ya los tiene por defecto para Ubuntu):

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

`data/autoinstall/windows/kiosk.xml`: documento `<unattend>` estándar — mira la [referencia de autounattend de Microsoft](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/). Los placeholders funcionan dentro de cualquier nodo de texto:

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## Notas sobre Windows

Las instalaciones de Windows van por SMB. Cuando una imagen tiene un archivo autounattend adjunto, Bootimus:

1. Coloca `AutoUnattend.xml` en el share SMB de instalación al parchear `boot.wim`.
2. Parchea `startnet.cmd` para que WinPE lo copie a `X:\AutoUnattend.xml` (el RAM disk local) al arrancar.
3. Lanza Setup como `setup.exe /unattend:X:\AutoUnattend.xml`.

Sin el archivo autounattend por imagen, Setup corre interactivamente como antes.

**Resiliencia ante reinicios.** WinPE reinicia a mitad de instalación y se reconecta desde la misma IP de cliente. La config Samba incluida define `reset on zero vc = yes` y desactiva oplocks para que el segundo `net use` no tropiece con estado de sesión obsoleto. Si has reemplazado `data/smb/smb.conf` con el tuyo, replica estos settings.

**Deshacer el parche.** El primer parche SMB guarda una copia intacta de `boot.wim` como `boot.wim.orig` a su lado. **Images** → **Unpatch SMB** restaura el original y elimina el share SMB de la imagen — la ISO arranca entonces exactamente como se subió. Las imágenes parcheadas por versiones antiguas de Bootimus no tienen copia de seguridad: re-extrae primero la imagen (lo que produce una copia intacta fresca) y luego quita el parche.

## API REST

Todo lo de la UI es también una llamada REST.

```bash
# List all auto-install files
curl -u admin:pw http://localhost:8081/api/autoinstall-files

# Read a file
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/get?distro=ubuntu&filename=default.yaml"

# Create or overwrite a file
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/save \
  -H "Content-Type: application/json" \
  -d '{"distro":"ubuntu","filename":"default.yaml","content":"#cloud-config\n..."}'

# Upload a file
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/upload \
  -F "distro=windows" \
  -F "filename=kiosk.xml" \
  -F "file=@./kiosk.xml"

# Download
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/download?distro=ubuntu&filename=default.yaml" -o default.yaml

# Delete
curl -u admin:pw -X POST "http://localhost:8081/api/autoinstall-files/delete?distro=ubuntu&filename=default.yaml"
```

Adjuntar un archivo a una imagen:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

Adjuntar un archivo a un cliente:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

El endpoint de auto-instalación que los clientes consultan al arrancar:

```
GET /autoinstall/<image-filename>/?mac=<mac>
```

El query param `mac` se añade automáticamente por el menú de arranque para que los overrides por cliente se resuelvan correctamente.

## Solución de problemas

### 404 de `/autoinstall/...`

`no auto-install configuration for this image/client` — no hay nada adjuntado en ningún nivel de la cadena de resolución. O adjunta un archivo a la imagen, al cliente o a su grupo, o verifica que `auto_install_file` realmente apunta a un archivo que existe bajo `data/autoinstall/`.

### Placeholders renderizados literalmente

`{{HOSTNAME}}` apareciendo como la string literal en el sistema instalado significa que el archivo se sirvió antes de que se ejecutara la sustitución — normalmente porque el cliente arrancó solo por IP y la petición no incluyó un query param `mac`. Confirma que el menú de arranque está generando URLs del estilo `/autoinstall/<iso>/?mac=<mac>`.

### Archivo equivocado servido

La resolución es de lo más específico a lo más general. Si un cliente tiene su propio override y no lo esperabas, ese es el motivo por el que el default a nivel de imagen no se usa. Revisa la línea de log del servidor:

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

El campo `source:` te dice exactamente qué slot ganó.

### Windows Setup corre interactivamente

- La imagen debe tener un archivo autounattend adjunto (propiedades de imagen → Auto-Install).
- Re-parchea `boot.wim` tras adjuntar: **Images** → **Patch SMB** (o se re-parchea automáticamente en el siguiente arranque).
- Confirma que el share SMB es accesible desde el cliente (`net view \\<server>` desde WinPE).

### "AutoUnattend.xml not on share, running interactive setup"

Registrado por `startnet.cmd` cuando el archivo no está donde lo espera. O el paso de staging falló (revisa el log del servidor Bootimus alrededor de la hora del patch) o el share SMB perdió el archivo. Vuelve a correr el patch SMB desde las propiedades de imagen.

### `net use falla tras reiniciar la VM`

Solucionado en 0.1.58 habilitando `reset on zero vc = yes` en la config Samba incluida. Si mantienes un `smb.conf` custom, añade:

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## Siguientes pasos

- Mira la [Gestión de imágenes](images.md) para adjuntar archivos a imágenes.
- Mira la [Gestión de clientes](clients.md) para overrides por cliente.
- Mira [Perfiles de distro](distro-profiles.md) para los IDs de perfil subyacentes que mapean a los subdirectorios de la biblioteca.
