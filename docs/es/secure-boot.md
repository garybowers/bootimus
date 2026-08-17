# UEFI Secure Boot

Cómo arrancar por red máquinas que tienen UEFI Secure Boot habilitado, sin tocar la configuración de su firmware.

## Tabla de contenidos

- [Cómo funciona](#cómo-funciona)
- [Habilitar el soporte de Secure Boot](#habilitar-el-soporte-de-secure-boot)
- [Arrancar imágenes](#arrancar-imágenes)
- [Qué funciona y qué no](#qué-funciona-y-qué-no)
- [Avanzado: bootloaders autofirmados para NBD/NFS](#avanzado-bootloaders-autofirmados-para-nbdnfs)
- [Solución de problemas](#solución-de-problemas)

## Cómo funciona

Bootimus incluye un set de bootloaders integrado llamado `secureboot-official`. Contiene únicamente binarios de release firmados oficialmente, de modo que cada eslabón de la cadena de arranque se verifica contra claves en las que el firmware de fábrica ya confía — sin registro de certificados, sin cambios de firmware en los clientes:

```
Firmware (Secure Boot on, stock Microsoft keys)
  → ipxe-shimx64.efi     Microsoft-signed shim from the iPXE release
    → ipxe.efi           signed by the iPXE project's Secure Boot CA,
                         which the shim carries as its vendor certificate
      → autoexec.ipxe    fetched over TFTP; Bootimus generates it per client
        → boot menu      as normal
```

Cuando arrancas una imagen extraída, la entrada de menú generada carga el kernel y el initrd, y luego encadena el **shim firmado por Microsoft de la propia distro** desde el ISO extraído con el comando `shim` de iPXE. Ese shim verifica la firma del kernel de la distro, completando la cadena:

```ipxe
kernel http://server:8080/boot/ubuntu-26.04/vmlinuz ...
initrd http://server:8080/boot/ubuntu-26.04/initrd
iseq ${platform} efi && shim http://server:8080/boot/ubuntu-26.04/iso/EFI/boot/bootx64.efi ||
boot
```

Bootimus detecta el shim automáticamente durante la extracción del ISO. La guarda `iseq … ||` convierte la línea en un no-op en clientes BIOS y en builds de iPXE sin el comando `shim`, e iPXE solo invoca un shim cargado cuando Secure Boot se está aplicando realmente — las máquinas con Secure Boot deshabilitado arrancan exactamente igual que antes.

## Habilitar el soporte de Secure Boot

1. Abre **Bootloaders** en la UI admin y selecciona **`secureboot-official`** como set activo (o configúralo por cliente en el formulario de edición del cliente).
2. Si usas el proxyDHCP integrado, ya está — anuncia `ipxe-shimx64.efi` a los clientes UEFI automáticamente.
3. Si corres un servidor DHCP externo, cambia su bootfile UEFI a `ipxe-shimx64.efi` (y `ipxe-shimaa64.efi` para clientes ARM64). Consulta la [guía DHCP](dhcp.md).

Los binarios oficiales de iPXE no llevan ningún script de Bootimus embebido; dependen de que iPXE ≥ 2.0 descargue `autoexec.ipxe` por TFTP, que Bootimus sirve. Mantén el puerto UDP 69 accesible desde los clientes.

## Arrancar imágenes

Para clientes con Secure Boot, usa **arranque extraído (kernel/initrd)** para tus imágenes — sube el ISO y pulsa **Extract** en la lista de imágenes. Durante la extracción, Bootimus registra el shim `EFI/BOOT/BOOTX64.EFI` (o `BOOTAA64.EFI`) del ISO y lo conecta al menú automáticamente.

Las imágenes extraídas antes de que se añadiera el soporte de Secure Boot necesitan una re-extracción para recoger su shim: borra la carpeta de arranque de la imagen y extrae de nuevo.

Las instalaciones de Windows arrancan a través del `wimboot` oficial firmado por Microsoft, que el set `secureboot-official` incluye; todo lo que wimboot carga después desde `boot.wim` está firmado por Microsoft.

## Qué funciona y qué no

| Escenario | Con Secure Boot |
|-----------|-----------------|
| ISOs Linux extraídos de distros que soportan Secure Boot (Ubuntu, Debian, Fedora, openSUSE, Rocky, Alma, …) | ✅ Funciona |
| Instalador de Windows (wimboot) | ✅ Funciona |
| Distros que no soportan Secure Boot en sus propios medios (Arch, Alpine, Void, …) | ❌ Sus kernels no están firmados — igual que arrancar desde su USB. Deshabilita Secure Boot para estas. |
| Arranque `sanboot` (ISO entero) | ❌ Usa arranque extraído en su lugar |
| Métodos de arranque NBD / NFS | ❌ Arrancan el kernel sin firmar propio de Bootimus — ver [abajo](#avanzado-bootloaders-autofirmados-para-nbdnfs) |
| Imágenes de herramientas (ShredOS, memtest, …) | ❌ Kernels sin firmar |

## Avanzado: bootloaders autofirmados para NBD/NFS

Los métodos de arranque NBD y NFS cargan el kernel del entorno de arranque propio de Bootimus, que ninguna CA pública va a firmar. Si los necesitas con Secure Boot, construye un set autofirmado y registra tu certificado en cada cliente una sola vez:

```bash
scripts/generate-signing-key.sh
BOOTIMUS_SIGNING_KEY=… BOOTIMUS_SIGNING_CERT=… scripts/build-secureboot-set.sh
```

Copia la salida a `<data>/bootloaders/secureboot/`, selecciónalo en la UI, y luego registra el certificado en cada cliente con `mokutil --import bootimus-signing.crt` (o vía la gestión de claves del firmware). Para la mayoría de despliegues, deshabilitar Secure Boot en las máquinas afectadas es la respuesta más simple.

## Solución de problemas

- **Pantalla de "Security Violation" del firmware antes de que cargue iPXE** — el cliente no está descargando `ipxe-shimx64.efi`. Comprueba el bootfile de tu DHCP y que el set `secureboot-official` esté activo.
- **iPXE carga pero no aparece ningún menú** — los binarios oficiales necesitan `autoexec.ipxe` por TFTP. Comprueba UDP 69 entre cliente y servidor.
- **Una línea de "Security Violation" en el log de iPXE al arrancar una imagen** — esperado e inofensivo; es iPXE sondeando la imagen del shim, no un fallo.
- **Las teclas del menú no responden en cierto hardware** — iPXE v2.0.0 tiene una regresión conocida de teclado en menús interactivos en ciertas máquinas. Establece una imagen por defecto o usa la selección next-boot como workaround, o cambia ese cliente al set `default` si no necesita Secure Boot.
- **El kernel se niega a cargar con Secure Boot** — probablemente la imagen se extrajo antes de que existiera el soporte de shim (re-extráela), o la distro no soporta Secure Boot en absoluto.
