#  Guía de despliegue

Guía completa para desplegar Bootimus en varios entornos con configuraciones de red y almacenamiento.

##  Tabla de contenidos

- [Inicio rápido](#inicio-rápido)
- [Despliegue con Docker](#despliegue-con-docker)
- [Despliegue con binario](#despliegue-con-binario)
- [Configuración de red](#configuración-de-red)
- [Configuración de almacenamiento](#configuración-de-almacenamiento)
- [Opciones de base de datos](#opciones-de-base-de-datos)
- [Actualizaciones remotas y privacidad](#actualizaciones-remotas-y-privacidad)
- [Despliegue en producción](#despliegue-en-producción)

## Inicio rápido

### Docker (recomendado)

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

### Binario standalone

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

## Despliegue con Docker

### Docker Compose con PostgreSQL

```bash
# Clone repository
git clone https://github.com/garybowers/bootimus
cd bootimus

# Start with PostgreSQL
docker-compose up -d

# View logs
docker-compose logs -f bootimus
```

El stack de Docker Compose incluye:
- **Servidor Bootimus**: Servidor principal de arranque PXE/HTTP
- **PostgreSQL**: Base de datos para gestión de clientes/imágenes
- **Health checks**: Monitorización automática de servicios
- **Almacenamiento persistente**: Volúmenes de datos para ISOs y base de datos

### Estructura de directorios

Bootimus crea automáticamente los subdirectorios:
- `/data/isos/` - Archivos de imagen ISO y archivos de arranque extraídos (en subdirectorios por ISO)
- `/data/bootloaders/` - Archivos de bootloader custom (opcional)
- `/data/bootimus.db` - Base de datos SQLite (si se usa modo SQLite)

## Configuración de red

### Red bridge interna por defecto

Por defecto, los contenedores usan una red bridge interna con port forwarding:

```yaml
networks:
  bootimus_net:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
          gateway: 172.20.0.1
```

- **Servidor Bootimus**: `172.20.0.3`
- **PostgreSQL**: `172.20.0.2`
- **Acceso desde el host**: Vía port forwarding (p. ej., `localhost:8081`)

### Red bridged con IP estática en la LAN

Para entornos PXE de producción, puede que quieras el contenedor directamente en tu LAN con una IP estática.

#### Paso 1: Encuentra tu interfaz de red

```bash
ip addr show  # Linux
# Look for your primary interface (e.g., eth0, ens33, enp0s3)
```

#### Paso 2: Edita docker-compose.yml

Descomenta las secciones de red `host_bridge`:

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

#### Paso 3: Configura los detalles de red

Actualiza estos valores para tu red:
- `parent`: La interfaz de red de tu host (p. ej., `eth0`, `ens33`)
- `subnet`: Tu subnet LAN (p. ej., `192.168.1.0/24`)
- `gateway`: IP de tu router (p. ej., `192.168.1.1`)
- `ip_range`: La IP estática para Bootimus (p. ej., `192.168.1.100/32`)
- `BOOTIMUS_SERVER_ADDR`: Igual que la IP estática

#### Paso 4: Arranca el contenedor

```bash
docker-compose down
docker-compose up -d
```

#### Paso 5: Verifica la conectividad

```bash
# From another machine on the LAN
curl http://192.168.1.100:8081

# Ping the container
ping 192.168.1.100
```

###  Notas importantes para redes Macvlan

- **Red Macvlan**: El contenedor aparece como un dispositivo separado en tu LAN
- **El host no puede alcanzar el contenedor**: La máquina host no puede comunicarse directamente con contenedores macvlan. Usa una VM/contenedor separado para acceso admin, o crea una interfaz macvlan en el host.
- **Conflictos de DHCP**: Asegúrate de que la IP estática está fuera de tu rango DHCP o reservada en tu servidor DHCP
- **Reglas de firewall**: El contenedor bypassa el firewall del host — configura el firewall del contenedor por separado si es necesario

### Acceder a contenedores Macvlan desde el host

Si necesitas acceder al contenedor macvlan desde la máquina host:

```bash
# Create a macvlan interface on the host
sudo ip link add macvlan0 link eth0 type macvlan mode bridge
sudo ip addr add 192.168.1.101/32 dev macvlan0
sudo ip link set macvlan0 up
sudo ip route add 192.168.1.100/32 dev macvlan0

# Now you can access the container from the host
curl http://192.168.1.100:8081
```

## Despliegue con binario

### Requisitos del sistema

- **OS**: Linux (amd64, arm64, armv7)
- **Privilegios**: Root requerido para el puerto 69 (TFTP), o usa puertos no privilegiados
- **Disco**: 10 GB+ para almacenamiento de ISOs
- **Memoria**: 512 MB mínimo, 2 GB+ recomendado

### Instalación

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

### Servicio systemd

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

## Configuración de almacenamiento

### Estructura del directorio de datos

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

### Requisitos de espacio en disco

- **ISOs**: 1-10 GB por ISO
- **Archivos extraídos**: 100 MB-3 GB por ISO
- **Base de datos**: < 100 MB
- **Recomendado**: 50 GB+ para múltiples ISOs

### Buenas prácticas de almacenamiento

1. **Usa SSD**: Arranques más rápidos para los clientes
2. **Backups regulares**: Backup de base de datos e ISOs
3. **Monitoriza el espacio en disco**: Configura alertas de poco espacio
4. **Limpia ISOs antiguos**: Elimina ISOs sin usar para liberar espacio

## Opciones de base de datos

### Modo SQLite (por defecto)

SQLite está **habilitado por defecto** — ¡no se requiere configuración!

```bash
# Run with SQLite (default)
./bootimus serve

# Database automatically created at: <data_dir>/bootimus.db
```

**Beneficios**:
-  Cero configuración
-  Base de datos en un solo archivo
-  Perfecto para despliegues de un solo servidor
-  Backups fáciles (solo copia el archivo)

**Limitaciones**:
-  Menor concurrencia que PostgreSQL
-  Solo un servidor (sin clustering)

### Modo PostgreSQL

Para despliegues enterprise con alta concurrencia:

#### Método con archivo de configuración

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

#### Método con variables de entorno

```bash
export BOOTIMUS_DB_HOST=postgres.example.com
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secretpassword
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=require

./bootimus serve
```

**Beneficios**:
-  Alta concurrencia
-  Despliegues multi-servidor
-  Replicación avanzada
-  Mejor rendimiento a escala

**Requisitos**:
- Servidor PostgreSQL 12+
- Conectividad de red a la base de datos
- Infraestructura adicional

## Actualizaciones remotas y privacidad

Bootimus es self-hosted y **no** llama a casa en segundo plano. Viene con un catálogo completo de perfiles de distro y herramientas embebido en el binario, así que es totalmente funcional sin acceso saliente a internet.

La **única** vez que Bootimus contacta un servicio externo es cuando un operador dispara **explícitamente** una actualización de perfiles/herramientas — mediante los botones "Check for Updates" en la interfaz admin, el comando de CLI `bootimus profiles update`, o los endpoints `POST /api/profiles/update` y `POST /api/tools/update`. Cada uno de ellos realiza un `GET` sin autenticar de un archivo JSON estático en GitHub (`raw.githubusercontent.com/garybowers/bootimus/main/...`) y no envía información del sistema ni identificadores.

Para garantizar que nunca ocurra ningún contacto remoto (p. ej., despliegues air-gapped), arráncalo con las actualizaciones remotas desactivadas:

```bash
bootimus serve --disable-remote-profiles
# or in bootimus.yaml:  disable_remote_profiles: true
# or via env:           BOOTIMUS_DISABLE_REMOTE_PROFILES=true
```

Consulta la [Guía de perfiles de distro](distro-profiles.md#actualizaciones-remotas-y-privacidad) para todos los detalles.

## Despliegue en producción

### Docker con SQLite (lo más simple)

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

### Docker Compose con PostgreSQL

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

### Opciones de configuración

Bootimus usa defaults sensatos y requiere configuración mínima.

#### Precedencia de configuración

1. Flags de línea de comandos (mayor prioridad)
2. Variables de entorno (prefijadas con `BOOTIMUS_`)
3. Archivo de configuración (`bootimus.yaml`)

#### Ejemplo de archivo de configuración

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

#### Variables de entorno

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

## Solución de problemas

### Permission Denied en el puerto 69

```bash
# Run as root
sudo ./bootimus serve

# Or use Docker with NET_BIND_SERVICE capability
docker run --cap-add NET_BIND_SERVICE ...

# Or use non-privileged port
./bootimus serve --tftp-port 6969
```

### Falló la conexión a la base de datos

```bash
# Check SQLite database
ls -la data/bootimus.db

# For PostgreSQL, test connection
psql -h localhost -U bootimus -d bootimus

# Check PostgreSQL logs
docker logs bootimus-db
```

### No se puede alcanzar el contenedor en la LAN

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

### Sin espacio en disco

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

## Siguientes pasos

-  Lee la [Guía de gestión de imágenes](images.md) para manejo de ISOs
-  Mira la [Guía de la consola admin](admin.md) para gestión
-  Configura el [servidor DHCP](dhcp.md) para arranque PXE
