#  Guide de gestion des images

Guide complet pour gérer les images ISO, extraire les fichiers de boot, et gérer les cas spéciaux comme le netboot Debian/Ubuntu.

##  Table des matières

- [Ajouter des images](#ajouter-des-images)
- [Sommes de contrôle et notifications de mise à jour](#sommes-de-contrôle-et-notifications-de-mise-à-jour)
- [Images noyau unifiées (UKI)](#images-noyau-unifiées-uki)
- [OpenBSD et NetBSD](#openbsd-et-netbsd)
- [WinPE et médias de secours](#winpe-et-médias-de-secours)
- [Extraction du kernel](#extraction-du-kernel)
- [Support netboot](#support-netboot)
- [Optimisation Ubuntu Desktop](#optimisation-ubuntu-desktop)
- [Distributions supportées](#distributions-supportées)
- [Dépannage](#dépannage)

## Ajouter des images

### Upload via l'interface web

1. Va sur le panneau admin : `http://your-server:8081`
2. Clique sur le bouton **« Upload ISO »**
3. Drag-and-drop le fichier ISO ou clique pour parcourir
4. Optionnellement, ajoute une description
5. Coche **« Public »** pour le rendre accessible à tous les clients
6. Clique sur **« Upload »**

**Limites d'upload** : 10 Go par fichier

### Upload via l'API

```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Télécharger depuis une URL

Télécharge les ISOs directement sur le serveur sans upload local :

**Via l'interface web** :
1. Clique sur le bouton **« Download from URL »**
2. Entre l'URL de téléchargement de l'ISO
3. Ajoute une description
4. Clique sur **« Download »**

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'
```

**Suivre la progression** :
```bash
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Sommes de contrôle et notifications de mise à jour

**Vérification au téléchargement.** Quand un ISO est récupéré via **Get Images** (le catalogue intégré), Bootimus cherche un fichier de sommes de contrôle publié à côté (`SHA256SUMS`, `sha256sum.txt` et similaires) et hache le téléchargement au fil de l'eau. Une différence fait échouer le téléchargement et supprime le fichier, de sorte qu'une image tronquée ou altérée n'atteint jamais le menu de boot. Les téléchargements depuis une simple URL peuvent aussi être vérifiés en passant un `sha256` ou une `checksum_url` à `/api/images/download` :

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{"url": "https://releases.ubuntu.com/24.04.4/ubuntu-24.04.4-live-server-amd64.iso",
       "checksum_url": "https://releases.ubuntu.com/24.04.4/SHA256SUMS"}'
```

Les images vérifiées affichent un badge **✓ Verified** dans la liste des images et le SHA-256 enregistré dans le panneau des propriétés de l'image. Les images téléchargées sans somme de contrôle publiée, ou uploadées à la main, sont marquées **Unverified** ; colle un SHA-256 ou l'URL d'un fichier de sommes de contrôle dans le panneau des propriétés et clique sur **Verify** pour hacher le fichier en arrière-plan (`POST /api/images/verify?filename=…`, interroge la même URL en `GET`).

**Nouvelles versions.** Une fois par jour (et via **Settings → Housekeeping → Check now**, ou `POST /api/catalogue/check`) Bootimus compare les images installées avec le catalogue. Une image téléchargée depuis le catalogue est reconnue par sa distribution et sa ligne de version, donc `ubuntu-24.04.3-desktop-amd64.iso` est signalée quand `24.04.4` apparaît ; les images ajoutées avant cette fonctionnalité sont reconnues d'après la forme de leur nom de fichier. Les images signalées portent un badge **Update available** et un bouton **Download newer version** dans le panneau des propriétés, les outils affichent le même badge avec **Download newer**, et le tableau de bord compte les mises à jour en attente. La vérification utilise le catalogue en ligne quand les mises à jour de profils distantes sont autorisées et se rabat sur le catalogue embarqué dans le binaire quand `--disable-remote-profiles` est défini. Décoche **Check the catalogue daily** sous Housekeeping pour la désactiver.

### Organiser avec des dossiers

Les ISOs placés dans des sous-répertoires sont automatiquement regroupés dans le menu de boot :

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

Les groupes sont créés automatiquement au démarrage et lors du scan. Ils peuvent aussi être gérés manuellement via l'onglet Groups dans l'UI admin.

### Scanner les ISOs existants

Si tu copies manuellement des ISOs dans le répertoire data (y compris dans des sous-répertoires) :

1. Copie les fichiers ISO dans le répertoire `/data/isos/` (ou ses sous-répertoires)
2. Clique sur le bouton **« Scan for ISOs »** dans le panneau admin
3. Bootimus détecte et enregistre les nouveaux ISOs et crée des groupes à partir des dossiers

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Images noyau unifiées (UKI)

Une image noyau unifiée (UKI) est un exécutable EFI unique qui regroupe un noyau Linux, son initrd et sa ligne de commande noyau. Bootimus démarre les UKI directement, sans extraction.

**Ajouter une UKI** : téléverse le fichier `.efi`, télécharge-le depuis une URL ou copie-le dans `/data/isos/` puis lance un scan, exactement comme pour une ISO. Bootimus n'enregistre que les fichiers `.efi` qui sont des UKI (exécutables EFI avec une section `.linux`) ; les autres programmes EFI sont refusés et les fichiers situés dans le dossier extrait d'une ISO sont ignorés.

**Paramètres de boot** : la ligne de commande intégrée dans la section `.cmdline` de l'UKI est copiée dans les paramètres de boot de l'image lors de son premier ajout, pour que tu puisses la voir et la modifier dans les propriétés de l'image. Bootimus démarre l'image avec :

```
kernel http://<server>:8080/isos/<image>.efi <boot parameters>
```

Avec Secure Boot désactivé, ces paramètres remplacent la ligne de commande intégrée, c'est pourquoi celle-ci est pré-remplie. Avec Secure Boot activé, le systemd-stub de la plupart des UKI les ignore et utilise toujours la ligne de commande intégrée.

**UEFI uniquement** : les UKI sont des programmes EFI, les clients en BIOS legacy ne peuvent donc pas les démarrer. En choisir une sur un client BIOS affiche un message indiquant qu'UEFI est nécessaire.

## OpenBSD et NetBSD

OpenBSD et NetBSD ne bootent pas depuis un kernel et un initrd à la manière de Linux. Chacun embarque son propre chargeur réseau de second niveau, et ce chargeur ne parle que TFTP : il récupère son fichier de configuration et le kernel d'installation à la racine du serveur TFTP. Bootimus supporte les deux en extrayant ces fichiers de l'ISO d'installation et en les servant via son serveur TFTP ; le port TFTP doit donc être joignable depuis les clients (il l'est déjà pour le PXE lui-même).

**Ajouter une image** : uploade `install77.iso` (ou `cd77.iso`) pour OpenBSD, ou `NetBSD-10.1-amd64.iso` pour NetBSD, puis lance **Extract Kernel/Initrd** comme pour n'importe quel autre ISO. L'extraction copie les chargeurs et le kernel d'installation dans un dossier `bsd/` à côté de l'ISO et extrait l'arborescence complète de l'ISO pour que l'installeur puisse récupérer ses sets depuis `http://<server>:8080/boot/<image>/iso/`.

| Distribution | Chargeurs pris dans l'ISO | Kernel |
|--------------|---------------------------|--------|
| OpenBSD | `pxeboot` (BIOS), `BOOTX64.EFI`, `BOOTIA32.EFI`, `BOOTAA64.EFI` (UEFI) depuis `/<version>/<arch>/` | `bsd.rd` |
| NetBSD | `pxeboot_ia32.bin` (BIOS) depuis `/<arch>/installation/misc/`, `bootx64.efi` (UEFI) quand l'ISO en contient un | `netbsd-INSTALL.gz` |

**Fonctionnement du boot** : l'entrée de menu chaîne vers un petit script iPXE sur le port HTTP, qui note que ce client (par adresse IP) a choisi cette image, puis chaîne le chargeur correspondant en TFTP depuis `bsd/<image>/`. Pendant les 15 minutes suivantes, les requêtes de cette IP pour les fichiers que les chargeurs cherchent à la racine TFTP (`etc/boot.conf`, `etc/random.seed`, `bsd.rd`, `bsd` pour OpenBSD ; `boot.cfg`, `EFI/NetBSD/boot.cfg`, `netbsd` et `netbsd-INSTALL.gz` pour NetBSD) sont servies depuis cette image. Le fichier de configuration est généré à la volée et pointe le chargeur vers le kernel sous `bsd/<image>/`, donc les chemins explicites fonctionnent aussi. Si l'ISO n'a pas le chargeur pour le firmware du client, celui-ci voit un message l'indiquant et revient au menu.

**Console série et autres réglages du chargeur** : place des lignes de configuration supplémentaires du chargeur dans les **Boot Parameters** de l'image, séparées par des points-virgules. Pour OpenBSD, elles sont ajoutées au `boot.conf` généré, par exemple `stty com0 115200;set tty com0`. Pour NetBSD, elles sont ajoutées au `boot.cfg` généré, par exemple `consdev=com0`.

**Limitations connues** :

- Ce support est récent et n'a pas encore été vérifié sur du matériel réel. Merci de signaler ce qui fonctionne et ce qui ne fonctionne pas.
- Le `pxeboot_ia32.bin` de NetBSD effectue son propre échange DHCP une fois chargé et lit le champ DHCP `filename` pour choisir entre NFS et TFTP. Sans préfixe `tftp:` à cet endroit, il utilise NFS par défaut, ce que Bootimus ne fournit pas pour cet usage. Si le chargeur indique qu'il ne peut pas ouvrir `boot.cfg` ou le kernel, tape `boot tftp:bsd/<image>/netbsd-INSTALL.gz` à son invite, ou donne aux clients NetBSD `filename "tftp:boot.cfg";` sur ton serveur DHCP. Les chargeurs d'OpenBSD utilisent toujours TFTP et n'ont pas besoin de cette étape.
- L'association par client est indexée par adresse IP ; deux clients derrière la même adresse NAT qui bootent des images BSD différentes à moins de 15 minutes d'écart recevraient donc la même configuration.
- L'autoinstall des installeurs BSD n'est pas encore câblé. Les installeurs peuvent quand même récupérer leurs sets depuis l'arborescence ISO extraite en HTTP.

## WinPE et médias de secours

Bootimus démarre les installeurs Windows via [wimboot](https://ipxe.org/wimboot), et le même chemin couvre désormais les médias de secours basés sur WinPE tels que le WinPE de Sergei Strelec, Active@ Boot Disk et Hiren's BootCD PE.

**Dispositions supportées** : tout ISO qui contient un `BCD` (sous `boot/` ou `efi/microsoft/boot/`), `boot/boot.sdi` et au moins un fichier `.wim`. Les médias d'installation ont aussi `sources/install.wim` ou `install.esd` ; un média qui n'en a pas est classé dans le profil **WinPE / Rescue Media** (`winpe`) au lieu de **Windows**, donc aucun traitement d'auto-installation ou SMB ne lui est appliqué. Les fichiers WIM situés dans les quatre premiers niveaux de répertoires sont trouvés, donc les `SSTR/*.wim` de Strelec et les dispositions similaires fonctionnent sans rien renommer.

**Choisir le WIM** : après extraction, le panneau des propriétés de l'image liste chaque WIM trouvé avec sa taille. Choisis celui que wimboot doit charger et, quand un WIM contient plusieurs images, l'index d'image. L'index n'est ajouté à la ligne wimboot que s'il est supérieur à un, et un `index=` explicite dans les paramètres de boot est prioritaire. La même chose peut se régler via l'API :

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"wim_path":"SSTR/strelec10x64Eng.wim","wim_index":1}' \
  "http://localhost:8081/api/images/winpe?filename=WinPE_Strelec.iso"
```

**Fonctionnement du boot** : seul le WIM sélectionné est envoyé au client ; wimboot synthétise lui-même le BCD et `boot.sdi`, ce qui évite l'écran noir que certains médias produisent quand le BCD du DVD est utilisé. Les paramètres de boot sont limités aux options propres à wimboot (`rawbcd`, `rawwim`, `gui`, `linear`, `quiet`, `pause`, `index`, `initrdfile`), exactement comme pour les installeurs Windows.

**Limitations** :

- Les programmes que l'environnement de secours s'attend à trouver sur son propre média de boot (par exemple le dossier `SSTR` d'outils portables de Strelec) ne sont pas dans le WIM, ils manquent donc après un boot PXE à moins que l'environnement ne puisse les récupérer lui-même par le réseau. Le bureau WinPE de base et les outils intégrés au WIM fonctionnent.
- Les médias qui ont besoin de leurs propres entrées BCD (menus `bootmgr` personnalisés, menus de sélection multi-WIM) démarrent directement dans le WIM choisi au lieu d'afficher ce menu.
- Les fichiers `.wim` seuls ne peuvent pas encore être uploadés tels quels ; place-les sur un ISO avec un `BCD` et `boot.sdi`, ou utilise un ISO de secours complet.

## Extraction du kernel

La plupart des ISOs modernes supportent le boot HTTP direct via la commande `sanboot` d'iPXE, qui télécharge et boote l'ISO entier. Cependant, extraire le kernel et l'initrd apporte des bénéfices significatifs :

###  Avantages de l'extraction du kernel

- **Temps de boot plus rapides** : ne télécharge que kernel/initrd (~100 Mo) au lieu de l'ISO entier (1-10 Go)
- **Bande passante réduite** : critique pour les réseaux avec plusieurs clients
- **Meilleure compatibilité** : certains ISOs ne supportent pas correctement `sanboot`
- **Installation réseau** : utilise les fichiers netboot pour les installeurs Debian/Ubuntu
- **UEFI Secure Boot** : l'extraction détecte le shim signé de l'ISO, ce qui permet aux clients Secure Boot de vérifier le kernel — voir le [guide Secure Boot](secure-boot.md)

### Comment extraire

**Via l'interface web** :
1. Va dans l'onglet **Images**
2. Trouve ton image ISO
3. Clique sur le bouton **« Extract »**
4. Attends que l'extraction se termine

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04-live-server-amd64.iso"}'
```

### Extraction manuelle

Si l'extracteur intégré ne supporte pas ton ISO, tu peux extraire les fichiers de boot manuellement et bootimus les détectera automatiquement.

1. Crée un répertoire avec le même nom que l'ISO (sans l'extension `.iso`) :
   ```bash
   mkdir -p data/isos/my-custom-distro/
   ```

2. Place le kernel et l'initrd dans ce répertoire avec ces noms exacts :
   ```
   data/isos/
   ├── my-custom-distro.iso
   └── my-custom-distro/
       ├── vmlinuz          # kernel
       └── initrd           # initrd/initramfs
   ```

3. Clique sur **« Scan for ISOs »** dans le panneau admin (ou redémarre bootimus). L'image sera automatiquement détectée comme extraite et passée en méthode de boot kernel.

Ça marche aussi pour les ISOs dans les sous-répertoires :
```
data/isos/linux/my-custom-distro.iso
data/isos/linux/my-custom-distro/vmlinuz
data/isos/linux/my-custom-distro/initrd
```

### Ce qui est extrait

Bootimus détecte automatiquement la distribution et extrait :

- **Kernel** : `vmlinuz` (ou `linux`, `bzImage`)
- **Initrd** : `initrd`, `initrd.gz`, `initrd.lz`
- **Squashfs** (Ubuntu/Debian live) : `filesystem.squashfs`
- **Métadonnées de distribution** : type d'OS, paramètres de boot

**Emplacement des fichiers extraits** :
```
/data/isos/
├── ubuntu-24.04.iso                    # Original ISO
└── ubuntu-24.04/                       # Extracted directory
    ├── vmlinuz                         # Kernel
    ├── initrd                          # Initrd
    └── casper/
        └── filesystem.squashfs         # Squashfs filesystem
```

### Sélection automatique de la méthode de boot

Après l'extraction, Bootimus sélectionne automatiquement la méthode de boot optimale :

| Distribution | Méthode de boot | Téléchargements |
|--------------|-------------|-----------|
| Ubuntu Desktop (extrait) | `fetch=` | ~2.8 Go (squashfs uniquement) |
| Ubuntu Desktop (non extrait) | `url=` | ~18 Go (ISO × 3) |
| Ubuntu Server (netboot) | Netboot | ~50 Mo (fichiers netboot) |
| Debian Installer (netboot) | Netboot | ~30 Mo (fichiers netboot) |
| Arch Linux | HTTP boot | ~100 Mo (kernel/initrd) |
| Fedora/RHEL | HTTP boot | ~150 Mo (kernel/initrd + stage2) |

## Support netboot

Certains ISOs d'installation (Debian, Ubuntu Server) ne contiennent pas un OS complet — ils sont conçus pour télécharger des paquets pendant l'installation. Pour ceux-là, Bootimus supporte le téléchargement des fichiers netboot officiels.

###  Détection du besoin de netboot

Quand tu extrais un ISO d'installeur Debian ou Ubuntu Server, Bootimus détecte qu'il nécessite le netboot :

**Indicateurs** :
- L'ISO contient un répertoire `/install/` (pas `/casper/`)
- Type installeur (pas live/desktop)
- Petite taille d'ISO (< 1 Go)

**Le panneau admin affiche** :
-  badge « Netboot Required »
- 📥 bouton « Download Netboot »

### Télécharger les fichiers netboot

**Via l'interface web** :
1. Va dans l'onglet **Images**
2. Trouve l'ISO d'installeur avec le badge « Netboot Required »
3. Clique sur le bouton **« Download Netboot »**
4. Attends le téléchargement et l'extraction

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

### Qu'est-ce que les fichiers netboot ?

Les fichiers netboot sont des fichiers de boot minimaux officiels fournis par les distributions :

**Netboot Debian** :
- Source : `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`
- Taille : ~30 Mo
- Contient : `vmlinuz`, `initrd.gz`, fichiers d'installeur

**Netboot Ubuntu** :
- Source : `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`
- Taille : ~50 Mo
- Contient : `vmlinuz`, `initrd.gz`, fichiers d'installeur

### Fonctionnement du netboot

1. **Le client boote** : télécharge kernel/initrd netboot (~50 Mo)
2. **L'installeur démarre** : l'initrd netboot lance l'installeur réseau
3. **Téléchargement des paquets** : l'installeur télécharge les paquets depuis les mirroirs Ubuntu/Debian
4. **Installation** : OS installé directement depuis les dépôts internet

**Avantages** :
-  Toujours les derniers paquets (pas les paquets périmés de l'ISO)
-  Bande passante minimale vers le serveur PXE (pas de téléchargement d'ISO)
-  Besoins de stockage plus petits
-  Fichiers de boot officiels et signés

### Netboot installeur Debian

**ISOs supportés** :
- `debian-*-netinst.iso` — installeur réseau
- Petits ISOs d'installeur Debian avec répertoire `/install/`

**Détection** :
```
ISO structure:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**URL netboot** : `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`

**Paramètres de boot** : `priority=critical ip=dhcp`

### Netboot Ubuntu Server

**ISOs supportés** :
- `ubuntu-*-live-server-*.iso` — installeur live server avec répertoire `/install/`
- Anciens installeurs Ubuntu server

**Détection** :
```
ISO structure:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**URL netboot** : `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`

**Paramètres de boot** : `ip=dhcp`

###  Important : Ubuntu Desktop vs Server

Il y a **deux types** d'ISOs Ubuntu avec des méthodes de boot différentes :

| Type | Pattern de nom d'ISO | Répertoire | Méthode de boot | Netboot ? |
|------|------------------|-----------|-------------|----------|
| **Desktop/Live** | `ubuntu-*-desktop-*.iso` | `/casper/` | `fetch=` ou `url=` |  Non |
| **Server Installer** | `ubuntu-*-live-server-*.iso` (avec `/install/`) | `/install/` | Netboot |  Oui |

**Ubuntu Desktop** (`/casper/`) :
- Contient un OS live complet
- Utilise le boot casper avec `fetch=` ou `url=`
- Extraire le kernel pour utiliser `fetch=` (télécharge uniquement le squashfs)
- Pas de support netboot

**Ubuntu Server Installer** (`/install/`) :
- Installeur réseau minimal
- Nécessite les fichiers netboot
- Télécharge les paquets pendant l'installation
- Beaucoup plus efficace

## Optimisation Ubuntu Desktop

Les ISOs Ubuntu Desktop utilisent le système de live boot casper. Sans optimisation, ils téléchargent l'ISO entier **trois fois** (~18 Go pour un ISO de 6 Go).

###  Problème : triple téléchargement d'ISO

**Comportement par défaut** (sans extraction) :
```
Boot parameter: url=http://server/ubuntu.iso

Result:
- Download 1: Kernel verifies ISO (6GB)
- Download 2: Initrd verifies ISO (6GB)
- Download 3: Casper mounts ISO (6GB)
Total: ~18GB downloaded
```

###  Solution 1 : extraire et utiliser le paramètre `fetch=`

**Après extraction** :
```
Boot parameter: fetch=http://server/ubuntu/casper/filesystem.squashfs

Result:
- Download: Only squashfs (~2.8GB)
Total: ~2.8GB downloaded
```

**Comment activer** :
1. Extrais kernel/initrd de l'ISO
2. Bootimus utilise automatiquement le paramètre `fetch=`
3. Seul le squashfs est téléchargé (pas l'ISO entier)

**Économies** : réduction de 85 % (18 Go → 2.8 Go)

###  Solution 2 : utiliser le netboot Ubuntu Server à la place

Pour les déploiements serveur, utilise l'installeur Ubuntu Server avec le netboot :

**Approche netboot** :
```
1. Upload ubuntu-server.iso
2. Extract kernel/initrd
3. Download netboot files
4. Boot with netboot (~50MB download)
5. Install from Ubuntu repositories
```

**Économies** : réduction de 99 % (18 Go → 50 Mo)

### Référence des paramètres de boot

**Ubuntu Desktop (casper)** :
```bash
# Default (no extraction) - downloads ISO 3 times
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp url=http://server/ubuntu.iso

# Optimised (with extraction) - downloads squashfs once
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp fetch=http://server/ubuntu/casper/filesystem.squashfs
```

**Ubuntu Server (netboot)** :
```bash
# Netboot - minimal download
ip=dhcp
```

## Distributions supportées

### Entièrement testées

| Distribution | Extraction kernel | Netboot | Notes |
|--------------|-------------------|---------|-------|
| **Arch Linux** |  Oui |  N/A | `/arch/boot/x86_64/vmlinuz-linux` |
| **Fedora Workstation** |  Oui |  N/A | `/isolinux/vmlinuz` |
| **Rocky Linux** |  Oui |  N/A | `/isolinux/vmlinuz` |
| **Debian (installeur)** |  Oui |  Oui | `/install/vmlinuz` + netboot |
| **Debian Live** |  Oui |  Non | `/live/vmlinuz` |
| **Ubuntu Desktop** |  Oui |  Non | `/casper/vmlinuz` + optimisation fetch |
| **Ubuntu Server** |  Oui |  Oui | `/install/vmlinuz` + netboot |
| **Pop!_OS** |  Oui |  Non | `/casper/vmlinuz` |
| **TrueNAS SCALE** |  Oui |  Non | 25.10.2.1 : installation et démarrage sur disque dans QEMU UEFI ; ISO complète téléchargée |
| **Proxmox VE** |  Oui |  Non | `/boot/linux26` + `/boot/initrd.img` |
| **openSUSE** |  Oui |  N/A | `/boot/x86_64/loader/linux` |
| **NixOS** |  N/A |  N/A | Sanboot |

### Patterns de détection

Bootimus détecte les distributions en scannant des patterns de fichiers spécifiques :

**Arch Linux** :
```
/arch/boot/x86_64/vmlinuz-linux
/arch/boot/x86_64/initramfs-linux.img
```

**Fedora/RHEL/Rocky** :
```
/isolinux/vmlinuz
/isolinux/initrd.img
```

**Ubuntu Desktop (casper)** :
```
/casper/vmlinuz or /casper/vmlinuz.efi
/casper/initrd or /casper/initrd.gz or /casper/initrd.lz
/casper/filesystem.squashfs
```

**Ubuntu Server Installer** :
```
/install/vmlinuz or /install.amd/vmlinuz
/install/initrd.gz or /install.amd/initrd.gz
```

**Debian Installer** :
```
/install/vmlinuz or /install.amd/vmlinuz
/install/initrd.gz or /install.amd/initrd.gz
```

**TrueNAS SCALE** :
```
/vmlinuz
/initrd.img
/live/filesystem.squashfs
/TrueNAS-SCALE.update
```

Bootimus reconnaît SCALE grâce au fichier `TrueNAS-SCALE.update` et sélectionne un profil dédié. Extraire le noyau/initrd comme d’habitude : le profil télécharge l’ISO complète et la monte sur `/cdrom` pour le programme d’installation. L’ISO reste intacte. Le programme d’installation doit inclure systemd 254 ou ultérieur.

Vérifié avec SCALE 25.10.2.1 dans QEMU en UEFI avec 8 Gio de RAM : installation sur un nouveau disque virtuel, puis démarrage depuis ce disque. L’ISO complète reste en RAM pendant l’installation. Les autres versions, le matériel physique et Secure Boot ne sont pas testés. Ce profil ne couvre pas TrueNAS CORE, basé sur FreeBSD.

Extraire à nouveau les images déjà extraites pour remplacer les anciens paramètres Debian, puis vérifier les paramètres de démarrage personnalisés.

**Proxmox VE** :
```
/boot/linux26
/boot/initrd.img
```

## Dépannage

### Échec de l'extraction

**Symptômes** : erreur « Extraction failed » dans le panneau admin

**Causes courantes** :
1. **ISO corrompu** : re-télécharge l'ISO
2. **ISO non supporté** : vérifie si la distribution est supportée
3. **Espace disque** : assure-toi d'avoir assez d'espace pour l'extraction
4. **Permissions** : vérifie les permissions de fichier sur le répertoire data

**Debugging** :
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

### Échec du téléchargement netboot

**Symptômes** : erreur « Netboot download failed »

**Causes courantes** :
1. **Connectivité réseau** : impossible de joindre les mirroirs Debian/Ubuntu
2. **URL modifiée** : l'URL du mirroir a pu être mise à jour
3. **Échec de l'extraction du tarball** : téléchargement corrompu

**Solutions** :
```bash
# Test mirror connectivity
curl -I http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz

# Check server logs
docker logs bootimus | grep -i netboot

# Manually verify netboot URL
wget http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz
tar -tzf netboot.tar.gz | grep vmlinuz
```

### Le menu de boot affiche le mauvais type d'image

**Symptômes** : l'image affiche un badge « [kernel] » mais ne boote pas avec la méthode kernel

**Cause** : base de données et filesystem désynchronisés

**Solution** :
```bash
# Re-extract kernel/initrd
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'

# Or re-scan ISOs
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### Le client télécharge l'ISO plusieurs fois

**Symptômes** : l'ISO Ubuntu Desktop est téléchargé 3 fois

**Cause** : utilisation du paramètre `url=` sans extraction

**Solution** :
1. Extrais kernel/initrd de l'ISO
2. Bootimus utilisera automatiquement le paramètre `fetch=`
3. Seul le squashfs est téléchargé (pas l'ISO entier)

**Vérifier** :
```bash
# Check if extracted
ls -la /data/isos/ubuntu-24.04/casper/filesystem.squashfs

# Check server logs during boot
docker logs -f bootimus
# Look for: "fetch=..." instead of "url=..."
```

### Netboot requis mais pas de bouton de téléchargement

**Symptômes** : l'image affiche « Netboot Required » mais pas de bouton de téléchargement

**Cause** : URL netboot non configurée ou détection échouée

**Solution** :
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

## Étapes suivantes

-  Voir le [Guide de la console d'administration](admin.md) pour gérer les images
-  Lis le [Guide de déploiement](deployment.md) pour la configuration du stockage
-  Configure le [Serveur DHCP](dhcp.md) pour le boot PXE
-  Configure la [Gestion des clients](clients.md) pour le contrôle d'accès
