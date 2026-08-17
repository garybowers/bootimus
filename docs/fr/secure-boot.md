# UEFI Secure Boot

Comment netbooter des machines avec UEFI Secure Boot activé, sans toucher aux réglages de leur firmware.

## Table des matières

- [Fonctionnement](#fonctionnement)
- [Activer le support Secure Boot](#activer-le-support-secure-boot)
- [Booter des images](#booter-des-images)
- [Ce qui marche et ce qui ne marche pas](#ce-qui-marche-et-ce-qui-ne-marche-pas)
- [Avancé : bootloaders auto-signés pour NBD/NFS](#avancé--bootloaders-auto-signés-pour-nbdnfs)
- [Dépannage](#dépannage)

## Fonctionnement

Bootimus livre un set de bootloaders intégré appelé `secureboot-official`. Il ne contient que des binaires de release officiellement signés, donc chaque maillon de la chaîne de boot est vérifié avec des clés auxquelles un firmware d'usine fait déjà confiance — pas d'enrôlement de certificat, aucun changement de firmware sur les clients :

```
Firmware (Secure Boot on, stock Microsoft keys)
  → ipxe-shimx64.efi     Microsoft-signed shim from the iPXE release
    → ipxe.efi           signed by the iPXE project's Secure Boot CA,
                         which the shim carries as its vendor certificate
      → autoexec.ipxe    fetched over TFTP; Bootimus generates it per client
        → boot menu      as normal
```

Quand tu bootes une image extraite, l'entrée de menu générée charge le kernel et l'initrd, puis chain-load le **shim signé Microsoft de la distro elle-même** depuis l'ISO extrait, via la commande `shim` d'iPXE. Ce shim vérifie la signature du kernel de la distro, ce qui complète la chaîne :

```ipxe
kernel http://server:8080/boot/ubuntu-26.04/vmlinuz ...
initrd http://server:8080/boot/ubuntu-26.04/initrd
iseq ${platform} efi && shim http://server:8080/boot/ubuntu-26.04/iso/EFI/boot/bootx64.efi ||
boot
```

Bootimus détecte le shim automatiquement pendant l'extraction de l'ISO. La garde `iseq … ||` rend la ligne sans effet sur les clients BIOS et sur les builds iPXE sans la commande `shim`, et iPXE n'invoque un shim chargé que quand Secure Boot est réellement appliqué — les machines avec Secure Boot désactivé bootent exactement comme avant.

## Activer le support Secure Boot

1. Ouvre **Bootloaders** dans l'UI admin et sélectionne **`secureboot-official`** comme set actif (ou définis-le par client dans le formulaire d'édition du client).
2. Si tu utilises le proxyDHCP intégré, c'est tout — il annonce automatiquement `ipxe-shimx64.efi` aux clients UEFI.
3. Si tu fais tourner un serveur DHCP externe, change son nom de bootfile UEFI en `ipxe-shimx64.efi` (et `ipxe-shimaa64.efi` pour les clients ARM64). Voir le [guide DHCP](dhcp.md).

Les binaires iPXE officiels n'embarquent aucun script Bootimus ; ils s'appuient sur iPXE ≥ 2.0 qui récupère `autoexec.ipxe` via TFTP, servi par Bootimus. Garde le port UDP 69 accessible depuis les clients.

## Booter des images

Pour les clients Secure Boot, utilise le **boot extrait (kernel/initrd)** pour tes images — upload l'ISO et clique **Extract** dans la liste des images. Pendant l'extraction, Bootimus enregistre le shim `EFI/BOOT/BOOTX64.EFI` (ou `BOOTAA64.EFI`) de l'ISO et le branche automatiquement dans le menu.

Les images extraites avant l'ajout du support Secure Boot ont besoin d'une ré-extraction pour récupérer leur shim : supprime le dossier de boot de l'image et extrais à nouveau.

Les installs Windows bootent via le `wimboot` officiel signé Microsoft, inclus dans le set `secureboot-official` ; tout ce que wimboot charge ensuite depuis `boot.wim` est signé Microsoft.

## Ce qui marche et ce qui ne marche pas

| Scénario | Sous Secure Boot |
|----------|------------------|
| ISOs Linux extraits de distros qui supportent Secure Boot (Ubuntu, Debian, Fedora, openSUSE, Rocky, Alma, …) | ✅ Marche |
| Installeur Windows (wimboot) | ✅ Marche |
| Distros qui ne supportent pas Secure Boot sur leurs propres médias (Arch, Alpine, Void, …) | ❌ Leurs kernels ne sont pas signés — pareil qu'en bootant leur clé USB. Désactive Secure Boot pour celles-là. |
| Boot `sanboot` (ISO entier) | ❌ Utilise le boot extrait à la place |
| Méthodes de boot NBD / NFS | ❌ Elles bootent le kernel non signé de Bootimus — voir [plus bas](#avancé--bootloaders-auto-signés-pour-nbdnfs) |
| Images d'outils (ShredOS, memtest, …) | ❌ Kernels non signés |

## Avancé : bootloaders auto-signés pour NBD/NFS

Les méthodes de boot NBD et NFS chargent le kernel de l'environnement de boot de Bootimus, qu'aucune CA publique ne signera. Si tu en as besoin sous Secure Boot, construis un set auto-signé et enrôle ton certificat une fois sur chaque client :

```bash
scripts/generate-signing-key.sh
BOOTIMUS_SIGNING_KEY=… BOOTIMUS_SIGNING_CERT=… scripts/build-secureboot-set.sh
```

Copie la sortie vers `<data>/bootloaders/secureboot/`, sélectionne le set dans l'UI, puis enrôle le certificat sur chaque client avec `mokutil --import bootimus-signing.crt` (ou via la gestion des clés du firmware). Pour la plupart des déploiements, désactiver Secure Boot sur les machines concernées est la réponse la plus simple.

## Dépannage

- **Écran « Security Violation » du firmware avant le chargement d'iPXE** — le client ne récupère pas `ipxe-shimx64.efi`. Vérifie le nom de bootfile de ton DHCP et que le set `secureboot-official` est actif.
- **iPXE charge mais aucun menu n'apparaît** — les binaires officiels ont besoin d'`autoexec.ipxe` via TFTP. Vérifie l'UDP 69 entre le client et le serveur.
- **Une ligne « Security Violation » dans le log d'iPXE pendant le boot d'une image** — attendu et sans danger ; c'est iPXE qui sonde l'image shim, pas un échec.
- **Touches du menu qui ne répondent pas sur certains matériels** — iPXE v2.0.0 a une régression clavier connue dans les menus interactifs sur certaines machines. Définis une image par défaut ou utilise la sélection next-boot comme contournement, ou bascule ce client sur le set `default` s'il n'a pas besoin de Secure Boot.
- **Le kernel refuse de charger sous Secure Boot** — l'image a probablement été extraite avant que le support du shim existe (ré-extrais-la), ou la distro ne supporte pas du tout Secure Boot.
