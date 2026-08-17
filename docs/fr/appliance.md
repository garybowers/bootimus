# Appliance USB Bootimus

Une image Alpine Linux flashable et autonome qui boote sur un serveur PXE Bootimus prêt à l'emploi. Branche-la sur un switch, allume, et chaque machine du même broadcast domain peut booter en PXE dessus — pas de reconfiguration DHCP, pas d'install OS, pas de setup.

## Ce qu'il y a dedans

- **Alpine Linux** (minimal, ~100 Mo de base)
- **bootimus** avec proxyDHCP activé par défaut
- **Samba** servant `/var/lib/bootimus/isos` en lecture seule sous `\\BOOTIMUS\isos` pour les installeurs Windows qui veulent un accès SMB pendant le setup
- **dnsmasq** disponible en paquet mais désactivé (le proxyDHCP intégré de bootimus s'en charge par défaut)
- **Serveur SSH** pour l'administration à distance

## Builder l'image

Prérequis sur l'hôte de build :
- Docker (avec `--privileged` disponible)
- Go 1.24+ pour cross-compiler bootimus
- ~3 Go d'espace disque libre

Le build tourne entièrement dans un conteneur Alpine privilégié — aucun module noyau hôte n'est chargé, aucun outil installé sur ta machine.

```bash
make appliance
```

Produit `appliance/build/bootimus-appliance.img` — une image disque brute prête à flasher avec Etcher, Rufus, ou `dd`.

## Flasher sur une clé USB

**Identifie ton périphérique cible avec soin** — `dd` écrasera sans demander.

```bash
lsblk                                   # find your USB stick, e.g. /dev/sdb
sudo dd if=appliance/build/bootimus-appliance.img \
        of=/dev/sdX bs=4M conv=fsync status=progress
sync
```

Sur macOS/Windows, [Etcher](https://etcher.balena.io) ou [Rufus](https://rufus.ie) fonctionnent directement avec le fichier `.img`.

## Premier boot

1. Branche la clé USB sur n'importe quel PC avec Ethernet et un réseau filaire.
2. Boote sur USB (menu one-time boot ou changement de priorité dans le BIOS).
3. Alpine boote, récupère son IP par DHCP depuis le LAN, et démarre bootimus + samba + proxyDHCP.
4. La console affiche :

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

5. Ouvre l'URL admin depuis n'importe quelle autre machine du LAN. Connecte-toi en tant que `admin` avec le mot de passe affiché.
6. Upload ou scanne des ISOs via l'UI admin — ils atterrissent dans `/var/lib/bootimus/isos` et sont immédiatement servis en HTTP *et* via le partage SMB.

## Limitations et compromis

- **Réseau filaire uniquement.** Aucun firmware de driver WiFi n'est embarqué. Servir du PXE en WiFi est une idée pourrie de toute façon (broadcast-flooding + latence).
- **UEFI Secure Boot supporté** — sélectionne le set de bootloaders intégré `secureboot-official` et les machines cibles avec Secure Boot actif netbootent sans changement de firmware, comme sur une install bootimus standard. Voir le [guide Secure Boot](secure-boot.md) pour les limitations (les boots NBD/NFS et les distros non signées ont toujours besoin de Secure Boot désactivé).
- **Une seule partition.** Les ISOs vivent sur la même partition root qu'Alpine. Une clé de 32 Go te laisse ~29 Go pour les ISOs. Pour une bibliothèque plus grosse, étends la partition root manuellement après le premier boot (`resize2fs /dev/sda1`) ou rebuild avec `IMAGE_SIZE=16G make appliance`.
- **Coexistence proxyDHCP.** Si le LAN sur lequel tu te branches a déjà un proxyDHCP dnsmasq/ISC qui annonce du PXE, les deux proxies vont s'engueuler. Désactive l'un : soit `BOOTIMUS_PROXY_DHCP_ENABLED=false` dans `/etc/conf.d/bootimus`, soit éteins l'autre.
- **L'appliance est stateful.** La clé USB EST le serveur. ISOs, clients, plannings et settings persistent dessus. Si la clé claque en plein milieu d'un déploiement, tu voudras un backup (`make appliance` produit des builds déterministes mais tes *données* vivent sur la clé — utilise le bouton « Download Backup » dans Settings régulièrement).

## Personnaliser

Le build repose sur trois éléments :

- **`appliance/build.sh`** — l'orchestrateur. Ajuste les variables d'env `IMAGE_SIZE` et `ALPINE_BRANCH` sans éditer de code.
- **`appliance/setup.sh`** — tourne dans le chroot de l'image pendant le build. Ajoute des lignes `apk add` ici pour embarquer du tooling supplémentaire.
- **`appliance/overlay/`** — tout fichier déposé ici est copié tel quel dans le rootfs. Éditions courantes :
  - `etc/conf.d/bootimus` — désactive proxyDHCP, change les ports, fige une IP serveur
  - `etc/samba/smb.conf` — élargis le partage SMB, ajoute des tweaks spécifiques Windows
  - `etc/network/interfaces` — IP statique au lieu du DHCP
  - `etc/profile.d/bootimus-motd.sh` — remplace la bannière de login

Après chaque modif, relance `make appliance`.

## Accès SSH

Le login root par mot de passe est **désactivé** dans l'image (hygiène sécurité — tu serais choqué de voir combien d'images appliance « sécurisées » sortent avec des identifiants par défaut). Pour activer l'admin à distance :

1. Boote l'appliance une fois sur la console.
2. Lance `passwd` pour définir un mot de passe root, OU dépose une clé SSH dans `/root/.ssh/authorized_keys`.
3. `rc-service sshd restart` (SSH est déjà activé par défaut mais n'accepte pas les logins sans mot de passe).

## Rebuilder sur une nouvelle release bootimus

Chaque `make appliance` reprend l'arbre source bootimus actuel. Bump `VERSION`, fais une release, puis rebuild l'image — le binaire `bootimus` embarqué affiche la version contre laquelle tu as buildé dans le footer de l'UI admin.
