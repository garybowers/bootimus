#  Guide de déploiement

Guide complet pour déployer Bootimus dans différents environnements avec configurations réseau et stockage.

##  Table des matières

- [Démarrage rapide](#démarrage-rapide)
- [Déploiement Docker](#déploiement-docker)
- [Déploiement en binaire](#déploiement-en-binaire)
- [Configuration réseau](#configuration-réseau)
- [Configuration du stockage](#configuration-du-stockage)
- [Options de base de données](#options-de-base-de-données)
- [Mises à jour distantes et confidentialité](#mises-à-jour-distantes-et-confidentialité)
- [Déploiement en production](#déploiement-en-production)

## Démarrage rapide

### Docker (recommandé)

```bash
# Create data directory
mkdir -p data

# Run with SQLite (no database container needed)
docker run -d \
  --name bootimus \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -p 8081:8081/tcp \
  -v $(pwd)/data:/data \
  garybowers/bootimus:latest

# Check logs for admin password
docker logs bootimus | grep "Password"

# Access admin interface
open http://localhost:8081
```

### Binaire standalone

```bash
# Download binary
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64
chmod +x bootimus-amd64

# Create data directory
mkdir -p data

# Run (SQLite mode - no database required)
./bootimus-amd64 serve

# Admin panel: http://localhost:8081
# Admin password shown in startup logs
```

## Déploiement Docker

### Docker Compose avec PostgreSQL

```bash
# Clone repository
git clone https://github.com/garybowers/bootimus
cd bootimus

# Start with PostgreSQL
docker-compose up -d

# View logs
docker-compose logs -f bootimus
```

La stack Docker Compose inclut :
- **Serveur Bootimus** : serveur principal de boot PXE/HTTP
- **PostgreSQL** : base de données pour la gestion clients/images
- **Health checks** : monitoring automatique des services
- **Stockage persistant** : volumes de données pour les ISOs et la base

### Structure des répertoires

Bootimus crée automatiquement les sous-répertoires :
- `/data/isos/` - fichiers d'images ISO et fichiers de boot extraits (en sous-répertoires par ISO)
- `/data/bootloaders/` - fichiers de bootloader personnalisés (optionnel)
- `/data/bootimus.db` - base SQLite (si mode SQLite)

## Configuration réseau

### Réseau bridge interne par défaut

Par défaut, les conteneurs utilisent un réseau bridge interne avec port forwarding :

```yaml
networks:
  bootimus_net:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
          gateway: 172.20.0.1
```

- **Serveur Bootimus** : `172.20.0.3`
- **PostgreSQL** : `172.20.0.2`
- **Accès depuis l'hôte** : via port forwarding (par ex. `localhost:8081`)

### Réseau bridgé avec IP statique sur le LAN

Pour les environnements PXE en production, tu peux vouloir le conteneur directement sur ton LAN avec une IP statique.

#### Étape 1 : trouver ton interface réseau

```bash
ip addr show  # Linux
# Look for your primary interface (e.g., eth0, ens33, enp0s3)
```

#### Étape 2 : éditer docker-compose.yml

Décommente les sections réseau `host_bridge` :

```yaml
services:
  bootimus:
    networks:
      # Comment out internal bridge
      # bootimus_net:
      #   ipv4_address: 172.20.0.3
      # Enable host bridge
      host_bridge:
        ipv4_address: 192.168.1.100  # Your desired static IP
    environment:
      BOOTIMUS_SERVER_ADDR: 192.168.1.100  # Set static server address

networks:
  # Uncomment and configure for your LAN
  host_bridge:
    driver: macvlan
    driver_opts:
      parent: eth0  # Your network interface
    ipam:
      config:
        - subnet: 192.168.1.0/24      # Your LAN subnet
          gateway: 192.168.1.1         # Your LAN gateway
          ip_range: 192.168.1.100/32   # Container static IP
```

#### Étape 3 : configurer les détails du réseau

Mets à jour ces valeurs pour ton réseau :
- `parent` : l'interface réseau de ton hôte (par ex. `eth0`, `ens33`)
- `subnet` : ton sous-réseau LAN (par ex. `192.168.1.0/24`)
- `gateway` : l'IP de ton routeur (par ex. `192.168.1.1`)
- `ip_range` : l'IP statique pour Bootimus (par ex. `192.168.1.100/32`)
- `BOOTIMUS_SERVER_ADDR` : identique à l'IP statique

#### Étape 4 : démarrer le conteneur

```bash
docker-compose down
docker-compose up -d
```

#### Étape 5 : vérifier la connectivité

```bash
# From another machine on the LAN
curl http://192.168.1.100:8081

# Ping the container
ping 192.168.1.100
```

###  Notes importantes sur le réseau macvlan

- **Réseau macvlan** : le conteneur apparaît comme un périphérique séparé sur ton LAN
- **L'hôte ne peut pas joindre le conteneur** : la machine hôte ne peut pas communiquer directement avec les conteneurs macvlan. Utilise une VM/conteneur séparé pour l'accès admin, ou crée une interface macvlan sur l'hôte.
- **Conflits DHCP** : assure-toi que l'IP statique est hors de ta plage DHCP ou réservée dans ton serveur DHCP
- **Règles de pare-feu** : le conteneur contourne le pare-feu de l'hôte — configure le pare-feu du conteneur séparément si besoin

### Accéder aux conteneurs macvlan depuis l'hôte

Si tu as besoin d'accéder au conteneur macvlan depuis la machine hôte :

```bash
# Create a macvlan interface on the host
sudo ip link add macvlan0 link eth0 type macvlan mode bridge
sudo ip addr add 192.168.1.101/32 dev macvlan0
sudo ip link set macvlan0 up
sudo ip route add 192.168.1.100/32 dev macvlan0

# Now you can access the container from the host
curl http://192.168.1.100:8081
```

## Déploiement en binaire

### Configuration système requise

- **OS** : Linux (amd64, arm64, armv7)
- **Privilèges** : root requis pour le port 69 (TFTP), ou utilise des ports non privilégiés
- **Disque** : 10 Go+ pour le stockage des ISOs
- **Mémoire** : 512 Mo minimum, 2 Go+ recommandés

### Installation

```bash
# Download binary for your architecture
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64

# Make executable
chmod +x bootimus-amd64

# Move to system location
sudo mv bootimus-amd64 /usr/local/bin/bootimus

# Create data directory
sudo mkdir -p /opt/bootimus/data

# Create systemd service
sudo nano /etc/systemd/system/bootimus.service
```

### Service systemd

```ini
[Unit]
Description=Bootimus PXE/HTTP Boot Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/bootimus
ExecStart=/usr/local/bin/bootimus serve --data-dir /opt/bootimus/data
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable bootimus
sudo systemctl start bootimus

# Check status
sudo systemctl status bootimus

# View logs
sudo journalctl -u bootimus -f
```

## Configuration du stockage

### Structure du répertoire data

```
/opt/bootimus/data/
├── isos/                           # ISO files
│   ├── ubuntu-24.04.iso           # ISO file
│   ├── ubuntu-24.04/              # Extracted boot files
│   │   ├── vmlinuz
│   │   ├── initrd
│   │   └── casper/
│   │       └── filesystem.squashfs
│   └── debian-12.iso
├── bootloaders/                    # Custom bootloaders (optional)
├── bootimus.db                     # SQLite database (if using SQLite)
└── .admin_password                 # Generated admin password
```

### Espace disque requis

- **ISOs** : 1-10 Go par ISO
- **Fichiers extraits** : 100 Mo-3 Go par ISO
- **Base de données** : < 100 Mo
- **Recommandé** : 50 Go+ pour plusieurs ISOs

### Bonnes pratiques de stockage

1. **Utilise du SSD** : temps de boot plus rapides pour les clients
2. **Backups réguliers** : sauvegarde la base et les ISOs
3. **Surveille l'espace disque** : mets en place des alertes pour le manque d'espace
4. **Nettoie les vieux ISOs** : supprime les ISOs inutilisés pour libérer de l'espace

## Options de base de données

### Mode SQLite (par défaut)

SQLite est **activé par défaut** — aucune configuration requise !

```bash
# Run with SQLite (default)
./bootimus serve

# Database automatically created at: <data_dir>/bootimus.db
```

**Avantages** :
-  Zéro configuration
-  Base de données dans un seul fichier
-  Parfait pour les déploiements mono-serveur
-  Backups faciles (juste copier le fichier)

**Limitations** :
-  Concurrence inférieure à PostgreSQL
-  Mono-serveur uniquement (pas de clustering)

### Mode PostgreSQL

Pour les déploiements entreprise avec forte concurrence :

#### Méthode fichier de configuration

```yaml
# bootimus.yaml
db:
  host: postgres.example.com
  port: 5432
  user: bootimus
  password: secretpassword
  name: bootimus
  sslmode: require
```

#### Méthode variables d'environnement

```bash
export BOOTIMUS_DB_HOST=postgres.example.com
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secretpassword
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=require

./bootimus serve
```

**Avantages** :
-  Forte concurrence
-  Déploiements multi-serveurs
-  Réplication avancée
-  Meilleures performances à grande échelle

**Prérequis** :
- Serveur PostgreSQL 12+
- Connectivité réseau vers la base
- Infrastructure supplémentaire

## Mises à jour distantes et confidentialité

Bootimus est auto-hébergé et ne fait **pas** de « phone home » en arrière-plan. Il est livré avec un catalogue complet de profils de distro et d'outils embarqué dans le binaire, il est donc pleinement fonctionnel sans aucun accès internet sortant.

Le **seul** moment où Bootimus contacte un service externe, c'est quand un opérateur déclenche **explicitement** une mise à jour des profils/outils — via les boutons « Check for Updates » de l'interface d'administration, la commande CLI `bootimus profiles update`, ou les endpoints `POST /api/profiles/update` et `POST /api/tools/update`. Chacun d'eux effectue un `GET` non authentifié d'un fichier JSON statique sur GitHub (`raw.githubusercontent.com/garybowers/bootimus/main/...`) et n'envoie aucune information système ni aucun identifiant.

Pour garantir qu'aucun contact distant n'ait jamais lieu (par ex. les déploiements air-gapped), lance-le avec les mises à jour distantes désactivées :

```bash
bootimus serve --disable-remote-profiles
# or in bootimus.yaml:  disable_remote_profiles: true
# or via env:           BOOTIMUS_DISABLE_REMOTE_PROFILES=true
```

Voir le [Guide des profils de distro](distro-profiles.md#mises-à-jour-distantes-et-confidentialité) pour tous les détails.

## Déploiement en production

### Docker avec SQLite (le plus simple)

```bash
docker run -d \
  --name bootimus \
  --restart unless-stopped \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -p 8081:8081/tcp \
  -v /opt/bootimus/data:/data \
  garybowers/bootimus:latest
```

### Docker Compose avec PostgreSQL

```yaml
version: '3.8'

services:
  bootimus:
    image: garybowers/bootimus:latest
    container_name: bootimus
    restart: unless-stopped
    cap_add:
      - NET_BIND_SERVICE
    ports:
      - "69:69/udp"
      - "8080:8080/tcp"
      - "8081:8081/tcp"
    volumes:
      - ./data:/data
      - ./bootimus.yaml:/app/bootimus.yaml
    environment:
      - BOOTIMUS_DB_HOST=postgres
      - BOOTIMUS_DB_PASSWORD=secretpassword
    depends_on:
      - postgres

  postgres:
    image: postgres:17-alpine
    container_name: bootimus-db
    restart: unless-stopped
    environment:
      - POSTGRES_USER=bootimus
      - POSTGRES_PASSWORD=secretpassword
      - POSTGRES_DB=bootimus
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

### Options de configuration

Bootimus utilise des valeurs par défaut raisonnables et nécessite très peu de configuration.

#### Ordre de priorité de la configuration

1. Flags en ligne de commande (priorité maximale)
2. Variables d'environnement (préfixées par `BOOTIMUS_`)
3. Fichier de configuration (`bootimus.yaml`)

#### Exemple de fichier de configuration

```yaml
# bootimus.yaml
tftp_port: 69
http_port: 8080
admin_port: 8081
data_dir: ./data          # Base data directory
server_addr: ""           # Auto-detected if not specified

# Database configuration (optional)
# If no db.host is specified, SQLite is used automatically
db:
  host: localhost       # Leave empty for SQLite
  port: 5432
  user: bootimus
  password: bootimus
  name: bootimus
  sslmode: disable
```

#### Variables d'environnement

```bash
# Server settings
export BOOTIMUS_TFTP_PORT=69
export BOOTIMUS_HTTP_PORT=8080
export BOOTIMUS_ADMIN_PORT=8081
export BOOTIMUS_DATA_DIR=/var/lib/bootimus/data
export BOOTIMUS_SERVER_ADDR=192.168.1.100

# Database settings (PostgreSQL only)
export BOOTIMUS_DB_HOST=postgres      # Empty = SQLite
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secret
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=disable

./bootimus serve
```

## Dépannage

### Permission denied sur le port 69

```bash
# Run as root
sudo ./bootimus serve

# Or use Docker with NET_BIND_SERVICE capability
docker run --cap-add NET_BIND_SERVICE ...

# Or use non-privileged port
./bootimus serve --tftp-port 6969
```

### Connexion à la base échouée

```bash
# Check SQLite database
ls -la data/bootimus.db

# For PostgreSQL, test connection
psql -h localhost -U bootimus -d bootimus

# Check PostgreSQL logs
docker logs bootimus-db
```

### Le conteneur n'est pas joignable sur le LAN

```bash
# Verify macvlan configuration
docker network inspect bootimus_host_bridge

# Check IP address assignment
docker exec bootimus ip addr show

# Verify routing
ip route | grep 192.168.1.100

# Check firewall
sudo iptables -L -n | grep 192.168.1.100
```

### Espace disque épuisé

```bash
# Check disk usage
df -h /opt/bootimus/data

# Find large files
du -sh /opt/bootimus/data/*

# Clean up old ISOs
rm /opt/bootimus/data/isos/old-image.iso

# Scan for new ISOs to update database
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Étapes suivantes

-  Lis le [Guide de gestion des images](images.md) pour la gestion des ISOs
-  Voir le [Guide de la console d'administration](admin.md) pour la gestion
-  Configure le [Serveur DHCP](dhcp.md) pour le boot PXE
