#  Guide de la console d'administration

Guide complet pour utiliser l'interface d'administration Bootimus et son API REST.

##  Table des matières

- [Accès au panneau d'administration](#accès-au-panneau-dadministration)
- [Tableau de bord](#tableau-de-bord)
- [Gestion des clients](#gestion-des-clients)
- [Gestion des images](#gestion-des-images)
- [Logs de boot](#logs-de-boot)
- [API REST](#api-rest)
- [Exemples d'automatisation](#exemples-dautomatisation)
- [Bonnes pratiques de sécurité](#bonnes-pratiques-de-sécurité)

## Accès au panneau d'administration

### Interface web

```
http://your-server:8081/
```

**Prérequis** :
- L'interface d'administration tourne sur un port séparé (8081 par défaut)
- Fonctionne avec SQLite ou PostgreSQL
- Authentification par token JWT (avec backend LDAP/AD optionnel)

### Première connexion

Au premier démarrage, Bootimus génère un mot de passe administrateur aléatoire :

```
╔════════════════════════════════════════════════════════════════╗
║                    ADMIN PASSWORD GENERATED                    ║
╠════════════════════════════════════════════════════════════════╣
║  Username: admin                                               ║
║  Password: AbCdEfGh1234567890-_XyZ123456                       ║
╠════════════════════════════════════════════════════════════════╣
║  This password will NOT be shown again!                        ║
║  Save it now or reset it using --reset-admin-password flag     ║
╚════════════════════════════════════════════════════════════════╝
```

Va sur `http://your-server:8081`, tu tomberas sur une page de connexion dédiée. Entre les identifiants admin pour accéder au panneau.

**Identifiants de connexion** :
- **Username** : `admin`
- **Password** : à récupérer dans les logs de démarrage du serveur

Si LDAP est configuré, un menu déroulant apparaît sur la page de connexion pour choisir entre l'authentification locale et LDAP. Voir le [Guide d'authentification](authentication.md) pour les détails.

### Démarrage rapide

1. Démarre Bootimus :
   ```bash
   docker-compose up -d
   # OR
   ./bootimus serve
   ```

2. Copie le mot de passe admin depuis les logs serveur

3. Ouvre ton navigateur sur `http://localhost:8081/`

4. Connecte-toi avec le nom d'utilisateur `admin` et le mot de passe généré

## Tableau de bord

Le tableau de bord fournit des statistiques en temps réel :

-  **Clients totaux** — Tous les clients enregistrés
-  **Clients actifs** — Clients activés autorisés à booter
-  **Images totales** — Toutes les images ISO
-  **Images activées** — Images disponibles dans le menu de boot
-  **Boots totaux** — Nombre de tentatives de boot

Toutes les statistiques se mettent à jour en temps réel via WebSocket/SSE.

## Gestion des clients

### Ajouter un client

1. Clique sur le bouton **« Add Client »**
2. Entre l'adresse MAC (format : `00:11:22:33:44:55`)
3. Optionnellement, ajoute un nom et une description
4. Coche **« Enabled »** pour autoriser le boot
5. Clique sur **« Create Client »**

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Machine 1",
    "description": "Test workstation",
    "enabled": true
  }'
```

### Modifier un client

1. Clique sur **« Edit »** sur n'importe quelle ligne client
2. Modifie le nom, la description ou le statut activé
3. Sélectionne les ISOs auxquels ce client peut accéder (multi-sélection)
4. Clique sur **« Update Client »**

**Via l'API** :
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Name",
    "enabled": false
  }'
```

### Supprimer un client

Clique sur **« Delete »** sur n'importe quelle ligne client et confirme la suppression.

**Via l'API** :
```bash
curl -u admin:password -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

### Assigner des images à un client

**Via l'interface web** :
1. Clique sur **« Edit »** sur le client
2. Sélectionne les images dans la liste déroulante multi-sélection
3. Clique sur **« Update Client »**

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "image_filenames": ["ubuntu-24.04.iso", "debian-12.iso"]
  }'
```

## Gestion des images

### Uploader un ISO

**Via l'interface web** :
1. Clique sur le bouton **« Upload ISO »**
2. Drag-and-drop le fichier ISO ou clique pour parcourir
3. Optionnellement, ajoute une description
4. Coche **« Public »** pour le rendre accessible à tous les clients
5. Clique sur **« Upload »**

**Limite d'upload** : 10 Go par fichier

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Télécharger depuis une URL

Télécharge les ISOs directement sur le serveur :

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

# Monitor progress
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Extraire kernel/initrd

Extrait les fichiers de boot pour accélérer le boot et réduire la bande passante :

**Via l'interface web** :
1. Trouve l'image dans l'onglet **Images**
2. Clique sur le bouton **« Extract »**
3. Attends que l'extraction se termine

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'
```

**Avantages** :
-  Boot plus rapide (télécharge 100 Mo au lieu de 6 Go)
-  Bande passante réduite (critique pour plusieurs clients)
-  Meilleure compatibilité (certains ISOs ne gèrent pas sanboot)

Voir le [Guide de gestion des images](images.md) pour les détails sur l'extraction.

### Télécharger les fichiers netboot

Pour les ISOs d'installation Debian/Ubuntu qui nécessitent le netboot :

**Via l'interface web** :
1. Trouve l'image avec le badge **« Netboot Required »**
2. Clique sur le bouton **« Download Netboot »**
3. Attends le téléchargement et l'extraction

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

**Qu'est-ce que les fichiers netboot ?**
- Fichiers de boot minimaux officiels Debian/Ubuntu
- Téléchargement ~30-50 Mo (au lieu de l'ISO complet)
- L'installeur télécharge les paquets depuis internet pendant l'installation
- Toujours les paquets à jour

Voir [Support netboot](images.md#netboot-support) pour plus de détails.

### Scanner les ISOs

Scanne le répertoire data à la recherche d'ISOs ajoutés manuellement :

**Via l'interface web** :
1. Copie manuellement les fichiers ISO dans le répertoire `/data/isos/`
2. Clique sur le bouton **« Scan for ISOs »**
3. Bootimus détecte et enregistre les nouveaux ISOs

**Via l'API** :
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### Activer/Désactiver une image

**Via l'interface web** :
- Clique sur **« Enable »** ou **« Disable »** sur n'importe quelle image
- Les images désactivées n'apparaîtront pas dans les menus de boot

**Via l'API** :
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

### Rendre publique/privée

**Via l'interface web** :
- Clique sur **« Make Public »** pour autoriser tous les clients à y accéder
- Clique sur **« Make Private »** pour restreindre aux clients assignés uniquement

**Via l'API** :
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": true}'
```

### Supprimer une image

**Via l'interface web** :
- Clique sur **« Delete »** sur n'importe quelle ligne image
- Confirme la suppression
- L'image est retirée de la base
- Le fichier ISO reste sur le disque (supprime-le manuellement si nécessaire)

**Via l'API** :
```bash
# Delete from database only
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso"

# Delete from database and filesystem
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso&delete_file=true"
```

## Logs de boot

Visualise les tentatives de boot récentes avec streaming live :

**Informations affichées** :
-  Timestamp
-  Adresse MAC du client
-  Nom de l'image
-  Adresse IP
- / Statut succès/échec
-  Messages d'erreur (le cas échéant)

**Auto-refresh** : les logs se mettent à jour en temps réel via SSE (Server-Sent Events)

**Via l'API** :
```bash
# Get last 100 logs (default)
curl -u admin:password http://localhost:8081/api/logs

# Get last 10 logs
curl -u admin:password http://localhost:8081/api/logs?limit=10

# Get last 500 logs (max 1000)
curl -u admin:password http://localhost:8081/api/logs?limit=500
```

## API REST

Toutes les fonctions admin sont disponibles via une API REST pour l'automatisation.

### Authentification

Authentification HTTP Basic requise pour tous les endpoints :
- **Username** : `admin`
- **Password** : auto-généré au premier démarrage

```bash
curl -u admin:your-password http://localhost:8081/api/stats
```

### Endpoints de l'API

#### Stats

```bash
GET /api/stats
```

**Réponse** :
```json
{
  "success": true,
  "data": {
    "total_clients": 10,
    "active_clients": 8,
    "total_images": 5,
    "enabled_images": 4,
    "total_boots": 127
  }
}
```

#### Clients

| Méthode | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/clients` | Liste tous les clients |
| `GET` | `/api/clients?mac=<MAC>` | Récupère un client par MAC |
| `POST` | `/api/clients` | Crée un client |
| `PUT` | `/api/clients?mac=<MAC>` | Met à jour un client |
| `DELETE` | `/api/clients?mac=<MAC>` | Supprime un client |
| `POST` | `/api/clients/assign` | Assigne des images à un client |

#### Images

| Méthode | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/images` | Liste toutes les images |
| `GET` | `/api/images?filename=<name>` | Récupère une image |
| `PUT` | `/api/images?filename=<name>` | Met à jour une image |
| `DELETE` | `/api/images?filename=<name>` | Supprime une image |
| `POST` | `/api/images/upload` | Upload un ISO |
| `POST` | `/api/images/download` | Télécharge un ISO depuis une URL |
| `POST` | `/api/images/extract` | Extrait kernel/initrd |
| `POST` | `/api/images/netboot/download` | Télécharge les fichiers netboot |
| `POST` | `/api/scan` | Scanne les nouveaux ISOs |

#### Téléchargements

| Méthode | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/downloads` | Liste les téléchargements actifs |
| `GET` | `/api/downloads/progress?filename=<name>` | Progression du téléchargement |

#### Logs

| Méthode | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/logs?limit=<N>` | Récupère les logs de boot |
| `GET` | `/api/logs/stream` | Flux SSE des logs en temps réel |

## Exemples d'automatisation

### Ajout en masse de clients

```bash
#!/bin/bash
# bulk-add-clients.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

CLIENTS=(
  "00:11:22:33:44:01:Server1"
  "00:11:22:33:44:02:Server2"
  "00:11:22:33:44:03:Workstation1"
)

for entry in "${CLIENTS[@]}"; do
  IFS=':' read -r mac1 mac2 mac3 mac4 mac5 mac6 name <<< "$entry"
  mac="${mac1}:${mac2}:${mac3}:${mac4}:${mac5}:${mac6}"

  curl -u admin:$ADMIN_PASSWORD -X POST http://localhost:8081/api/clients \
    -H "Content-Type: application/json" \
    -d "{\"mac_address\":\"$mac\",\"name\":\"$name\",\"enabled\":true}"

  echo "Added $name ($mac)"
done
```

### Rendre toutes les images publiques

```bash
#!/bin/bash
# make-all-public.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

images=$(curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/images | jq -r '.data[].filename')

for filename in $images; do
  curl -u admin:$ADMIN_PASSWORD -X PUT "http://localhost:8081/api/images?filename=$filename" \
    -H "Content-Type: application/json" \
    -d '{"public":true}'
  echo "Made $filename public"
done
```

### Surveiller les tentatives de boot

```bash
#!/bin/bash
# monitor-boots.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

while true; do
  clear
  echo "=== Recent Boot Attempts ==="
  curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/logs?limit=20 | \
    jq -r '.data[] | "\(.created_at) | \(.mac_address) | \(.image_name) | \(if .success then "" else "✗" end)"'
  sleep 5
done
```

### Exporter les statistiques

```bash
#!/bin/bash
# export-stats.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

echo "Bootimus Usage Report - $(date)"
echo "================================"

stats=$(curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/stats | jq '.data')

echo "Total Clients: $(echo $stats | jq -r '.total_clients')"
echo "Active Clients: $(echo $stats | jq -r '.active_clients')"
echo "Total Images: $(echo $stats | jq -r '.total_images')"
echo "Total Boots: $(echo $stats | jq -r '.total_boots')"

echo -e "\nTop Clients by Boot Count:"
curl -u admin:$ADMIN_PASSWORD -s http://localhost:8081/api/clients | \
  jq -r '.data | sort_by(.boot_count) | reverse | .[:5] | .[] | "\(.boot_count) boots - \(.name // .mac_address)"'
```

## Bonnes pratiques de sécurité

### Isolation réseau

Garde le port admin séparé du réseau de boot :

```bash
# Allow boot traffic (TFTP/HTTP) on one interface
# Allow admin traffic on different interface or localhost only
```

### Règles de pare-feu

```bash
# Allow admin access only from specific IP range
sudo ufw allow from 192.168.1.0/24 to any port 8081

# Or block admin port from external access entirely
sudo ufw deny 8081
```

### Tunnel SSH

Accède à l'interface admin de manière sécurisée via tunnel SSH :

```bash
# Create SSH tunnel
ssh -L 8081:localhost:8081 user@bootimus-server

# Access admin panel
open http://localhost:8081/
```

### Accès VPN

- Place le port admin de Bootimus uniquement sur le réseau VPN
- Exige une connexion VPN pour l'accès admin
- Garde les ports de boot (69, 8080) sur un segment réseau séparé

### Gestion des mots de passe

-  Stocke le mot de passe admin de manière sécurisée (gestionnaire de mots de passe)
-  Renouvelle le mot de passe régulièrement en supprimant `.admin_password` et en redémarrant
- 🛡 Envisage une couche d'authentification supplémentaire (nginx avec certificats clients)

## Dépannage

### L'interface admin ne charge pas

```bash
# Check service is running
docker ps | grep bootimus

# Check logs
docker logs bootimus

# Verify port is accessible
curl -u admin:password http://localhost:8081/api/stats

# Check firewall
sudo ufw status | grep 8081
```

### Impossible d'uploader de gros ISOs

```bash
# Check available disk space
df -h /opt/bootimus/data

# Upload limit is 10GB by default
# For larger ISOs, use download from URL or manual copy + scan
```

### Les changements ne se reflètent pas

- Force le rafraîchissement du navigateur (Ctrl+F5 ou Cmd+Shift+R)
- Vérifie la console du navigateur pour les erreurs (F12)
- Vérifie les réponses de l'API avec curl
- Consulte les logs serveur pour les erreurs détaillées

### L'API retourne des erreurs

```bash
# Check request format (JSON content-type for POST/PUT)
curl -v -u admin:password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","name":"Test"}'

# Verify resource exists for update/delete
curl -u admin:password http://localhost:8081/api/images | jq

# Check server logs
docker logs bootimus | tail -50
```

## Étapes suivantes

-  Lis le [Guide de gestion des images](images.md) pour la gestion des ISOs
-  Voir le [Guide de déploiement](deployment.md) pour une configuration en production
-  Configure le [Serveur DHCP](dhcp.md) pour le boot PXE
-  Configure la [Gestion des clients](clients.md) pour le contrôle d'accès
