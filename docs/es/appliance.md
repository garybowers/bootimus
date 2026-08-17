# Appliance USB de Bootimus

Una imagen flasheable y autocontenida de Alpine Linux que arranca en un servidor PXE Bootimus listo para usar. Enchúfalo a un switch, dale corriente, y cada máquina del mismo dominio de broadcast podrá arrancar por PXE contra él — sin reconfigurar DHCP, sin instalar el OS, sin setup.

## Qué lleva dentro

- **Alpine Linux** (mínimo, base de ~100 MB)
- **bootimus** con proxyDHCP habilitado por defecto
- **Samba** sirviendo `/var/lib/bootimus/isos` en solo lectura como `\\BOOTIMUS\isos` para instaladores Windows que quieran acceso SMB durante el setup
- Paquete **dnsmasq** disponible pero deshabilitado (el proxyDHCP integrado de bootimus cubre esto por defecto)
- **Servidor SSH** para administración remota

## Construir la imagen

Requisitos en la máquina de build:
- Docker (con `--privileged` disponible)
- Go 1.24+ para cross-compilar bootimus
- ~3 GB libres en disco

El build corre entero dentro de un contenedor privilegiado de Alpine — no se cargan módulos del kernel del host, ni se instalan herramientas en tu máquina.

```bash
make appliance
```

Produce `appliance/build/bootimus-appliance.img` — una imagen de disco plana lista para flashear con Etcher, Rufus o `dd`.

## Flashear a un USB

**Identifica tu dispositivo objetivo con cuidado** — `dd` sobreescribirá sin preguntar.

```bash
lsblk                                   # find your USB stick, e.g. /dev/sdb
sudo dd if=appliance/build/bootimus-appliance.img \
        of=/dev/sdX bs=4M conv=fsync status=progress
sync
```

En macOS/Windows, [Etcher](https://etcher.balena.io) o [Rufus](https://rufus.ie) funcionan con el archivo `.img` directamente.

## Primer arranque

1. Enchufa el USB a cualquier PC con Ethernet y red cableada.
2. Arranca desde USB (menú de arranque one-time o cambio de prioridad en BIOS).
3. Alpine arranca, pide IP por DHCP a la LAN, y arranca bootimus + samba + proxyDHCP.
4. La consola muestra:

   ```
    ____              _   _
   | __ )  ___   ___ | |_(_)_ __ ___  _   _ ___
   |  _ \ / _ \ / _ \| __| | '_ ` _ \| | | / __|
   | |_) | (_) | (_) | |_| | | | | | | |_| \__ \
   |____/ \___/ \___/ \__|_|_| |_| |_|\__,_|___/

     Appliance: bootimus
     Admin UI:  http://10.0.0.42:8081
     PXE HTTP:  http://10.0.0.42:8080
     SMB share: //10.0.0.42/isos  (read-only, guest)
     Initial admin password: <printed once>
     (delete /var/lib/bootimus/admin-password.txt after you've saved it)
   ```

5. Abre la URL admin desde cualquier otra máquina de la LAN. Inicia sesión como `admin` con la contraseña impresa.
6. Sube o escanea ISOs desde la UI admin — aterrizan en `/var/lib/bootimus/isos` y se sirven inmediatamente por HTTP *y* por el recurso compartido SMB.

## Salvedades y tradeoffs

- **Solo red cableada.** No se incluye firmware de drivers WiFi. Servir PXE por WiFi es una idea horrible de todos modos (broadcast-flooding + latencia).
- **UEFI Secure Boot soportado** — selecciona el set de bootloaders integrado `secureboot-official` y las máquinas objetivo con Secure Boot activado arrancan por red sin cambios de firmware, igual que en una instalación normal de bootimus. Consulta la [guía de Secure Boot](secure-boot.md) para las limitaciones (los arranques NBD/NFS y las distros sin firmar siguen necesitando Secure Boot desactivado).
- **Una sola partición.** Los ISOs viven en la misma partición raíz que Alpine. Un USB de 32 GB te da ~29 GB para ISOs. Para una biblioteca más grande, extiende la partición raíz manualmente tras el primer arranque (`resize2fs /dev/sda1`) o reconstruye con `IMAGE_SIZE=16G make appliance`.
- **Coexistencia de proxyDHCP.** Si la LAN a la que te enchufas ya tiene un proxyDHCP de dnsmasq/ISC anunciando PXE, dos proxies se pelearán. Deshabilita uno: o pones `BOOTIMUS_PROXY_DHCP_ENABLED=false` en `/etc/conf.d/bootimus` o apagas el otro.
- **El appliance tiene estado.** El USB ES el servidor. ISOs, clientes, schedules y settings persisten en él. Si el USB muere a mitad de despliegue querrás un backup (`make appliance` produce builds deterministas pero tus *datos* viven en el USB — usa el botón "Download Backup" en Settings con regularidad).

## Personalizar

El build se controla con tres piezas:

- **`appliance/build.sh`** — orquestador. Ajusta los env vars `IMAGE_SIZE` y `ALPINE_BRANCH` sin editar código.
- **`appliance/setup.sh`** — corre dentro del chroot de la imagen durante el build. Añade líneas `apk add` aquí para empaquetar herramientas extra.
- **`appliance/overlay/`** — cualquier archivo aquí se copia al rootfs tal cual. Ediciones comunes:
  - `etc/conf.d/bootimus` — apagar proxyDHCP, cambiar puertos, fijar una IP de servidor específica
  - `etc/samba/smb.conf` — ampliar el share SMB, añadir tweaks específicos de Windows
  - `etc/network/interfaces` — IP estática en vez de DHCP
  - `etc/profile.d/bootimus-motd.sh` — cambiar el banner de login

Tras cualquier cambio, vuelve a correr `make appliance`.

## Acceso SSH

El login root por contraseña está **deshabilitado** en la imagen (higiene de seguridad — te sorprendería cuántos appliances "seguros" salen con credenciales por defecto). Para habilitar la admin remota:

1. Arranca el appliance una vez en la consola.
2. Corre `passwd` para establecer una contraseña de root, O suelta una clave SSH en `/root/.ssh/authorized_keys`.
3. `rc-service sshd restart` (SSH ya está habilitado por defecto pero no acepta logins sin contraseña).

## Reconstruir con una nueva release de bootimus

Cada `make appliance` toma el árbol de fuentes actual de bootimus. Sube `VERSION`, corta una release, luego reconstruye la imagen — el binario `bootimus` incluido reporta la versión contra la que se construyó en el footer de la UI admin.
