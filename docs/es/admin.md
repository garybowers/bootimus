#  Guía de la consola admin

Guía completa para usar la interfaz admin de Bootimus y la API REST.

##  Tabla de contenidos

- [Acceder al panel admin](#acceder-al-panel-admin)
- [Dashboard](#dashboard)
- [Gestión de clientes](#gestión-de-clientes)
- [Gestión de imágenes](#gestión-de-imágenes)
- [Logs de arranque](#logs-de-arranque)
- [API REST](#api-rest)
- [Ejemplos de automatización](#ejemplos-de-automatización)
- [Buenas prácticas de seguridad](#buenas-prácticas-de-seguridad)

## Acceder al panel admin

### Interfaz web

```
http://your-server:8081/
```

**Requisitos**:
- La interfaz admin corre en un puerto separado (por defecto 8081)
- Funciona con SQLite o PostgreSQL
- Autenticación basada en tokens JWT (con backend LDAP/AD opcional)

### Primer inicio de sesión

En el primer arranque, Bootimus genera una contraseña admin aleatoria:

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

Navega a `http://your-server:8081` y verás una página de login dedicada. Introduce las credenciales admin para acceder al panel.

**Credenciales de login**:
- **Usuario**: `admin`
- **Contraseña**: Revisa los logs de arranque del servidor

Si LDAP está configurado, aparecerá un desplegable en la página de login para elegir entre autenticación local y LDAP. Mira la [Guía de autenticación](authentication.md) para más detalles.

### Inicio rápido

1. Arranca Bootimus:
   ```bash
   docker-compose up -d
   # OR
   ./bootimus serve
   ```

2. Copia la contraseña admin de los logs del servidor

3. Abre el navegador en `http://localhost:8081/`

4. Inicia sesión con el usuario `admin` y la contraseña generada

## Dashboard

El dashboard proporciona estadísticas en tiempo real:

-  **Total de clientes** - Todos los clientes registrados
-  **Clientes activos** - Clientes habilitados que pueden arrancar
-  **Total de imágenes** - Todas las imágenes ISO
-  **Imágenes habilitadas** - Imágenes disponibles en el menú de arranque
-  **Total de arranques** - Número de intentos de arranque

Todas las estadísticas se actualizan en tiempo real vía WebSocket/SSE.

## Gestión de clientes

### Añadir un cliente

1. Haz click en el botón **"Add Client"**
2. Introduce la dirección MAC (formato: `00:11:22:33:44:55`)
3. Opcionalmente añade nombre y descripción
4. Marca **"Enabled"** para permitir el arranque
5. Haz click en **"Create Client"**

**Vía API**:
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

### Editar un cliente

1. Haz click en **"Edit"** en cualquier fila de cliente
2. Modifica nombre, descripción o estado de habilitación
3. Selecciona a qué ISOs puede acceder este cliente (multi-selección)
4. Haz click en **"Update Client"**

**Vía API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Name",
    "enabled": false
  }'
```

### Borrar un cliente

Haz click en **"Delete"** en cualquier fila de cliente y confirma el borrado.

**Vía API**:
```bash
curl -u admin:password -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

### Asignar imágenes a un cliente

**Vía interfaz web**:
1. Haz click en **"Edit"** en el cliente
2. Selecciona imágenes del desplegable multi-selección
3. Haz click en **"Update Client"**

**Vía API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "image_filenames": ["ubuntu-24.04.iso", "debian-12.iso"]
  }'
```

## Gestión de imágenes

### Subir una ISO

**Vía interfaz web**:
1. Haz click en el botón **"Upload ISO"**
2. Arrastra y suelta el archivo ISO o haz click para buscar
3. Opcionalmente añade descripción
4. Marca **"Public"** para hacerla disponible a todos los clientes
5. Haz click en **"Upload"**

**Límite de subida**: 10 GB por archivo

**Vía API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Descargar desde URL

Descarga ISOs directamente al servidor:

**Vía interfaz web**:
1. Haz click en el botón **"Download from URL"**
2. Introduce la URL de descarga del ISO
3. Añade descripción
4. Haz click en **"Download"**

**Vía API**:
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

### Extraer kernel/initrd

Extrae archivos de arranque para arranques más rápidos y menor ancho de banda:

**Vía interfaz web**:
1. Encuentra la imagen en la pestaña **Images**
2. Haz click en el botón **"Extract"**
3. Espera a que se complete la extracción

**Vía API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'
```

**Beneficios**:
-  Arranque más rápido (descarga 100 MB en vez de 6 GB)
-  Menor ancho de banda (crítico para múltiples clientes)
-  Mejor compatibilidad (algunas ISOs no soportan sanboot)

Mira la [Guía de gestión de imágenes](images.md) para información detallada sobre extracción.

### Descargar archivos netboot

Para ISOs instaladores de Debian/Ubuntu que requieren netboot:

**Vía interfaz web**:
1. Encuentra la imagen con el badge **"Netboot Required"**
2. Haz click en el botón **"Download Netboot"**
3. Espera a la descarga y extracción

**Vía API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

**¿Qué son los archivos netboot?**
- Archivos de arranque mínimos oficiales de Debian/Ubuntu
- Descarga de ~30-50 MB (en lugar del ISO completo)
- El instalador descarga paquetes desde internet durante la instalación
- Siempre obtienes los últimos paquetes

Mira [Soporte netboot](images.md#netboot-support) para más detalles.

### Escanear ISOs

Escanea el directorio de datos en busca de ISOs añadidos manualmente:

**Vía interfaz web**:
1. Copia manualmente los archivos ISO al directorio `/data/isos/`
2. Haz click en el botón **"Scan for ISOs"**
3. Bootimus detecta y registra los nuevos ISOs

**Vía API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### Habilitar/deshabilitar imagen

**Vía interfaz web**:
- Haz click en el botón **"Enable"** o **"Disable"** en cualquier imagen
- Las imágenes deshabilitadas no aparecerán en los menús de arranque

**Vía API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

### Hacer pública/privada

**Vía interfaz web**:
- Haz click en **"Make Public"** para permitir el acceso a todos los clientes
- Haz click en **"Make Private"** para restringir solo a clientes asignados

**Vía API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": true}'
```

### Borrar imagen

**Vía interfaz web**:
- Haz click en **"Delete"** en cualquier fila de imagen
- Confirma el borrado
- La imagen se elimina de la base de datos
- El archivo ISO permanece en disco (bórralo manualmente si es necesario)

**Vía API**:
```bash
# Delete from database only
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso"

# Delete from database and filesystem
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso&delete_file=true"
```

## Logs de arranque

Visualiza los intentos de arranque recientes con streaming en vivo:

**Información mostrada**:
-  Timestamp
-  Dirección MAC del cliente
-  Nombre de la imagen
-  Dirección IP
- / Estado de éxito/fallo
-  Mensajes de error (si los hay)

**Auto-refresco**: Los logs se actualizan en tiempo real vía SSE (Server-Sent Events)

**Vía API**:
```bash
# Get last 100 logs (default)
curl -u admin:password http://localhost:8081/api/logs

# Get last 10 logs
curl -u admin:password http://localhost:8081/api/logs?limit=10

# Get last 500 logs (max 1000)
curl -u admin:password http://localhost:8081/api/logs?limit=500
```

## API REST

Todas las funciones admin están disponibles vía API REST para automatización.

### Autenticación

Se requiere HTTP Basic Authentication para todos los endpoints:
- **Usuario**: `admin`
- **Contraseña**: Auto-generada en el primer arranque

```bash
curl -u admin:your-password http://localhost:8081/api/stats
```

### Endpoints de la API

#### Stats

```bash
GET /api/stats
```

**Respuesta**:
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

#### Clientes

| Método | Endpoint | Descripción |
|--------|----------|-------------|
| `GET` | `/api/clients` | Lista todos los clientes |
| `GET` | `/api/clients?mac=<MAC>` | Obtiene cliente por MAC |
| `POST` | `/api/clients` | Crea cliente |
| `PUT` | `/api/clients?mac=<MAC>` | Actualiza cliente |
| `DELETE` | `/api/clients?mac=<MAC>` | Borra cliente |
| `POST` | `/api/clients/assign` | Asigna imágenes a un cliente |

#### Imágenes

| Método | Endpoint | Descripción |
|--------|----------|-------------|
| `GET` | `/api/images` | Lista todas las imágenes |
| `GET` | `/api/images?filename=<name>` | Obtiene imagen |
| `PUT` | `/api/images?filename=<name>` | Actualiza imagen |
| `DELETE` | `/api/images?filename=<name>` | Borra imagen |
| `POST` | `/api/images/upload` | Sube ISO |
| `POST` | `/api/images/download` | Descarga ISO desde URL |
| `POST` | `/api/images/extract` | Extrae kernel/initrd |
| `POST` | `/api/images/netboot/download` | Descarga archivos netboot |
| `POST` | `/api/scan` | Escanea nuevos ISOs |

#### Descargas

| Método | Endpoint | Descripción |
|--------|----------|-------------|
| `GET` | `/api/downloads` | Lista descargas activas |
| `GET` | `/api/downloads/progress?filename=<name>` | Progreso de descarga |

#### Logs

| Método | Endpoint | Descripción |
|--------|----------|-------------|
| `GET` | `/api/logs?limit=<N>` | Obtiene logs de arranque |
| `GET` | `/api/logs/stream` | Stream SSE de logs en tiempo real |

## Ejemplos de automatización

### Añadir clientes en masa

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

### Hacer todas las imágenes públicas

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

### Monitorizar intentos de arranque

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

### Exportar estadísticas

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

## Buenas prácticas de seguridad

### Aislamiento de red

Mantén el puerto admin separado de la red de arranque:

```bash
# Allow boot traffic (TFTP/HTTP) on one interface
# Allow admin traffic on different interface or localhost only
```

### Reglas de firewall

```bash
# Allow admin access only from specific IP range
sudo ufw allow from 192.168.1.0/24 to any port 8081

# Or block admin port from external access entirely
sudo ufw deny 8081
```

### Túnel SSH

Accede a la interfaz admin de forma segura vía túnel SSH:

```bash
# Create SSH tunnel
ssh -L 8081:localhost:8081 user@bootimus-server

# Access admin panel
open http://localhost:8081/
```

### Acceso VPN

- Coloca el puerto admin de Bootimus solo en la red VPN
- Requiere conexión VPN para acceso admin
- Mantén los puertos de arranque (69, 8080) en un segmento de red separado

### Gestión de contraseñas

-  Almacena la contraseña admin de forma segura (gestor de contraseñas)
-  Rota la contraseña periódicamente borrando `.admin_password` y reiniciando
- 🛡 Considera una capa de autenticación adicional (nginx con client certs)

## Solución de problemas

### La interfaz admin no carga

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

### No se pueden subir ISOs grandes

```bash
# Check available disk space
df -h /opt/bootimus/data

# Upload limit is 10GB by default
# For larger ISOs, use download from URL or manual copy + scan
```

### Los cambios no se reflejan

- Refresco fuerte del navegador (Ctrl+F5 o Cmd+Shift+R)
- Revisa la consola del navegador en busca de errores (F12)
- Verifica las respuestas de la API con curl
- Revisa los logs del servidor para errores detallados

### La API devuelve errores

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

## Siguientes pasos

-  Lee la [Guía de gestión de imágenes](images.md) para manejo de ISOs
-  Mira la [Guía de despliegue](deployment.md) para setup en producción
-  Configura el [servidor DHCP](dhcp.md) para arranque PXE
-  Configura la [Gestión de clientes](clients.md) para control de acceso
