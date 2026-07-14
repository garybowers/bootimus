# Guía de autenticación

Bootimus usa autenticación JWT (JSON Web Token) para el panel admin. Opcionalmente, puedes conectar un servidor LDAP o Active Directory como backend de autenticación.

## Tabla de contenidos

- [Autenticación local](#autenticación-local)
- [Flujo de login](#flujo-de-login)
- [Autenticación de la API](#autenticación-de-la-api)
- [LDAP / Active Directory](#ldap--active-directory)
- [Referencia de configuración](#referencia-de-configuración)
- [Solución de problemas](#solución-de-problemas)

## Autenticación local

Por defecto, Bootimus usa cuentas locales almacenadas en la base de datos (SQLite o PostgreSQL).

### Cuenta admin por defecto

En el primer arranque se genera una contraseña aleatoria y se imprime en los logs del servidor:

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

### Resetear contraseña admin

Resetear a una nueva contraseña aleatoria (arranca una instancia completa del servidor):
```bash
./bootimus serve --reset-admin-password
# o con Docker
docker exec bootimus /bootimus serve --reset-admin-password
```

O establecer una contraseña concreta directamente en la base de datos, sin
abrir ningún puerto (ideal para recuperación de emergencia):
```bash
# Solicitud interactiva (entrada oculta, mantiene la contraseña fuera del historial del shell)
./bootimus user set-password admin

# O proporcionarla de forma no interactiva para scripting
./bootimus user set-password admin --password 'nueva-contraseña'
```

### Gestión de usuarios

Se pueden crear usuarios adicionales desde la pestaña **Users** del panel admin. Cada usuario tiene:
- **Username**: Nombre de login único
- **Password**: Almacenada como hash bcrypt
- **Admin**: Si el usuario tiene privilegios de admin
- **Enabled**: Se puede deshabilitar sin borrar

Los usuarios también se pueden gestionar desde la CLI sin arrancar el servidor
(útil para recuperación). Estos comandos operan directamente sobre la base de
datos configurada (SQLite o PostgreSQL):
```bash
./bootimus user list                       # listar todos los usuarios locales
./bootimus user enable <username>          # habilitar una cuenta
./bootimus user disable <username>         # deshabilitar una cuenta
./bootimus user set-admin <username>       # conceder derechos de admin
./bootimus user unset-admin <username>     # revocar derechos de admin
./bootimus user set-password <username>    # establecer una contraseña (solicita, o --password)
```

## Flujo de login

1. Navega a `http://your-server:8081`
2. Se muestra la página de login con campos de usuario y contraseña
3. Si LDAP está configurado, aparece un desplegable de autenticación para elegir el backend
4. Tras un login exitoso, se emite un token JWT (válido 24 horas)
5. El token se almacena en el navegador y se envía con todas las peticiones a la API
6. Al hacer logout o expirar el token, se muestra de nuevo la página de login

## Autenticación de la API

Todos los endpoints de la API (excepto `/api/login` y `/api/auth-info`) requieren un token JWT válido.

### Obtener un token

```bash
# Login and get token
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.data.token')

echo $TOKEN
```

### Usar el token

```bash
# Include in all API requests
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients

# Example: list images
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images
```

### Detalles del token

- **Algoritmo**: HMAC-SHA256
- **Expiración**: 24 horas desde la emisión
- **Secret**: Generado aleatoriamente en cada arranque del servidor (todos los tokens se invalidan al reiniciar)
- **Claims**: Username, estado admin, hora de emisión, expiración

### Comprobar backends de auth disponibles

```bash
# No authentication required
curl http://localhost:8081/api/auth-info
```

Respuesta:
```json
{
  "success": true,
  "data": [
    {"id": "local", "name": "Local"},
    {"id": "ldap", "name": "LDAP (dc.example.com)"}
  ]
}
```

## LDAP / Active Directory

Bootimus soporta autenticación LDAP como backend adicional. Cuando está configurado, los usuarios pueden elegir entre autenticación local y LDAP en la página de login. Las cuentas locales siempre funcionan como fallback.

### Cómo funciona

1. El usuario selecciona "LDAP" en la página de login e introduce las credenciales
2. Bootimus se conecta al servidor LDAP usando la cuenta de servicio (bind DN)
3. Busca al usuario por el filtro configurado
4. Intenta bindear como el usuario encontrado con la contraseña proporcionada
5. Si tiene éxito, comprueba la membresía de grupo para acceso admin
6. Emite un token JWT (igual que en auth local)

### Ejemplo con Active Directory

```bash
# Environment variables
export BOOTIMUS_LDAP_HOST=dc.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="service-account-password"
export BOOTIMUS_LDAP_USER_FILTER="(sAMAccountName=%s)"
export BOOTIMUS_LDAP_GROUP_FILTER="cn=bootimus-admins"
```

### Ejemplo con OpenLDAP

```bash
export BOOTIMUS_LDAP_HOST=ldap.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=readonly,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="readonly-password"
export BOOTIMUS_LDAP_USER_FILTER="(uid=%s)"
```

### LDAPS (TLS)

```bash
export BOOTIMUS_LDAP_HOST=ldaps.example.com
export BOOTIMUS_LDAP_PORT=636
export BOOTIMUS_LDAP_TLS=true

# Or use StartTLS on port 389
export BOOTIMUS_LDAP_HOST=ldap.example.com
export BOOTIMUS_LDAP_STARTTLS=true

# Skip certificate verification (not recommended for production)
export BOOTIMUS_LDAP_SKIP_VERIFY=true
```

### Ejemplo con Docker Compose

```yaml
services:
  bootimus:
    image: garybowers/bootimus:latest
    environment:
      BOOTIMUS_LDAP_HOST: dc.example.com
      BOOTIMUS_LDAP_BASE_DN: dc=example,dc=com
      BOOTIMUS_LDAP_BIND_DN: cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com
      BOOTIMUS_LDAP_BIND_PASSWORD: service-account-password
      BOOTIMUS_LDAP_USER_FILTER: (sAMAccountName=%s)
      BOOTIMUS_LDAP_GROUP_FILTER: cn=bootimus-admins
```

### Membresía de grupo admin

Si `BOOTIMUS_LDAP_GROUP_FILTER` está definido, solo los usuarios miembros del grupo coincidente obtienen acceso admin. La membresía de grupo se comprueba mediante:

1. El atributo `memberOf` en el objeto de usuario
2. Una búsqueda de grupo si `memberOf` no está disponible

Si `BOOTIMUS_LDAP_GROUP_FILTER` **no está definido**, todos los usuarios LDAP obtienen acceso admin.

### Login por API con LDAP

```bash
# Specify auth_method: "ldap"
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"jdoe","password":"ldap-password","auth_method":"ldap"}' | jq -r '.data.token')
```

## Referencia de configuración

### Flags de CLI

| Flag | Default | Descripción |
|------|---------|-------------|
| `--ldap-host` | *(vacío)* | Hostname del servidor LDAP (habilita auth LDAP) |
| `--ldap-port` | `389` | Puerto del servidor LDAP |
| `--ldap-tls` | `false` | Usar LDAPS (TLS al conectar) |
| `--ldap-starttls` | `false` | Usar StartTLS tras conectar |
| `--ldap-skip-verify` | `false` | Saltarse la verificación del certificado TLS |
| `--ldap-bind-dn` | *(vacío)* | DN de la cuenta de servicio para búsqueda de usuarios |
| `--ldap-bind-password` | *(vacío)* | Contraseña de la cuenta de servicio |
| `--ldap-base-dn` | *(vacío)* | Base DN para búsqueda de usuarios |
| `--ldap-user-filter` | `(sAMAccountName=%s)` | Filtro de búsqueda de usuarios (`%s` = username) |
| `--ldap-group-filter` | *(vacío)* | CN del grupo para acceso admin |
| `--ldap-group-base-dn` | *(vacío)* | Base DN para búsqueda de grupos (por defecto, el base DN) |

### Variables de entorno

Todos los flags se pueden definir vía variables de entorno con el prefijo `BOOTIMUS_`:

| Variable | Mapea a |
|----------|---------|
| `BOOTIMUS_LDAP_HOST` | `--ldap-host` |
| `BOOTIMUS_LDAP_PORT` | `--ldap-port` |
| `BOOTIMUS_LDAP_TLS` | `--ldap-tls` |
| `BOOTIMUS_LDAP_STARTTLS` | `--ldap-starttls` |
| `BOOTIMUS_LDAP_SKIP_VERIFY` | `--ldap-skip-verify` |
| `BOOTIMUS_LDAP_BIND_DN` | `--ldap-bind-dn` |
| `BOOTIMUS_LDAP_BIND_PASSWORD` | `--ldap-bind-password` |
| `BOOTIMUS_LDAP_BASE_DN` | `--ldap-base-dn` |
| `BOOTIMUS_LDAP_USER_FILTER` | `--ldap-user-filter` |
| `BOOTIMUS_LDAP_GROUP_FILTER` | `--ldap-group-filter` |
| `BOOTIMUS_LDAP_GROUP_BASE_DN` | `--ldap-group-base-dn` |

### Archivo de configuración (bootimus.yaml)

```yaml
ldap:
  host: dc.example.com
  port: 389
  tls: false
  starttls: true
  bind_dn: cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com
  bind_password: service-account-password
  base_dn: dc=example,dc=com
  user_filter: (sAMAccountName=%s)
  group_filter: cn=bootimus-admins
```

## Solución de problemas

### Falló la conexión LDAP

Revisa conectividad y settings de TLS:
```bash
# Test LDAP connection
ldapsearch -H ldap://dc.example.com -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"

# Test LDAPS
ldapsearch -H ldaps://dc.example.com:636 -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"
```

### Usuario no encontrado

Verifica que el filtro de usuario devuelva resultados:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" dn
```

Filtros comunes:
- Active Directory: `(sAMAccountName=%s)`
- OpenLDAP: `(uid=%s)`
- Basado en email: `(mail=%s)`

### Usuario LDAP no es admin

Comprueba la membresía de grupo:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" memberOf
```

### Token expirado

Los tokens JWT son válidos durante 24 horas. Tras la expiración, la página de login se muestra automáticamente. Los tokens también se invalidan cuando el servidor se reinicia (el secret de firma se regenera).

### Admin local bloqueado

Resetea la contraseña admin:
```bash
./bootimus serve --reset-admin-password
```

O, si arrancar el servidor es incómodo (p. ej. conflictos de puertos), establece
la contraseña directamente en la base de datos:
```bash
./bootimus user set-password admin
```

Esto siempre funciona, independientemente de la configuración LDAP.
