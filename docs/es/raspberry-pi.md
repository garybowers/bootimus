# Arranque por red de Raspberry Pi

Arranca por red placas Raspberry Pi de las familias 3 y 4 sin tarjeta SD. Bootimus embebe todo lo que el firmware de la Pi solicita y entrega la placa directamente al menú de arranque UEFI/iPXE estándar.

## Tabla de contenidos

- [Placas soportadas](#placas-soportadas)
- [Cómo funciona](#cómo-funciona)
- [Preparación única de la placa](#preparación-única-de-la-placa)
- [Configuración DHCP](#configuración-dhcp)
- [Overrides por máquina](#overrides-por-máquina)
- [Limitaciones](#limitaciones)

## Placas soportadas

| Placa | Estado |
|-------|--------|
| Pi 4B, Pi 400, CM4 | ✅ Soportada |
| Pi 3B, 3B+, CM3 | ✅ Soportada |
| Pi 5 | ❌ Aún no hay port UEFI (EDK2) maduro |
| Pi 2 y anteriores, Zero | ❌ Sin firmware capaz de arrancar por red |

## Cómo funciona

La boot ROM/EEPROM de la Pi descarga su firmware por TFTP, y Bootimus sirve toda la cadena desde assets embebidos — nada que preparar, nada que flashear:

```
Pi EEPROM/bootcode netboot (no SD card)
  → TFTP: bootcode.bin / start4.elf + fixup + config.txt
    → config.txt [pi3]/[pi4] filters pick the right UEFI build
      → RPI_EFI_3.fd or RPI_EFI_4.fd (Tianocore EDK2)
        → UEFI PXE → ipxe-arm64.efi → Bootimus boot menu
```

Desde el menú en adelante, una Pi es simplemente otro cliente UEFI ARM64: imágenes extraídas, asignación de imagen por cliente, auto-instalación — todo funciona igual que en x86.

El firmware solicita primero los archivos bajo un prefijo de ruta `<8-hex-serial>/`; Bootimus lo elimina automáticamente, así que una flota entera arranca desde un único set compartido de archivos sin configuración por placa.

## Preparación única de la placa

Lo único que Bootimus no puede hacer por la red es decirle a una Pi que *intente* arrancar por red — eso es política de firmware almacenada en la placa:

- **Pi 4 / 400 / CM4**: establece el orden de arranque de la EEPROM una sola vez, p. ej. vía `raspi-config` (Advanced Options → Boot Order → Network Boot), o `BOOT_ORDER=0xf12` con `rpi-eeprom-config`. Las EEPROMs más recientes recurren a la red automáticamente cuando no hay tarjeta SD presente.
- **Pi 3B+**: el soporte de arranque por red está en la boot ROM — nada que configurar; simplemente arranca sin tarjeta SD.
- **Pi 3B / CM3**: arranca una vez desde una tarjeta SD con `program_usb_boot_mode=1` en `config.txt` para establecer el bit OTP de arranque (irreversible).

## Configuración DHCP

La vía más fácil es el proxyDHCP integrado de Bootimus (`--proxy-dhcp`): reconoce las direcciones MAC de Raspberry Pi y responde al firmware con la vendor option `Raspberry Pi Boot` que este requiere, y luego sirve a la etapa UEFI ARM64 su bootfile `ipxe-arm64.efi` — mientras tu servidor DHCP existente sigue repartiendo IPs sin tocar.

Con un servidor DHCP externo, en cambio, necesitas cubrir ambas etapas:

1. La **etapa de firmware** necesita `next-server` (option 66) apuntando a Bootimus *y* la vendor option 43 conteniendo la cadena `Raspberry Pi Boot`.
2. La **etapa UEFI** necesita el bootfile `ipxe-arm64.efi` para la arquitectura de cliente 11 (UEFI ARM64).

Muchas GUIs DHCP (OPNsense/pfSense entre ellas) no pueden expresar bootfiles por arquitectura ni la vendor option de la Pi — en esas redes, corre el proxyDHCP en paralelo. Consulta la [guía DHCP](dhcp.md).

## Overrides por máquina

Para darle a una placa concreta archivos de firmware distintos o un `config.txt` custom, coloca archivos en el directorio de datos bajo el número de serie de la placa:

```
<data>/raspberrypi/<serial>/config.txt      # this Pi only
<data>/raspberrypi/config.txt               # all Pis, overrides embedded copy
```

Todo lo que no se sobrescriba recurre a los assets embebidos. El serial es el valor de 8 dígitos hexadecimales de `cat /proc/cpuinfo` en la Pi, y aparece en los logs TFTP de Bootimus cuando la placa intenta arrancar por red por primera vez.

## Limitaciones

- **Solo Ethernet por cable** — el firmware de la Pi no puede arrancar por red vía WiFi.
- **Los ajustes UEFI no persisten.** El firmware arrancado por red no tiene un almacén de variables escribible, así que siempre arranca con los defaults de pftf — incluido el límite por defecto de 3 GB de RAM en la Pi 4. Si tu SO necesita toda la RAM, construye un `RPI_EFI.fd` con el límite deshabilitado y colócalo como override.
- **La Pi 5 no está soportada** hasta que su port EDK2 madure; lo mismo aplica a cualquier placa sin firmware UEFI.
- Los assets se refrescan con `scripts/build-raspberrypi-assets.sh`, que fija y verifica por checksum las releases UEFI de [pftf](https://github.com/pftf/RPi4). Los blobs de arranque de Broadcom que empaqueta están licenciados únicamente para uso con dispositivos Raspberry Pi (`LICENCE.broadcom.txt` se distribuye junto a ellos).
