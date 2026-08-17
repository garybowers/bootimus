# UEFI Secure Boot

Wie du Maschinen mit aktiviertem UEFI Secure Boot übers Netzwerk bootest, ohne ihre Firmware-Einstellungen anzufassen.

## Inhaltsverzeichnis

- [Funktionsweise](#funktionsweise)
- [Secure-Boot-Unterstützung aktivieren](#secure-boot-unterstützung-aktivieren)
- [Images booten](#images-booten)
- [Was funktioniert und was nicht](#was-funktioniert-und-was-nicht)
- [Fortgeschritten: selbstsignierte Bootloader für NBD/NFS](#fortgeschritten-selbstsignierte-bootloader-für-nbdnfs)
- [Fehlersuche](#fehlersuche)

## Funktionsweise

Bootimus bringt ein eingebautes Bootloader-Set namens `secureboot-official` mit. Es enthält ausschließlich offiziell signierte Release-Binaries, sodass jedes Glied der Boot-Chain gegen Schlüssel verifiziert wird, denen Stock-Firmware bereits vertraut — kein Zertifikats-Enrolment, keine Firmware-Änderungen auf den Clients:

```
Firmware (Secure Boot on, stock Microsoft keys)
  → ipxe-shimx64.efi     Microsoft-signed shim from the iPXE release
    → ipxe.efi           signed by the iPXE project's Secure Boot CA,
                         which the shim carries as its vendor certificate
      → autoexec.ipxe    fetched over TFTP; Bootimus generates it per client
        → boot menu      as normal
```

Wenn du ein extrahiertes Image bootest, lädt der generierte Menüeintrag Kernel und initrd und chain-lädt anschließend mit iPXEs `shim`-Befehl den **Microsoft-signierten shim der Distribution selbst** aus dem extrahierten ISO. Dieser shim verifiziert die Signatur des Distro-Kernels und vervollständigt damit die Chain:

```ipxe
kernel http://server:8080/boot/ubuntu-26.04/vmlinuz ...
initrd http://server:8080/boot/ubuntu-26.04/initrd
iseq ${platform} efi && shim http://server:8080/boot/ubuntu-26.04/iso/EFI/boot/bootx64.efi ||
boot
```

Bootimus erkennt den shim automatisch während der ISO-Extraktion. Der `iseq … ||`-Guard macht die Zeile auf BIOS-Clients und auf iPXE-Builds ohne `shim`-Befehl zu einem No-op, und iPXE ruft einen geladenen shim nur auf, wenn Secure Boot tatsächlich erzwungen wird — Maschinen mit deaktiviertem Secure Boot booten exakt wie bisher.

## Secure-Boot-Unterstützung aktivieren

1. Öffne **Bootloaders** im Admin-UI und wähle **`secureboot-official`** als aktives Set (oder setze es pro Client im Edit-Formular des Clients).
2. Wenn du das eingebaute proxyDHCP nutzt, bist du fertig — es advertised `ipxe-shimx64.efi` automatisch an UEFI-Clients.
3. Wenn du einen externen DHCP-Server betreibst, ändere dessen UEFI-Bootfile-Namen auf `ipxe-shimx64.efi` (und `ipxe-shimaa64.efi` für ARM64-Clients). Siehe den [DHCP-Leitfaden](dhcp.md).

Die offiziellen iPXE-Binaries tragen kein eingebettetes Bootimus-Skript; sie verlassen sich darauf, dass iPXE ≥ 2.0 `autoexec.ipxe` per TFTP holt, das Bootimus ausliefert. Halte UDP-Port 69 von den Clients aus erreichbar.

## Images booten

Nutze für Secure-Boot-Clients den **extrahierten (Kernel/initrd-)Boot** deiner Images — lade das ISO hoch und klicke in der Image-Liste auf **Extract**. Während der Extraktion registriert Bootimus den `EFI/BOOT/BOOTX64.EFI`- (bzw. `BOOTAA64.EFI`-)shim des ISOs und verdrahtet ihn automatisch ins Menü.

Images, die vor der Secure-Boot-Unterstützung extrahiert wurden, brauchen eine erneute Extraktion, um ihren shim aufzunehmen: lösche den Boot-Ordner des Images und extrahiere erneut.

Windows-Installationen booten über das offizielle Microsoft-signierte `wimboot`, das im Set `secureboot-official` enthalten ist; alles, was wimboot danach aus `boot.wim` lädt, ist Microsoft-signiert.

## Was funktioniert und was nicht

| Szenario | Unter Secure Boot |
|----------|------------------|
| Extrahierte Linux-ISOs von Distributionen mit Secure-Boot-Unterstützung (Ubuntu, Debian, Fedora, openSUSE, Rocky, Alma, …) | ✅ Funktioniert |
| Windows-Installer (wimboot) | ✅ Funktioniert |
| Distributionen, die Secure Boot auf ihren eigenen Medien nicht unterstützen (Arch, Alpine, Void, …) | ❌ Ihre Kernel sind unsigniert — genau wie beim Booten von deren USB-Stick. Deaktiviere Secure Boot für diese. |
| `sanboot`-Boot (ganzes ISO) | ❌ Nutze stattdessen extrahierten Boot |
| NBD-/NFS-Boot-Methoden | ❌ Sie booten Bootimus' eigenen unsignierten Kernel — siehe [unten](#fortgeschritten-selbstsignierte-bootloader-für-nbdnfs) |
| Tool-Images (ShredOS, memtest, …) | ❌ Unsignierte Kernel |

## Fortgeschritten: selbstsignierte Bootloader für NBD/NFS

Die NBD- und NFS-Boot-Methoden laden Bootimus' eigenen Boot-Environment-Kernel, den keine öffentliche CA signieren wird. Wenn du diese Methoden unter Secure Boot brauchst, baue ein selbstsigniertes Set und enrole dein Zertifikat einmalig auf jedem Client:

```bash
scripts/generate-signing-key.sh
BOOTIMUS_SIGNING_KEY=… BOOTIMUS_SIGNING_CERT=… scripts/build-secureboot-set.sh
```

Kopiere die Ausgabe nach `<data>/bootloaders/secureboot/`, wähle das Set im UI aus und enrole dann das Zertifikat auf jedem Client mit `mokutil --import bootimus-signing.crt` (oder über das Key-Management der Firmware). Für die meisten Deployments ist es die einfachere Antwort, Secure Boot auf den betroffenen Maschinen zu deaktivieren.

## Fehlersuche

- **"Security Violation"-Meldung der Firmware, bevor iPXE lädt** — der Client holt `ipxe-shimx64.efi` nicht. Prüfe deinen DHCP-Bootfile-Namen und ob das Set `secureboot-official` aktiv ist.
- **iPXE lädt, aber kein Menü erscheint** — die offiziellen Binaries brauchen `autoexec.ipxe` per TFTP. Prüfe UDP 69 zwischen Client und Server.
- **Eine "Security Violation"-Zeile im iPXE-Log beim Booten eines Images** — erwartet und harmlos; iPXE probt damit das shim-Image, das ist kein Fehler.
- **Menü-Tasten reagieren auf mancher Hardware nicht** — iPXE v2.0.0 hat eine bekannte Keyboard-Regression in interaktiven Menüs auf bestimmten Maschinen. Setze als Workaround ein Default-Image oder nutze die Next-Boot-Auswahl, oder stelle den Client auf das Set `default` um, falls er kein Secure Boot braucht.
- **Kernel lädt unter Secure Boot nicht** — das Image wurde vermutlich extrahiert, bevor es die shim-Unterstützung gab (erneut extrahieren), oder die Distribution unterstützt Secure Boot gar nicht.
