# Netboot Raspberry Pi

Netboote les cartes Raspberry Pi des familles 3 et 4 sans carte SD. Bootimus embarque tout ce que le firmware du Pi demande et amène la carte directement dans le menu de boot UEFI/iPXE standard.

## Table des matières

- [Cartes supportées](#cartes-supportées)
- [Fonctionnement](#fonctionnement)
- [Préparation unique de la carte](#préparation-unique-de-la-carte)
- [Configuration DHCP](#configuration-dhcp)
- [Overrides par machine](#overrides-par-machine)
- [Limitations](#limitations)

## Cartes supportées

| Carte | Statut |
|-------|--------|
| Pi 4B, Pi 400, CM4 | ✅ Supporté |
| Pi 3B, 3B+, CM3 | ✅ Supporté |
| Pi 5 | ❌ Pas encore de port UEFI (EDK2) mature |
| Pi 2 et antérieurs, Zero | ❌ Pas de firmware capable de netbooter |

## Fonctionnement

Le boot ROM/EEPROM du Pi récupère son firmware via TFTP, et Bootimus sert toute la chaîne depuis des assets embarqués — rien à préparer, rien à flasher :

```
Pi EEPROM/bootcode netboot (no SD card)
  → TFTP: bootcode.bin / start4.elf + fixup + config.txt
    → config.txt [pi3]/[pi4] filters pick the right UEFI build
      → RPI_EFI_3.fd or RPI_EFI_4.fd (Tianocore EDK2)
        → UEFI PXE → ipxe-arm64.efi → Bootimus boot menu
```

À partir du menu, un Pi n'est qu'un client UEFI ARM64 comme un autre : images extraites, assignation d'image par client, auto-install — tout marche pareil que sur x86.

Le firmware demande d'abord les fichiers sous un préfixe de chemin `<8-hex-serial>/` ; Bootimus le retire automatiquement, donc toute une flotte boote depuis un seul jeu de fichiers partagé, sans configuration par carte.

## Préparation unique de la carte

La seule chose que Bootimus ne peut pas faire via le réseau, c'est dire à un Pi d'*essayer* de netbooter — c'est une politique du firmware, stockée sur la carte :

- **Pi 4 / 400 / CM4** : définis l'ordre de boot de l'EEPROM une fois, par ex. via `raspi-config` (Advanced Options → Boot Order → Network Boot), ou `BOOT_ORDER=0xf12` avec `rpi-eeprom-config`. Les EEPROMs récents basculent automatiquement sur le réseau quand aucune carte SD n'est présente.
- **Pi 3B+** : le support du boot réseau est dans le boot ROM — rien à configurer ; boote simplement sans carte SD.
- **Pi 3B / CM3** : boote une fois depuis une carte SD avec `program_usb_boot_mode=1` dans `config.txt` pour définir le bit de boot OTP (irréversible).

## Configuration DHCP

Le chemin le plus simple est le proxyDHCP intégré de Bootimus (`--proxy-dhcp`) : il reconnaît les adresses MAC Raspberry Pi et répond au firmware avec l'option vendor `Raspberry Pi Boot` qu'il exige, puis sert à l'étape UEFI ARM64 son bootfile `ipxe-arm64.efi` — pendant que ton serveur DHCP existant continue à distribuer les IPs sans être touché.

Avec un serveur DHCP externe à la place, tu dois couvrir les deux étapes :

1. L'**étape firmware** a besoin de `next-server` (option 66) pointant vers Bootimus *et* de l'option vendor 43 contenant la chaîne `Raspberry Pi Boot`.
2. L'**étape UEFI** a besoin du bootfile `ipxe-arm64.efi` pour l'architecture client 11 (UEFI ARM64).

Beaucoup de GUIs DHCP (OPNsense/pfSense entre autres) ne peuvent exprimer ni les bootfiles par architecture ni l'option vendor du Pi — sur ces réseaux, fais tourner le proxyDHCP en parallèle. Voir le [guide DHCP](dhcp.md).

## Overrides par machine

Pour donner à une carte spécifique des fichiers firmware différents ou un `config.txt` personnalisé, place des fichiers dans le répertoire de données sous le numéro de série de la carte :

```
<data>/raspberrypi/<serial>/config.txt      # this Pi only
<data>/raspberrypi/config.txt               # all Pis, overrides embedded copy
```

Tout ce qui n'est pas surchargé retombe sur les assets embarqués. Le numéro de série est la valeur à 8 chiffres hexa donnée par `cat /proc/cpuinfo` sur le Pi, et il apparaît dans les logs TFTP de Bootimus quand la carte essaie de netbooter pour la première fois.

## Limitations

- **Ethernet filaire uniquement** — le firmware du Pi ne peut pas netbooter en WiFi.
- **Les réglages UEFI ne persistent pas.** Le firmware netbooté n'a pas de variable store inscriptible, donc il boote toujours avec les défauts pftf — y compris la limite par défaut de 3 Go de RAM sur Pi 4. Si ton OS a besoin de toute la RAM, construis un `RPI_EFI.fd` avec la limite désactivée et dépose-le comme override.
- **Le Pi 5 n'est pas supporté** tant que son port EDK2 n'aura pas mûri ; pareil pour toute carte sans firmware UEFI.
- Les assets sont rafraîchis avec `scripts/build-raspberrypi-assets.sh`, qui épingle et vérifie par checksum les releases UEFI [pftf](https://github.com/pftf/RPi4). Les blobs de boot Broadcom qu'il embarque ne sont licenciés que pour un usage avec des appareils Raspberry Pi (`LICENCE.broadcom.txt` est livré à côté).
