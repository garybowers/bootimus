# Raspberry Pi Netboot

Boote Raspberry-Pi-Boards der 3er- und 4er-Familie übers Netzwerk — ganz ohne SD-Karte. Bootimus bettet alles ein, was die Firmware des Pi anfragt, und übergibt das Board direkt ins normale UEFI/iPXE-Boot-Menü.

## Inhaltsverzeichnis

- [Unterstützte Boards](#unterstützte-boards)
- [Funktionsweise](#funktionsweise)
- [Einmalige Board-Vorbereitung](#einmalige-board-vorbereitung)
- [DHCP-Setup](#dhcp-setup)
- [Overrides pro Maschine](#overrides-pro-maschine)
- [Einschränkungen](#einschränkungen)

## Unterstützte Boards

| Board | Status |
|-------|--------|
| Pi 4B, Pi 400, CM4 | ✅ Unterstützt |
| Pi 3B, 3B+, CM3 | ✅ Unterstützt |
| Pi 5 | ❌ Noch kein ausgereifter UEFI-(EDK2-)Port |
| Pi 2 und älter, Zero | ❌ Keine netboot-fähige Firmware |

## Funktionsweise

Das Boot-ROM/EEPROM des Pi holt seine Firmware per TFTP, und Bootimus liefert die gesamte Chain aus eingebetteten Assets — nichts zu stagen, nichts zu flashen:

```
Pi EEPROM/bootcode netboot (no SD card)
  → TFTP: bootcode.bin / start4.elf + fixup + config.txt
    → config.txt [pi3]/[pi4] filters pick the right UEFI build
      → RPI_EFI_3.fd or RPI_EFI_4.fd (Tianocore EDK2)
        → UEFI PXE → ipxe-arm64.efi → Bootimus boot menu
```

Ab dem Menü ist ein Pi einfach ein weiterer ARM64-UEFI-Client: extrahierte Images, Image-Zuweisung pro Client, Auto-Install — alles funktioniert genauso wie auf x86.

Die Firmware fragt Dateien zuerst unter einem `<8-hex-serial>/`-Pfadpräfix an; Bootimus entfernt ihn automatisch, sodass eine ganze Flotte von einem gemeinsamen Satz Dateien bootet — ohne Setup pro Board.

## Einmalige Board-Vorbereitung

Das Einzige, was Bootimus nicht übers Netzwerk erledigen kann, ist einem Pi zu sagen, dass er das Netbooten überhaupt *versuchen* soll — das ist Firmware-Policy, die auf dem Board gespeichert ist:

- **Pi 4 / 400 / CM4**: setze die EEPROM-Boot-Reihenfolge einmalig, z.B. über `raspi-config` (Advanced Options → Boot Order → Network Boot) oder mit `BOOT_ORDER=0xf12` via `rpi-eeprom-config`. Neuere EEPROMs fallen automatisch auf Netzwerk-Boot zurück, wenn keine SD-Karte steckt.
- **Pi 3B+**: die Netzwerk-Boot-Unterstützung steckt im Boot-ROM — nichts zu konfigurieren; einfach ohne SD-Karte booten.
- **Pi 3B / CM3**: boote einmal von einer SD-Karte mit `program_usb_boot_mode=1` in der `config.txt`, um das (unumkehrbare) OTP-Boot-Bit zu setzen.

## DHCP-Setup

Der einfachste Weg ist Bootimus' eingebautes proxyDHCP (`--proxy-dhcp`): es erkennt Raspberry-Pi-MAC-Adressen und beantwortet die Firmware mit der von ihr benötigten Vendor-Option `Raspberry Pi Boot`, und liefert der UEFI-Stage anschließend ihr ARM64-Bootfile `ipxe-arm64.efi` — während dein bestehender DHCP-Server unangetastet weiter IPs verteilt.

Mit einem externen DHCP-Server musst du stattdessen beide Stages abdecken:

1. Die **Firmware-Stage** braucht `next-server` (Option 66) mit Bootimus als Ziel *und* Vendor-Option 43 mit dem String `Raspberry Pi Boot`.
2. Die **UEFI-Stage** braucht das Bootfile `ipxe-arm64.efi` für Client-Architektur 11 (UEFI ARM64).

Viele DHCP-GUIs (darunter OPNsense/pfSense) können weder Bootfiles pro Architektur noch die Pi-Vendor-Option ausdrücken — betreibe in solchen Netzen das proxyDHCP parallel dazu. Siehe den [DHCP-Leitfaden](dhcp.md).

## Overrides pro Maschine

Um einem bestimmten Board andere Firmware-Dateien oder eine eigene `config.txt` zu geben, lege Dateien im Datenverzeichnis unter der Seriennummer des Boards ab:

```
<data>/raspberrypi/<serial>/config.txt      # this Pi only
<data>/raspberrypi/config.txt               # all Pis, overrides embedded copy
```

Alles, was nicht überschrieben wird, fällt auf die eingebetteten Assets zurück. Die Seriennummer ist der 8-stellige Hex-Wert aus `cat /proc/cpuinfo` auf dem Pi und taucht in Bootimus' TFTP-Logs auf, sobald das Board zum ersten Mal zu netbooten versucht.

## Einschränkungen

- **Nur kabelgebundenes Ethernet** — die Pi-Firmware kann nicht über WLAN netbooten.
- **UEFI-Einstellungen bleiben nicht erhalten.** Die netgebootete Firmware hat keinen beschreibbaren Variable Store und bootet daher immer mit pftf-Defaults — inklusive des standardmäßigen 3-GB-RAM-Limits auf dem Pi 4. Wenn dein OS den ganzen RAM braucht, baue eine `RPI_EFI.fd` mit deaktiviertem Limit und lege sie als Override ab.
- **Pi 5 wird nicht unterstützt**, bis sein EDK2-Port ausgereift ist; dasselbe gilt für jedes Board ohne UEFI-Firmware.
- Die Assets werden mit `scripts/build-raspberrypi-assets.sh` aktualisiert, das die [pftf](https://github.com/pftf/RPi4)-UEFI-Releases pinnt und per Checksumme verifiziert. Die gebündelten Broadcom-Boot-Blobs sind ausschließlich für die Nutzung mit Raspberry-Pi-Geräten lizenziert (`LICENCE.broadcom.txt` liegt bei).
