# Authentication Guide

Bootimus uses JWT (JSON Web Token) authentication for the admin panel. Optionally, you can connect an LDAP or Active Directory server as an authentication backend.

## Table of Contents

- [Local Authentication](#local-authentication)
- [Login Flow](#login-flow)
- [API Authentication](#api-authentication)
- [LDAP / Active Directory](#ldap--active-directory)
- [Configuration Reference](#configuration-reference)
- [Troubleshooting](#troubleshooting)

## Local Authentication

By default, Bootimus uses local user accounts stored in the database (SQLite or PostgreSQL).

### Default Admin Account

On first startup, a random password is generated and printed to the server logs:

```
╔════════════════════════════════════════════════════════════════╗
║                    ADMIN PASSWORD GENERATED                    ║
╠════════════════════════════════════════════════════════════════╣
║  Username: admin                                                ║
║  Password: AbCdEfGh1234567890-_XyZ123456                       ║
╠════════════════════════════════════════════════════════════════╣
║  This password will NOT be shown again!                        ║
║  Save it now or reset it using --reset-admin-password flag    ║
╚════════════════════════════════════════════════════════════════╝
```

### Reset Admin Password

```bash
./bootimus serve --reset-admin-password
# or with Docker
docker exec bootimus /bootimus serve --reset-admin-password
```

### User Management

Additional users can be created from the **Users** tab in the admin panel. Each user has:
- **Username**: Unique login name
- **Password**: Stored as bcrypt hash
- **Admin**: Whether the user has admin privileges
- **Enabled**: Can be disabled without deletion

## Login Flow

1. Navigate to `http://your-server:8081`
2. The login page is displayed with username and password fields
3. If LDAP is configured, an authentication dropdown appears to select the backend
4. On successful login, a JWT token is issued (valid for 24 hours)
5. The token is stored in the browser and sent with all API requests
6. On logout or token expiry, the login page is shown again

## API Authentication

All API endpoints (except `/api/login` and `/api/auth-info`) require a valid JWT token.

### Obtain a Token

```bash
# Login and get token
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.data.token')

echo $TOKEN
```

### Use the Token

```bash
# Include in all API requests
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients

# Example: list images
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images
```

### Token Details

- **Algorithm**: HMAC-SHA256
- **Expiry**: 24 hours from issue
- **Secret**: Randomly generated on each server startup (all tokens are invalidated on restart)
- **Claims**: Username, admin status, issue time, expiry

### Check Available Auth Backends

```bash
# No authentication required
curl http://localhost:8081/api/auth-info
```

Response:
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

Bootimus supports LDAP authentication as an additional backend. When configured, users can choose between local and LDAP authentication on the login page. Local accounts always work as a fallback.

### How It Works

1. User selects "LDAP" on the login page and enters credentials
2. Bootimus connects to the LDAP server using the service account (bind DN)
3. Searches for the user by the configured filter
4. Attempts to bind as the found user with the provided password
5. If successful, checks group membership for admin access
6. Issues a JWT token (same as local auth)

### Active Directory Example

```bash
# Environment variables
export BOOTIMUS_LDAP_HOST=dc.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="service-account-password"
export BOOTIMUS_LDAP_USER_FILTER="(sAMAccountName=%s)"
export BOOTIMUS_LDAP_GROUP_FILTER="cn=bootimus-admins"
```

### OpenLDAP Example

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

### Docker Compose Example

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

### Admin Group Membership

If `BOOTIMUS_LDAP_GROUP_FILTER` is set, only users who are members of the matching group are granted admin access. Group membership is checked via:

1. The `memberOf` attribute on the user object
2. A group search query if `memberOf` is not available

If `BOOTIMUS_LDAP_GROUP_FILTER` is **not set**, all LDAP users are granted admin access.

### Login via API with LDAP

```bash
# Specify auth_method: "ldap"
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"jdoe","password":"ldap-password","auth_method":"ldap"}' | jq -r '.data.token')
```

## Configuration Reference

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--ldap-host` | *(empty)* | LDAP server hostname (enables LDAP auth) |
| `--ldap-port` | `389` | LDAP server port |
| `--ldap-tls` | `false` | Use LDAPS (TLS on connect) |
| `--ldap-starttls` | `false` | Use StartTLS after connect |
| `--ldap-skip-verify` | `false` | Skip TLS certificate verification |
| `--ldap-bind-dn` | *(empty)* | Service account DN for user search |
| `--ldap-bind-password` | *(empty)* | Service account password |
| `--ldap-base-dn` | *(empty)* | Base DN for user search |
| `--ldap-user-filter` | `(sAMAccountName=%s)` | User search filter (`%s` = username) |
| `--ldap-group-filter` | *(empty)* | Group CN for admin access |
| `--ldap-group-base-dn` | *(empty)* | Base DN for group search (defaults to base DN) |

### Environment Variables

All flags can be set via environment variables with the `BOOTIMUS_` prefix:

| Variable | Maps to |
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

### Config File (bootimus.yaml)

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

## Troubleshooting

### LDAP Connection Failed

Check connectivity and TLS settings:
```bash
# Test LDAP connection
ldapsearch -H ldap://dc.example.com -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"

# Test LDAPS
ldapsearch -H ldaps://dc.example.com:636 -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"
```

### User Not Found

Verify the user filter returns results:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" dn
```

Common filters:
- Active Directory: `(sAMAccountName=%s)`
- OpenLDAP: `(uid=%s)`
- Email-based: `(mail=%s)`

### LDAP User Not Admin

Check group membership:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" memberOf
```

### Token Expired

JWT tokens are valid for 24 hours. After expiry, the login page is shown automatically. Tokens are also invalidated when the server restarts (the signing secret is regenerated).

### Local Admin Locked Out

Reset the admin password:
```bash
./bootimus serve --reset-admin-password
```

This always works regardless of LDAP configuration.
