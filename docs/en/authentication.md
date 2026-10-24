# Authentication Guide

Bootimus uses JWT (JSON Web Token) authentication for the admin panel. Optionally, you can connect an LDAP or Active Directory server, or an OpenID Connect (OIDC) identity provider such as Authentik or Keycloak for single sign-on.

## Table of Contents

- [Local Authentication](#local-authentication)
- [Login Flow](#login-flow)
- [Roles](#roles)
- [API Authentication](#api-authentication)
- [LDAP / Active Directory](#ldap--active-directory)
- [OIDC / Single Sign-On](#oidc--single-sign-on)
- [Users from LDAP and OIDC](#users-from-ldap-and-oidc)
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
║  Username: admin                                               ║
║  Password: AbCdEfGh1234567890-_XyZ123456                       ║
╠════════════════════════════════════════════════════════════════╣
║  This password will NOT be shown again!                        ║
║  Save it now or reset it using --reset-admin-password flag     ║
╚════════════════════════════════════════════════════════════════╝
```

### Reset Admin Password

Reset to a new random password (starts a full server instance):
```bash
./bootimus serve --reset-admin-password
# or with Docker
docker exec bootimus /bootimus serve --reset-admin-password
```

Or set a specific password directly against the database, without binding any
ports (ideal for emergency recovery):
```bash
# Prompt interactively (hidden input, keeps the password out of shell history)
./bootimus user set-password admin

# Or supply it non-interactively for scripting
./bootimus user set-password admin --password 'new-password'
```

### User Management

Additional users can be created from the **Users** tab in the admin panel. Each user has:
- **Username**: Unique login name
- **Password**: Stored as bcrypt hash
- **Admin**: Whether the user is an administrator; other users are read-only (see [Roles](#roles))
- **Enabled**: Can be disabled without deletion
- **Source**: `Local`, `LDAP` or `OIDC` — where the account comes from (see [Users from LDAP and OIDC](#users-from-ldap-and-oidc))

Users can also be managed from the CLI without starting the server (useful for
recovery). These commands operate directly on the configured database
(SQLite or PostgreSQL):
```bash
./bootimus user list                       # list all users and their source
./bootimus user enable <username>          # enable an account
./bootimus user disable <username>         # disable an account
./bootimus user set-admin <username>       # grant admin rights
./bootimus user unset-admin <username>     # revoke admin rights
./bootimus user set-password <username>    # set a password (prompts, or --password)
```

## Login Flow

1. Navigate to `http://your-server:8081`
2. The login page is displayed with username and password fields
3. If LDAP is configured, an authentication dropdown appears to select the backend
4. If OIDC is configured, a **Sign in with …** button appears below the form
5. On successful login, a JWT token is issued (valid for 24 hours)
6. The token is stored in the browser and sent with all API requests
7. On logout or token expiry, the login page is shown again

## Roles

Bootimus has two roles:

| Role | Access |
|------|--------|
| **Admin** | Everything. |
| **Read-only** | Can view the dashboard, clients (including hardware inventory, boot history and BMC power status), client groups, images, public files, distro profiles, the boot menu, bootloaders, tools, Kubernetes clusters and nodes, scheduled tasks and the boot attempt history. |

Read-only users:

- cannot change anything — the server refuses every write with `403 Your account has read-only access`, and the admin panel hides the buttons
- cannot see users, webhook settings, backups, the live server log, auto-install files and scripts, or cluster machine configs
- never receive BMC (Redfish/Intel AMT) passwords or client config patches — these are removed from every API response sent to them

A user is an administrator if:

- **Local**: the **Administrator** box is ticked on their account
- **LDAP**: they match `BOOTIMUS_LDAP_GROUP_FILTER` (or no group filter is set)
- **OIDC**: they are in one of `BOOTIMUS_OIDC_ADMIN_GROUPS` or listed in `BOOTIMUS_OIDC_ADMIN_USERS`

Everyone else who can sign in is read-only. `GET /api/me` returns the signed-in user's role.

> **Upgrading:** before read-only access was added, local users without the **Administrator** box ticked could sign in but see nothing. They now have read-only access. Disable any such accounts you don't want to have it.

## API Authentication

All API endpoints (except `/api/login`, `/api/auth-info` and the `/api/oidc/` sign-in endpoints) require a valid JWT token. Read-only users can only make `GET` requests, and only to the endpoints listed under [Roles](#roles).

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
    {"id": "ldap", "name": "LDAP (dc.example.com)"},
    {"id": "oidc", "name": "Authentik", "type": "redirect", "url": "/api/oidc/login"}
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
6. Creates or updates a local user record for them (see [Users from LDAP and OIDC](#users-from-ldap-and-oidc))
7. Issues a JWT token (same as local auth)

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

Admin rights are re-evaluated at every login, so removing someone from the group demotes them the next time they sign in.

### Login via API with LDAP

```bash
# Specify auth_method: "ldap"
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"jdoe","password":"ldap-password","auth_method":"ldap"}' | jq -r '.data.token')
```

## OIDC / Single Sign-On

Bootimus can sign users in through any OpenID Connect provider — Authentik, Keycloak, Zitadel, Authelia, Entra ID, Google Workspace and others. Users click **Sign in with …** on the login page, authenticate at the provider, and are sent back to Bootimus already signed in. Accounts are created automatically on first sign-in, and admin rights follow the user's groups at the provider.

### How It Works

1. The user clicks **Sign in with …** on the login page
2. Bootimus redirects them to the provider (authorisation code flow with PKCE)
3. The user signs in at the provider, which redirects back to `/api/oidc/callback`
4. Bootimus exchanges the code for an ID token, verifies its signature, audience, expiry and nonce, and reads the username and groups claims
5. If allowed groups or users are configured, the user must match one of them (or be an administrator)
6. A local user record is created or updated (see [Users from LDAP and OIDC](#users-from-ldap-and-oidc)), as an administrator if they are in an admin group or on the admin user list, and read-only otherwise
7. The browser is returned to the admin panel with a JWT token (same as local auth)

If the ID token doesn't include the username or groups claim, Bootimus also asks the provider's userinfo endpoint.

### Provider Setup

Create an OIDC client (sometimes called an application or relying party) at your provider with:

| Setting | Value |
|---------|-------|
| Client type | Confidential (with a client secret) |
| Grant type | Authorisation code |
| Redirect URI | `https://bootimus.example.com/api/oidc/callback` — the URL users reach the admin panel on, plus `/api/oidc/callback` |
| Scopes | `openid`, `profile`, `email` |

Make sure the provider includes the user's groups in a `groups` claim. Bootimus doesn't guess its own public URL, so `BOOTIMUS_OIDC_REDIRECT_URL` must match the redirect URI registered at the provider exactly.

### Authentik Example

1. In Authentik, go to **Applications → Providers** and create an **OAuth2/OpenID Provider**:
   - **Name**: `bootimus`
   - **Authorisation flow**: your usual explicit or implicit consent flow
   - **Client type**: Confidential
   - **Redirect URIs/Origins**: `https://bootimus.example.com/api/oidc/callback` (strict)
   - **Scopes**: keep the default `openid`, `email` and `profile` mappings — Authentik's `profile` mapping includes the `groups` claim
   - **Subject mode**: any; Bootimus uses `preferred_username` as the username by default
2. Note the **Client ID** and **Client Secret**
3. Go to **Applications → Applications** and create an application with slug `bootimus` using that provider. Optionally bind a policy or group to it to control who can launch it at all.
4. Create groups for Bootimus administrators (e.g. `bootimus-admins`) and, optionally, other permitted users (e.g. `bootimus-operators`)
5. Configure Bootimus:

```bash
export BOOTIMUS_OIDC_ISSUER=https://authentik.example.com/application/o/bootimus/
export BOOTIMUS_OIDC_CLIENT_ID=<client id>
export BOOTIMUS_OIDC_CLIENT_SECRET=<client secret>
export BOOTIMUS_OIDC_REDIRECT_URL=https://bootimus.example.com/api/oidc/callback
export BOOTIMUS_OIDC_ADMIN_GROUPS=bootimus-admins
export BOOTIMUS_OIDC_ALLOWED_GROUPS=bootimus-operators
export BOOTIMUS_OIDC_DISPLAY_NAME=Authentik
```

The issuer URL is shown on the provider's overview page in Authentik as **OpenID Configuration Issuer**, and always ends in `/application/o/<application slug>/`.

### Keycloak Example

Create a confidential OpenID Connect client with **Standard flow** enabled and the redirect URI above. Keycloak doesn't send groups by default: add a **Group Membership** mapper to the client's dedicated scope with token claim name `groups` and **Full group path** turned off.

```bash
export BOOTIMUS_OIDC_ISSUER=https://keycloak.example.com/realms/example
export BOOTIMUS_OIDC_CLIENT_ID=bootimus
export BOOTIMUS_OIDC_CLIENT_SECRET=<client secret>
export BOOTIMUS_OIDC_REDIRECT_URL=https://bootimus.example.com/api/oidc/callback
export BOOTIMUS_OIDC_ADMIN_GROUPS=bootimus-admins
export BOOTIMUS_OIDC_DISPLAY_NAME=Keycloak
```

### Microsoft Entra ID Example

Entra ID sends group *IDs* rather than names in its groups claim, so the easiest way to map roles is with **app roles**, which Entra sends in a `roles` claim.

1. In the [Microsoft Entra admin centre](https://entra.microsoft.com), go to **App registrations → New registration**:
   - **Name**: `Bootimus`
   - **Supported account types**: *Accounts in this organisational directory only (Single tenant)*
   - **Redirect URI**: platform **Web**, `https://bootimus.example.com/api/oidc/callback`
2. Note the **Application (client) ID** and **Directory (tenant) ID** from the overview page
3. Under **Certificates & secrets**, create a **New client secret** and copy its **Value**
4. Under **App roles**, create two roles, each with **Allowed member types** *Users/Groups*:
   - Display name `Bootimus Admin`, value `Bootimus.Admin`
   - Display name `Bootimus Read-only`, value `Bootimus.ReadOnly`
5. Go to **Enterprise applications → Bootimus → Users and groups** and assign users or groups to the roles. Assigning groups requires Entra ID P1 or P2; on the free tier, assign users individually.
6. Optionally, under **Enterprise applications → Bootimus → Properties**, set **Assignment required?** to **Yes** so only assigned users can sign in at all
7. Configure Bootimus:

```bash
export BOOTIMUS_OIDC_ISSUER=https://login.microsoftonline.com/<tenant id>/v2.0
export BOOTIMUS_OIDC_CLIENT_ID=<application id>
export BOOTIMUS_OIDC_CLIENT_SECRET=<client secret value>
export BOOTIMUS_OIDC_REDIRECT_URL=https://bootimus.example.com/api/oidc/callback
export BOOTIMUS_OIDC_GROUPS_CLAIM=roles
export BOOTIMUS_OIDC_ADMIN_GROUPS=Bootimus.Admin
export BOOTIMUS_OIDC_ALLOWED_GROUPS=Bootimus.ReadOnly
export BOOTIMUS_OIDC_DISPLAY_NAME=Microsoft
```

Keep the default username claim, `preferred_username`, which is the user's sign-in name (e.g. `alice@example.com`). Don't use `email`: Entra doesn't verify it, and Bootimus refuses email usernames the provider hasn't marked as verified.

If you'd rather use security groups, add a **groups claim** under **Token configuration** and list the groups' **Object IDs** in `BOOTIMUS_OIDC_ADMIN_GROUPS` and `BOOTIMUS_OIDC_ALLOWED_GROUPS`, leaving `BOOTIMUS_OIDC_GROUPS_CLAIM` as `groups`.

### Google Example

Google doesn't send group membership or a `preferred_username` claim, so with Google you use email addresses as usernames and grant access with user and domain lists instead of groups.

1. In the [Google Cloud console](https://console.cloud.google.com), select or create a project and go to **APIs & Services → OAuth consent screen** (called **Google Auth Platform** in newer versions of the console):
   - **User type**: *Internal* if you use Google Workspace, so only accounts in your organisation can sign in. *External* allows any Google account, so always set `BOOTIMUS_OIDC_ALLOWED_USERS` in that case.
   - Add the `openid`, `email` and `profile` scopes
2. Go to **APIs & Services → Credentials → Create credentials → OAuth client ID**:
   - **Application type**: Web application
   - **Authorised redirect URIs**: `https://bootimus.example.com/api/oidc/callback` (Google only accepts HTTPS, apart from `http://localhost`)
3. Note the **Client ID** and **Client secret**
4. Configure Bootimus:

```bash
export BOOTIMUS_OIDC_ISSUER=https://accounts.google.com
export BOOTIMUS_OIDC_CLIENT_ID=<client id>.apps.googleusercontent.com
export BOOTIMUS_OIDC_CLIENT_SECRET=<client secret>
export BOOTIMUS_OIDC_REDIRECT_URL=https://bootimus.example.com/api/oidc/callback
export BOOTIMUS_OIDC_USERNAME_CLAIM=email
export BOOTIMUS_OIDC_ADMIN_USERS=alice@example.com,bob@example.com
export BOOTIMUS_OIDC_ALLOWED_USERS=@example.com
export BOOTIMUS_OIDC_DISPLAY_NAME=Google
```

Here Alice and Bob are administrators and everyone else with an `@example.com` address is read-only. Bootimus only accepts email usernames that Google marks as verified.

### Access Mapping

| Setting | Effect |
|---------|--------|
| `BOOTIMUS_OIDC_ADMIN_GROUPS` | Members of any of these groups are administrators. |
| `BOOTIMUS_OIDC_ADMIN_USERS` | These users are administrators. Entries are usernames, or `@domain` to match every username ending in that domain. |
| `BOOTIMUS_OIDC_ALLOWED_GROUPS` | If set, members of these groups can sign in as read-only users. |
| `BOOTIMUS_OIDC_ALLOWED_USERS` | If set, these users (usernames or `@domain` entries) can sign in as read-only users. |

If neither allowed list is set, **anyone the provider authenticates can sign in with read-only access** — Bootimus logs a warning at startup when that's the case. Once either allowed list is set, only users who match an allowed or admin list can sign in. If neither admin list is set, no OIDC user is an administrator.

All four take a comma-separated list, so group names can contain spaces (`Bootimus Admins,Infrastructure`). Matching ignores case; an `@domain` entry matches that exact domain only, not its subdomains. Groups and lists are re-checked at every sign-in, so changes apply the next time the user signs in.

### Docker Compose Example

```yaml
services:
  bootimus:
    image: garybowers/bootimus:latest
    environment:
      BOOTIMUS_OIDC_ISSUER: https://authentik.example.com/application/o/bootimus/
      BOOTIMUS_OIDC_CLIENT_ID: bootimus
      BOOTIMUS_OIDC_CLIENT_SECRET: client-secret
      BOOTIMUS_OIDC_REDIRECT_URL: https://bootimus.example.com/api/oidc/callback
      BOOTIMUS_OIDC_ADMIN_GROUPS: bootimus-admins
      BOOTIMUS_OIDC_DISPLAY_NAME: Authentik
```

### Security Notes

- The sign-in state, nonce and PKCE verifier are kept in a signed, short-lived (10 minute) `HttpOnly` cookie, scoped to `/api/oidc/`. It is marked `Secure` when the redirect URL uses HTTPS — serve the admin panel over HTTPS in production.
- Only the authorisation code flow is used; tokens from the provider never reach the browser. The browser receives the usual Bootimus JWT.
- Local password login keeps working alongside OIDC, so the local `admin` account remains available for recovery.

## Users from LDAP and OIDC

Users who sign in via LDAP or OIDC get a local user record the first time they sign in. This lets you see them under **Users**, disable them, and have their access checked on every request like local users.

- **Source**: each user is marked `Local`, `LDAP` or `OIDC`. A user can only sign in through their own source: an OIDC or LDAP sign-in will never take over a local account with the same name (such as `admin`), and LDAP and OIDC users cannot use local password login.
- **Usernames** are lower-cased, and may contain letters, digits and `. _ @ + -`. Sign-ins with other characters are refused.
- **Admin rights** follow the user's groups (and, for OIDC, the admin user list) and are refreshed at each sign-in, so changing the **Administrator** tick box for an LDAP or OIDC user only lasts until they next sign in. Users who aren't administrators are read-only (see [Roles](#roles)).
- **Disabling** a user in Bootimus blocks them immediately, even if the provider still lets them sign in.
- **Deleting** a user only removes the local record; it is recreated at their next sign-in. Use the provider's groups, or disable the user, to remove access.
- **Passwords** for LDAP and OIDC users are managed by the provider, so **Reset Password** is not offered for them and `bootimus user set-password` refuses them.

If someone signs in with a name that already belongs to a user from another source, the login is refused with *a user with this name already exists from a different source*. This includes local accounts created before LDAP users were provisioned automatically. Delete the old local account so it can be recreated from LDAP or OIDC at the next sign-in.

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
| `--oidc-issuer` | *(empty)* | OIDC issuer URL (enables OIDC sign-in) |
| `--oidc-client-id` | *(empty)* | OIDC client ID |
| `--oidc-client-secret` | *(empty)* | OIDC client secret |
| `--oidc-redirect-url` | *(empty)* | Redirect URL registered at the provider, ending `/api/oidc/callback` |
| `--oidc-scopes` | `openid,profile,email` | Comma-separated scopes to request (`openid` is always included) |
| `--oidc-username-claim` | `preferred_username` | Claim used as the Bootimus username (e.g. `email`) |
| `--oidc-groups-claim` | `groups` | Claim listing the user's groups |
| `--oidc-admin-groups` | *(empty)* | Comma-separated groups whose members are administrators |
| `--oidc-allowed-groups` | *(empty)* | Comma-separated groups allowed to sign in |
| `--oidc-admin-users` | *(empty)* | Comma-separated usernames or `@domain` entries who are administrators |
| `--oidc-allowed-users` | *(empty)* | Comma-separated usernames or `@domain` entries allowed to sign in |
| `--oidc-display-name` | `SSO` | Name shown on the sign-in button |
| `--oidc-skip-verify` | `false` | Skip TLS certificate verification when talking to the provider |

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
| `BOOTIMUS_OIDC_ISSUER` | `--oidc-issuer` |
| `BOOTIMUS_OIDC_CLIENT_ID` | `--oidc-client-id` |
| `BOOTIMUS_OIDC_CLIENT_SECRET` | `--oidc-client-secret` |
| `BOOTIMUS_OIDC_REDIRECT_URL` | `--oidc-redirect-url` |
| `BOOTIMUS_OIDC_SCOPES` | `--oidc-scopes` |
| `BOOTIMUS_OIDC_USERNAME_CLAIM` | `--oidc-username-claim` |
| `BOOTIMUS_OIDC_GROUPS_CLAIM` | `--oidc-groups-claim` |
| `BOOTIMUS_OIDC_ADMIN_GROUPS` | `--oidc-admin-groups` |
| `BOOTIMUS_OIDC_ALLOWED_GROUPS` | `--oidc-allowed-groups` |
| `BOOTIMUS_OIDC_ADMIN_USERS` | `--oidc-admin-users` |
| `BOOTIMUS_OIDC_ALLOWED_USERS` | `--oidc-allowed-users` |
| `BOOTIMUS_OIDC_DISPLAY_NAME` | `--oidc-display-name` |
| `BOOTIMUS_OIDC_SKIP_VERIFY` | `--oidc-skip-verify` |

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

oidc:
  issuer: https://authentik.example.com/application/o/bootimus/
  client_id: bootimus
  client_secret: client-secret
  redirect_url: https://bootimus.example.com/api/oidc/callback
  admin_groups: bootimus-admins
  allowed_groups: bootimus-operators
  admin_users: alice@example.com
  allowed_users: "@example.com"
  display_name: Authentik
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

### OIDC: "Unable to reach the identity provider"

Bootimus fetches `<issuer>/.well-known/openid-configuration` when the first user signs in. Check the server log for the exact error, and that the Bootimus host can reach the issuer:

```bash
curl https://authentik.example.com/application/o/bootimus/.well-known/openid-configuration
```

The `issuer` value in that document must match `BOOTIMUS_OIDC_ISSUER` exactly, including any trailing slash. If the provider uses a private CA, add it to the host's trust store; `BOOTIMUS_OIDC_SKIP_VERIFY=true` works for testing but is not recommended.

### OIDC: Provider Shows a Redirect URI Error

The redirect URI registered at the provider and `BOOTIMUS_OIDC_REDIRECT_URL` must match exactly — scheme, host, port and path.

### OIDC: "Sign-in expired or was started elsewhere"

The sign-in must finish within 10 minutes, in the same browser that started it, and Bootimus must not have restarted in between. If it keeps happening behind a reverse proxy, make sure the proxy passes cookies through and that users reach Bootimus on the same host as the redirect URL.

### OIDC: User Is Not an Administrator or Is Refused

The server log records why each sign-in was refused. Check which groups the provider actually sends — group names must match `BOOTIMUS_OIDC_ADMIN_GROUPS` or `BOOTIMUS_OIDC_ALLOWED_GROUPS`, and Keycloak sends full paths (`/bootimus-admins`) unless **Full group path** is turned off. If your provider uses a different claim name, set `BOOTIMUS_OIDC_GROUPS_CLAIM` (Entra ID app roles use `roles`). For providers without groups, such as Google, use `BOOTIMUS_OIDC_ADMIN_USERS` and `BOOTIMUS_OIDC_ALLOWED_USERS`.

### OIDC: "email … is not marked as verified by the provider"

When `BOOTIMUS_OIDC_USERNAME_CLAIM` is `email`, the provider must send `email_verified: true`, otherwise anyone able to set an unverified address could sign in as someone else. Verify the address at the provider, or use a different username claim such as `preferred_username`.

### "A user with this name already exists from a different source"

See [Users from LDAP and OIDC](#users-from-ldap-and-oidc). Delete the existing account under **Users** and sign in again.

### Token Expired

JWT tokens are valid for 24 hours. After expiry, the login page is shown automatically. Tokens are also invalidated when the server restarts (the signing secret is regenerated).

### Local Admin Locked Out

Reset the admin password:
```bash
./bootimus serve --reset-admin-password
```

Or, if starting the server is inconvenient (e.g. port conflicts), set the
password directly against the database:
```bash
./bootimus user set-password admin
```

This always works regardless of LDAP or OIDC configuration.
