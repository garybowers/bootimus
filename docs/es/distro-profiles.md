# Guía de perfiles de distro

Bootimus usa perfiles de distro para detectar tipos de ISO y generar los parámetros de arranque correctos. Los perfiles son data-driven — puedes añadir soporte para nuevas distribuciones sin cambiar el código.

## Tabla de contenidos

- [Visión general](#visión-general)
- [Cómo funciona](#cómo-funciona)
- [Ver perfiles](#ver-perfiles)
- [Actualizar perfiles](#actualizar-perfiles)
- [Actualizaciones remotas y privacidad](#actualizaciones-remotas-y-privacidad)
- [Crear perfiles custom](#crear-perfiles-custom)
- [Campos del perfil](#campos-del-perfil)
- [Placeholders](#placeholders)
- [Ejemplos](#ejemplos)
- [Solución de problemas](#solución-de-problemas)

## Visión general

Los perfiles de distro definen:
- **Cómo detectar** qué distro es un ISO (match de patrones de filename)
- **Dónde encontrar** el kernel, initrd y squashfs dentro del ISO
- **Qué parámetros de arranque** usar al arrancar por PXE
- **Qué tipo de auto-instalación** se soporta (preseed, kickstart, autoinstall, etc.)

### Tipos de perfil

| Tipo | Descripción |
|------|-------------|
| **Built-in** | Incluido con Bootimus, actualizado desde el repositorio central |
| **Custom** | Creado por el usuario, nunca sobrescrito por actualizaciones |

Los perfiles custom siempre tienen prioridad sobre los built-in al hacer match de nombres de ISO.

## Cómo funciona

1. Cuando un ISO se sube o se extrae, Bootimus hace match del filename contra los patrones de perfil
2. Los paths de kernel/initrd del perfil coincidente se usan para localizar archivos de arranque dentro del ISO
3. Los boot params del perfil se convierten en el default (editable en las Properties de la imagen)
4. En el momento del arranque, los placeholders en los params se resuelven a URLs reales

### Ciclo de vida del perfil

```
Build time:    distro-profiles.json embedded in binary
                        ↓
First startup:  Profiles seeded into database
                        ↓
"Check for Updates":  Latest profiles fetched from GitHub
                        ↓
User creates:   Custom profiles stored in database (never overwritten)
```

## Ver perfiles

Navega a **Boot > Distro Profiles** en el panel admin para ver todos los perfiles cargados con sus patrones de filename, parámetros de arranque, tipo (Built-in/Custom) y versión.

## Actualizar perfiles

Actualizar perfiles es siempre una **acción explícita y bajo demanda** — Bootimus nunca contacta el catálogo remoto por su cuenta. Hasta que disparas una actualización, se usan los perfiles embebidos en el binario en el momento de la compilación. Consulta [Actualizaciones remotas y privacidad](#actualizaciones-remotas-y-privacidad) para saber exactamente qué se contacta y cómo desactivarlo.

Cuando disparas una actualización:

- Se añaden los perfiles nuevos
- Los perfiles built-in existentes se actualizan a la última versión
- Los perfiles custom nunca se modifican

Hay tres formas de dispararla:

### Desde la interfaz admin

Haz click en **"Check for Updates"** en la pestaña **Boot > Distro Profiles**.

### Desde la CLI

```bash
bootimus profiles update
```

Esto usa la misma configuración de base de datos que `serve` (PostgreSQL si `db.host` está definido, en caso contrario la base de datos SQLite local bajo `data_dir`). Respeta `--disable-remote-profiles` y sale sin contactar la red cuando ese flag está activado.

### Vía API

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/profiles/update
```

Respuesta:
```json
{
  "success": true,
  "message": "Updated to version 0.1.21 (2 added, 5 updated)"
}
```

## Actualizaciones remotas y privacidad

Bootimus es self-hosted y no llama a casa en segundo plano. La única vez que contacta un servicio externo para obtener perfiles es cuando **tú** disparas explícitamente una actualización mediante uno de los métodos anteriores.

**Endpoint contactado (solo en una actualización explícita):**

```
https://raw.githubusercontent.com/garybowers/bootimus/main/distro-profiles.json
```

El catálogo equivalente de herramientas ("Check for Updates" en la pestaña **Tools** / `POST /api/tools/update`) se comporta de la misma manera y contacta:

```
https://raw.githubusercontent.com/garybowers/bootimus/main/tools-profiles.json
```

Estas son peticiones `GET` simples y sin autenticar al host de archivos estáticos de GitHub. Bootimus no envía información del sistema, identificadores ni datos de uso con ellas — simplemente descarga el archivo JSON. Ten en cuenta que, como con cualquier petición HTTP, GitHub ve tu dirección IP de origen y los metadatos estándar de la petición.

### Desactivar las actualizaciones remotas

Para garantizar que Bootimus nunca contacte el catálogo remoto — en despliegues air-gapped, o como cuestión de política — arráncalo con:

```bash
bootimus serve --disable-remote-profiles
```

o define el valor equivalente de config/env:

```yaml
# bootimus.yaml
disable_remote_profiles: true
```

```bash
# environment variable
BOOTIMUS_DISABLE_REMOTE_PROFILES=true
```

Cuando está desactivado, los perfiles embebidos se siguen sembrando desde el binario en el primer arranque, así que Bootimus es totalmente funcional sin conexión. El botón "Check for Updates", el endpoint `/api/profiles/update` y `bootimus profiles update` se negarán todos a ejecutarse.

## Crear perfiles custom

### Vía interfaz web

1. Ve a **Boot > Distro Profiles**
2. Haz click en **"+ Add Custom Profile"**
3. Rellena los campos del perfil
4. Haz click en **"Create Profile"**

### Vía API

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

### Borrar perfiles custom

Solo se pueden borrar perfiles custom. Los perfiles built-in se restauran en la próxima actualización.

```bash
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/profiles/delete?id=my-distro"
```

## Campos del perfil

| Campo | Requerido | Descripción |
|-------|----------|-------------|
| `profile_id` | Sí | Identificador único (p. ej., `ubuntu`, `my-distro`) |
| `display_name` | Sí | Nombre legible mostrado en la UI |
| `family` | No | Familia de distro (p. ej., `debian`, `arch`, `redhat`) — para agrupar |
| `filename_patterns` | Sí | Substrings a buscar en nombres de ISO (case-insensitive) |
| `kernel_paths` | No | Paths a probar para el kernel dentro del ISO (p. ej., `/casper/vmlinuz`) |
| `initrd_paths` | No | Paths a probar para el initrd dentro del ISO |
| `squashfs_paths` | No | Paths a probar para el filesystem root squashfs |
| `default_boot_params` | No | Parámetros de arranque del kernel por defecto (con soporte de placeholders) |
| `boot_params_with_squashfs` | No | Boot params alternativos usados cuando se detecta squashfs |
| `auto_install_type` | No | Formato de auto-instalación: `preseed`, `kickstart`, `autoinstall`, `autounattend` |
| `boot_method` | No | Override del método de arranque (p. ej., `wimboot` para Windows) |

## Placeholders

Los parámetros de arranque soportan estos placeholders, resueltos en el momento del arranque:

| Placeholder | Se resuelve a | Ejemplo |
|-------------|-------------|---------|
| `{{BASE_URL}}` | URL HTTP del servidor | `http://192.168.1.10:8080` |
| `{{CACHE_DIR}}` | Directorio de archivos extraídos | `ubuntu-24.04-server-amd64` |
| `{{FILENAME}}` | Filename del ISO (URL-encoded) | `ubuntu-24.04-server-amd64.iso` |
| `{{SQUASHFS}}` | URL completa al archivo squashfs | `http://192.168.1.10:8080/boot/ubuntu.../casper/filesystem.squashfs` |

### Ejemplo con placeholders

```
boot=live initrd=initrd fetch={{SQUASHFS}} ip=dhcp
```

Se resuelve a:
```
boot=live initrd=initrd fetch=http://192.168.1.10:8080/boot/debian-live-13/live/filesystem.squashfs ip=dhcp
```

## Ejemplos

### ISO live basada en Debian

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

### Distro basada en Arch

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

### Instalador basado en RHEL

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

## Solución de problemas

### ISO no detectado como la distro correcta

Comprueba si el filename del ISO coincide con algún patrón de perfil:

1. Ve a la pestaña **Distro Profiles**
2. Mira la columna "Filename Patterns"
3. Si ningún patrón coincide con el filename de tu ISO, crea un perfil custom

### Boot params incorrectos tras la extracción

1. Abre las **Properties** de la imagen
2. Haz click en **"Re-detect"** junto a Boot Parameters
3. O edita los boot params manualmente — soportan placeholders

### "Check for Updates" falló

La actualización descarga desde GitHub. Comprueba:
- El servidor tiene acceso a internet
- `raw.githubusercontent.com` no está bloqueado
- Inténtalo más tarde si GitHub está caído

### El perfil custom no coincide

Los perfiles custom tienen prioridad sobre los built-in. Asegúrate de que:
- Los `filename_patterns` contienen substrings que coinciden con tu filename ISO (case-insensitive)
- El profile ID es único
- El perfil se guardó correctamente

### Contribuir perfiles

Para añadir un perfil a la lista oficial para todos los usuarios:
1. Forkea el [repositorio de Bootimus](https://github.com/garybowers/bootimus)
2. Edita `distro-profiles.json` en la raíz del repo
3. Añade tu perfil al array `profiles`
4. Envía un pull request

Así todos los usuarios de Bootimus obtienen el nuevo perfil vía "Check for Updates".
