# Guide des profils de distro

Bootimus utilise des profils de distro pour détecter les types d'ISOs et générer les bons paramètres de boot. Les profils sont data-driven — tu peux ajouter le support de nouvelles distributions sans toucher au code.

## Table des matières

- [Vue d'ensemble](#vue-densemble)
- [Fonctionnement](#fonctionnement)
- [Voir les profils](#voir-les-profils)
- [Mettre à jour les profils](#mettre-à-jour-les-profils)
- [Mises à jour distantes et confidentialité](#mises-à-jour-distantes-et-confidentialité)
- [Créer des profils personnalisés](#créer-des-profils-personnalisés)
- [Champs de profil](#champs-de-profil)
- [Placeholders](#placeholders)
- [Exemples](#exemples)
- [Dépannage](#dépannage)

## Vue d'ensemble

Les profils de distro définissent :
- **Comment détecter** quelle distro est un ISO (matching de pattern sur le nom de fichier)
- **Où trouver** le kernel, l'initrd et le squashfs à l'intérieur de l'ISO
- **Quels paramètres de boot** utiliser pour le boot PXE
- **Quel type d'auto-install** est supporté (preseed, kickstart, autoinstall, etc.)

### Types de profils

| Type | Description |
|------|-------------|
| **Built-in** | Livré avec Bootimus, mis à jour depuis le dépôt central |
| **Custom** | Créé par l'utilisateur, jamais écrasé par les mises à jour |

Les profils custom ont toujours la priorité sur les profils built-in lors du matching des noms de fichiers d'ISO.

## Fonctionnement

1. Quand un ISO est uploadé ou extrait, Bootimus matche le nom de fichier contre les patterns de profils
2. Les chemins kernel/initrd du profil correspondant sont utilisés pour localiser les fichiers de boot dans l'ISO
3. Les boot params du profil deviennent le défaut (éditables dans les Properties de l'image)
4. Au moment du boot, les placeholders dans les params sont résolus en vraies URLs

### Cycle de vie d'un profil

```
Build time:    distro-profiles.json embedded in binary
                        ↓
First startup:  Profiles seeded into database
                        ↓
"Check for Updates":  Latest profiles fetched from GitHub
                        ↓
User creates:   Custom profiles stored in database (never overwritten)
```

## Voir les profils

Va dans **Boot > Distro Profiles** du panneau d'administration pour voir tous les profils chargés avec leurs patterns de nom de fichier, paramètres de boot, type (Built-in/Custom) et version.

## Mettre à jour les profils

Mettre à jour les profils est toujours une **action explicite, à la demande** — Bootimus ne contacte jamais le catalogue distant de lui-même. Tant que tu ne déclenches pas de mise à jour, ce sont les profils embarqués dans le binaire au moment du build qui sont utilisés. Voir [Mises à jour distantes et confidentialité](#mises-à-jour-distantes-et-confidentialité) pour savoir exactement ce qui est contacté et comment le désactiver.

Quand tu déclenches une mise à jour :

- Les nouveaux profils sont ajoutés
- Les profils built-in existants sont mis à jour vers la dernière version
- Les profils custom ne sont jamais modifiés

Il y a trois façons de la déclencher :

### Depuis l'interface d'administration

Clique sur **« Check for Updates »** dans l'onglet **Boot > Distro Profiles**.

### Depuis la CLI

```bash
bootimus profiles update
```

Ça utilise la même configuration de base de données que `serve` (PostgreSQL si `db.host` est défini, sinon la base SQLite locale sous `data_dir`). Ça respecte `--disable-remote-profiles` et se termine sans contacter le réseau quand ce flag est défini.

### Via l'API

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/profiles/update
```

Réponse :
```json
{
  "success": true,
  "message": "Updated to version 0.1.21 (2 added, 5 updated)"
}
```

## Mises à jour distantes et confidentialité

Bootimus est auto-hébergé et ne fait pas de « phone home » en arrière-plan. Le seul moment où il contacte un service externe pour les profils, c'est quand **toi** tu déclenches explicitement une mise à jour via l'une des méthodes ci-dessus.

**Endpoint contacté (uniquement lors d'une mise à jour explicite) :**

```
https://raw.githubusercontent.com/garybowers/bootimus/main/distro-profiles.json
```

Le catalogue d'outils équivalent (« Check for Updates » dans l'onglet **Tools** / `POST /api/tools/update`) se comporte de la même façon et contacte :

```
https://raw.githubusercontent.com/garybowers/bootimus/main/tools-profiles.json
```

Ce sont de simples requêtes `GET` non authentifiées vers l'hébergeur de fichiers statiques de GitHub. Bootimus n'envoie avec elles aucune information système, aucun identifiant, ni aucune donnée d'usage — il télécharge simplement le fichier JSON. Note que, comme pour toute requête HTTP, GitHub voit ton adresse IP source et les métadonnées de requête standard.

### Désactiver les mises à jour distantes

Pour garantir que Bootimus ne contacte jamais le catalogue distant — pour les déploiements air-gapped, ou par principe — démarre-le avec :

```bash
bootimus serve --disable-remote-profiles
```

ou définis la valeur config/env équivalente :

```yaml
# bootimus.yaml
disable_remote_profiles: true
```

```bash
# environment variable
BOOTIMUS_DISABLE_REMOTE_PROFILES=true
```

Quand c'est désactivé, les profils embarqués sont quand même initialisés depuis le binaire au premier lancement, donc Bootimus est pleinement fonctionnel hors ligne. Le bouton « Check for Updates », l'endpoint `/api/profiles/update` et `bootimus profiles update` refuseront tous de s'exécuter.

## Créer des profils personnalisés

### Via l'interface web

1. Va dans **Boot > Distro Profiles**
2. Clique sur **« + Add Custom Profile »**
3. Remplis les champs du profil
4. Clique sur **« Create Profile »**

### Via l'API

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

### Supprimer des profils personnalisés

Seuls les profils custom peuvent être supprimés. Les profils built-in sont restaurés à la prochaine mise à jour.

```bash
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/profiles/delete?id=my-distro"
```

## Champs de profil

| Champ | Requis | Description |
|-------|----------|-------------|
| `profile_id` | Oui | Identifiant unique (par ex. `ubuntu`, `my-distro`) |
| `display_name` | Oui | Nom lisible affiché dans l'UI |
| `family` | Non | Famille de distro (par ex. `debian`, `arch`, `redhat`) — pour le regroupement |
| `filename_patterns` | Oui | Sous-chaînes à matcher dans les noms de fichiers ISO (insensible à la casse) |
| `kernel_paths` | Non | Chemins à essayer pour le kernel dans l'ISO (par ex. `/casper/vmlinuz`) |
| `initrd_paths` | Non | Chemins à essayer pour l'initrd dans l'ISO |
| `squashfs_paths` | Non | Chemins à essayer pour le filesystem squashfs |
| `default_boot_params` | Non | Paramètres de boot kernel par défaut (avec support de placeholders) |
| `boot_params_with_squashfs` | Non | Boot params alternatifs utilisés quand un squashfs est détecté |
| `auto_install_type` | Non | Format d'auto-install : `preseed`, `kickstart`, `autoinstall`, `autounattend` |
| `boot_method` | Non | Override de la méthode de boot (par ex. `wimboot` pour Windows) |

## Placeholders

Les paramètres de boot supportent ces placeholders, résolus au moment du boot :

| Placeholder | Résout en | Exemple |
|-------------|-------------|---------|
| `{{BASE_URL}}` | URL HTTP du serveur | `http://192.168.1.10:8080` |
| `{{CACHE_DIR}}` | Répertoire des fichiers extraits | `ubuntu-24.04-server-amd64` |
| `{{FILENAME}}` | Nom de fichier ISO (URL-encoded) | `ubuntu-24.04-server-amd64.iso` |
| `{{SQUASHFS}}` | URL complète vers le fichier squashfs | `http://192.168.1.10:8080/boot/ubuntu.../casper/filesystem.squashfs` |

### Exemple avec placeholders

```
boot=live initrd=initrd fetch={{SQUASHFS}} ip=dhcp
```

Résout en :
```
boot=live initrd=initrd fetch=http://192.168.1.10:8080/boot/debian-live-13/live/filesystem.squashfs ip=dhcp
```

## Exemples

### ISO live basé sur Debian

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

### Distro basée sur Arch

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

### Installeur basé sur RHEL

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

## Dépannage

### ISO non détecté comme la bonne distro

Vérifie si le nom de fichier ISO matche un pattern de profil :

1. Va dans l'onglet **Distro Profiles**
2. Regarde la colonne « Filename Patterns »
3. Si aucun pattern ne matche ton nom de fichier ISO, crée un profil custom

### Boot params incorrects après extraction

1. Ouvre les **Properties** de l'image
2. Clique sur **« Re-detect »** à côté de Boot Parameters
3. Ou édite les boot params manuellement — ils supportent les placeholders

### « Check for Updates » a échoué

La mise à jour récupère depuis GitHub. Vérifie :
- Le serveur a accès à internet
- `raw.githubusercontent.com` n'est pas bloqué
- Réessaye plus tard si GitHub est down

### Profil custom qui ne matche pas

Les profils custom ont priorité sur les built-in. Assure-toi que :
- Les `filename_patterns` contiennent des sous-chaînes qui matchent ton nom de fichier ISO (insensible à la casse)
- L'ID du profil est unique
- Le profil a été sauvegardé avec succès

### Contribuer des profils

Pour ajouter un profil à la liste officielle pour tous les utilisateurs :
1. Fork le [dépôt Bootimus](https://github.com/garybowers/bootimus)
2. Édite `distro-profiles.json` à la racine du repo
3. Ajoute ton profil au tableau `profiles`
4. Soumets une pull request

Comme ça tous les utilisateurs Bootimus récupèrent le nouveau profil via « Check for Updates ».
