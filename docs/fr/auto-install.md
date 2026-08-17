# Guide d'installation automatique

Lance des installations sans surveillance sur Windows, Ubuntu, Debian et les distros de la famille Red Hat. Dépose un fichier de config une fois, attache-le à une image, et chaque boot PXE termine l'installation sans une seule frappe au clavier.

## Table des matières

- [Vue d'ensemble](#vue-densemble)
- [Formats supportés](#formats-supportés)
- [La bibliothèque de fichiers](#la-bibliothèque-de-fichiers)
- [Attacher des fichiers](#attacher-des-fichiers)
- [Ordre de résolution](#ordre-de-résolution)
- [Placeholders](#placeholders)
- [Exemples](#exemples)
- [Notes Windows](#notes-windows)
- [API REST](#api-rest)
- [Dépannage](#dépannage)

## Vue d'ensemble

Les configs d'auto-installation sont stockées dans une bibliothèque par distro sous `data/autoinstall/`. Tu peux :

- **Gérer les fichiers dans l'UI** sous l'onglet **Auto-Install** — créer, éditer, uploader, télécharger et supprimer des configs sans toucher au filesystem.
- **Définir un défaut** par image (chaque client bootant cette image obtient la même config).
- **Surcharger par client** ou **par groupe de clients** quand une machine a besoin d'une config différente (hostname différent, layout disque, rôle, etc.).
- **Templatiser** avec des placeholders comme `{{HOSTNAME}}` et `{{IP}}` résolus au moment du service en utilisant l'identité du client qui boote.

Bootimus sert le script en HTTP sur l'endpoint auto-install, et — pour Windows — stage `AutoUnattend.xml` sur le partage SMB d'installation pour que `setup.exe /unattend:` le récupère automatiquement.

## Formats supportés

| Famille de distro | Format | Extension de fichier | Détecté comme |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| Tout le reste | brut | n'importe quelle | `generic` |

L'extension détermine à la fois le label affiché dans l'UI et l'en-tête `Content-Type` quand le fichier est servi.

## La bibliothèque de fichiers

Tous les fichiers d'auto-install vivent sous `data/autoinstall/<distro>/<filename>` :

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

Le segment `<distro>` doit correspondre à un ID de profil de distro connu (voir [Profils de distro](distro-profiles.md)). Le répertoire est créé automatiquement au premier démarrage.

### Ajouter des fichiers via l'UI

**Auto-Install** → **New File** ouvre l'éditeur avec un sélecteur de distro, un champ nom de fichier et un textarea syntax-friendly. **Upload File** prend n'importe quel fichier local et le dépose dans le dossier distro choisi.

### Ajouter des fichiers manuellement

Dépose-les juste :

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

Ils apparaissent immédiatement dans l'UI — pas de redémarrage, pas de scan.

## Attacher des fichiers

Les fichiers d'auto-install n'ont aucun effet tant que tu ne les as pas branchés. Il y a trois endroits où en attacher un :

### Image (par défaut)

Onglet **Images** → ouvre les **Properties** d'une image → section **Auto-Install** → choisis un fichier. Chaque client qui boote cette image récupère cette config sauf si quelque chose de plus spécifique la surcharge.

### Client (override par machine)

Onglet **Clients** → ouvre un client → menu déroulant **Auto-Install File**. Utilise ça quand une machine spécifique a besoin d'une config différente (par ex. un serveur de build vs le reste de la flotte de bureau).

### Groupe de clients (override par flotte)

Onglet **Groups** → ouvre un groupe → **Auto-Install File**. S'applique à chaque client du groupe. Utile pour les scénarios « tous les postes de travail du lab 3 ».

## Ordre de résolution

Quand un client demande son fichier d'auto-install, Bootimus parcourt cette hiérarchie :

```
1. Per-client override        (Client.AutoInstallFile)
2. Per-group override         (ClientGroup.AutoInstallFile, if client is in a group)
3. Image default              (Image.AutoInstallFile)
4. Inline legacy script       (Image.AutoInstallScript — pre-0.1.58 setups)
5. → 404 (no auto-install configured)
```

La première correspondance non vide gagne. L'endpoint log la source depuis laquelle il a servi :

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## Placeholders

Ces tokens sont substitués par client au moment du service :

| Token | Remplacé par |
|-------|---------------|
| `{{MAC}}` | Adresse MAC du client (minuscules, séparée par des deux-points) |
| `{{CLIENT_NAME}}` | Nom convivial depuis la table Clients |
| `{{HOSTNAME}}` | Identique à `{{CLIENT_NAME}}` (alias pour plus de clarté dans les configs) |
| `{{IP}}` | IP du client qui a émis la requête |
| `{{SERVER_ADDR}}` | Adresse du serveur Bootimus |
| `{{IMAGE_NAME}}` | Nom d'affichage de l'image qui boote |
| `{{IMAGE_FILENAME}}` | Nom de fichier ISO de l'image qui boote |

Les placeholders sont de la substitution de chaîne brute — pas d'échappement. Quote-les correctement pour le format cible (XML, YAML, etc.).

## Exemples

### Ubuntu Server (cloud-init)

`data/autoinstall/ubuntu/default.yaml` :

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

Boot params (l'image concernée a déjà ceux-ci par défaut pour Ubuntu) :

```
autoinstall ds=nocloud-net;s=http://{{SERVER_ADDR}}:8080/autoinstall/{{IMAGE_FILENAME}}/
```

### Debian (preseed)

`data/autoinstall/debian/server.cfg` :

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

`data/autoinstall/rocky/workstation.ks` :

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

`data/autoinstall/windows/kiosk.xml` : document `<unattend>` standard — voir la [référence autounattend de Microsoft](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/). Les placeholders fonctionnent dans n'importe quel nœud texte :

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## Notes Windows

Les installations Windows sont pilotées par SMB. Quand une image a un fichier autounattend attaché, Bootimus :

1. Stage `AutoUnattend.xml` sur le partage SMB d'installation lors du patch de `boot.wim`.
2. Patche `startnet.cmd` pour que WinPE le copie dans `X:\AutoUnattend.xml` (le RAM disk local) au boot.
3. Lance Setup avec `setup.exe /unattend:X:\AutoUnattend.xml`.

Sans fichier autounattend par image, Setup tourne en interactif comme avant.

**Résilience au redémarrage.** WinPE redémarre en plein milieu de l'installation et se reconnecte depuis la même IP client. La config Samba embarquée définit `reset on zero vc = yes` et désactive les oplocks pour que le second `net use` ne se vautre pas sur un état de session périmé. Si tu as remplacé `data/smb/smb.conf` par le tien, miroite ces réglages.

**Annuler le patch.** Le premier patch SMB conserve une copie intacte de `boot.wim` sous le nom `boot.wim.orig` à côté. **Images** → **Retirer le patch SMB** restaure l'original et supprime le partage SMB de l'image — l'ISO boote alors exactement comme à l'upload. Les images patchées par d'anciennes versions de Bootimus n'ont pas de sauvegarde : ré-extrais d'abord l'image (ce qui produit une copie intacte fraîche), puis retire le patch.

## API REST

Tout ce qu'il y a dans l'UI est aussi un appel REST.

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

Attacher un fichier à une image :

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

Attacher un fichier à un client :

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

L'endpoint auto-install que les clients tapent au boot :

```
GET /autoinstall/<image-filename>/?mac=<mac>
```

Le query param `mac` est ajouté automatiquement par le menu de boot pour que les overrides par client se résolvent correctement.

## Dépannage

### 404 depuis `/autoinstall/...`

`no auto-install configuration for this image/client` — rien n'est attaché à aucun niveau de la chaîne de résolution. Soit attache un fichier à l'image, au client ou à son groupe, soit vérifie que `auto_install_file` pointe bien vers un fichier qui existe sous `data/autoinstall/`.

### Placeholders rendus littéralement

`{{HOSTNAME}}` qui apparaît comme une chaîne littérale dans le système installé signifie que le fichier a été servi avant que la substitution ne tourne — généralement parce que le client a booté uniquement par IP et que la requête ne contenait pas de query param `mac`. Confirme que le menu de boot génère des URLs de la forme `/autoinstall/<iso>/?mac=<mac>`.

### Mauvais fichier servi

La résolution se fait du plus spécifique au moins spécifique. Si un client a son propre override et que tu ne t'y attends pas, c'est pour ça que le défaut au niveau image n'est pas utilisé. Vérifie la ligne de log serveur :

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

Le champ `source:` te dit exactement quel slot a gagné.

### Windows Setup tourne en interactif

- L'image doit avoir un fichier autounattend attaché (image properties → Auto-Install).
- Re-patche `boot.wim` après l'attachement : **Images** → **Patch SMB** (ou ça se re-patche automatiquement au prochain boot).
- Confirme que le partage SMB est accessible depuis le client (`net view \\<server>` depuis WinPE).

### « AutoUnattend.xml not on share, running interactive setup »

Loggé par `startnet.cmd` quand le fichier n'est pas là où il s'y attend. Soit l'étape de staging a échoué (regarde le log serveur Bootimus autour de l'heure du patch), soit le partage SMB a perdu le fichier. Relance le patch SMB depuis les propriétés de l'image.

### `net use échoue après le redémarrage de la VM`

Corrigé en 0.1.58 en activant `reset on zero vc = yes` dans la config Samba embarquée. Si tu maintiens un `smb.conf` custom, ajoute :

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## Étapes suivantes

- Voir [Gestion des images](images.md) pour attacher des fichiers aux images.
- Voir [Gestion des clients](clients.md) pour les overrides par client.
- Voir [Profils de distro](distro-profiles.md) pour les IDs de profils sous-jacents qui mappent vers les sous-répertoires de la bibliothèque.
