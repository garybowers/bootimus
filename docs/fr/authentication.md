# Guide d'authentification

Bootimus utilise l'authentification JWT (JSON Web Token) pour le panneau d'administration. Tu peux également connecter un serveur LDAP ou Active Directory en backend d'authentification.

## Table des matières

- [Authentification locale](#authentification-locale)
- [Flux de connexion](#flux-de-connexion)
- [Authentification de l'API](#authentification-de-lapi)
- [LDAP / Active Directory](#ldap--active-directory)
- [Référence de configuration](#référence-de-configuration)
- [Dépannage](#dépannage)

## Authentification locale

Par défaut, Bootimus utilise des comptes utilisateurs locaux stockés en base (SQLite ou PostgreSQL).

### Compte admin par défaut

Au premier démarrage, un mot de passe aléatoire est généré et affiché dans les logs du serveur :

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

### Réinitialiser le mot de passe admin

Réinitialiser vers un nouveau mot de passe aléatoire (démarre une instance complète du serveur) :
```bash
./bootimus serve --reset-admin-password
# ou avec Docker
docker exec bootimus /bootimus serve --reset-admin-password
```

Ou définir un mot de passe précis directement dans la base de données, sans
ouvrir de port (idéal pour la récupération d'urgence) :
```bash
# Saisie interactive (entrée masquée, garde le mot de passe hors de l'historique du shell)
./bootimus user set-password admin

# Ou le fournir de façon non interactive pour les scripts
./bootimus user set-password admin --password 'nouveau-mot-de-passe'
```

### Gestion des utilisateurs

Des utilisateurs supplémentaires peuvent être créés depuis l'onglet **Users** du panneau d'administration. Chaque utilisateur a :
- **Username** : nom de login unique
- **Password** : stocké en hash bcrypt
- **Admin** : si l'utilisateur a les privilèges admin
- **Enabled** : peut être désactivé sans suppression

Les utilisateurs peuvent aussi être gérés depuis la CLI sans démarrer le serveur
(utile pour la récupération). Ces commandes agissent directement sur la base de
données configurée (SQLite ou PostgreSQL) :
```bash
./bootimus user list                       # lister tous les utilisateurs locaux
./bootimus user enable <username>          # activer un compte
./bootimus user disable <username>         # désactiver un compte
./bootimus user set-admin <username>       # accorder les droits admin
./bootimus user unset-admin <username>     # retirer les droits admin
./bootimus user set-password <username>    # définir un mot de passe (saisie interactive, ou --password)
```

## Flux de connexion

1. Va sur `http://your-server:8081`
2. La page de login s'affiche avec les champs nom d'utilisateur et mot de passe
3. Si LDAP est configuré, un menu déroulant d'authentification apparaît pour sélectionner le backend
4. À la connexion réussie, un token JWT est émis (valable 24 heures)
5. Le token est stocké dans le navigateur et envoyé avec toutes les requêtes API
6. À la déconnexion ou à l'expiration du token, la page de login est réaffichée

## Authentification de l'API

Tous les endpoints de l'API (sauf `/api/login` et `/api/auth-info`) requièrent un token JWT valide.

### Obtenir un token

```bash
# Login and get token
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.data.token')

echo $TOKEN
```

### Utiliser le token

```bash
# Include in all API requests
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients

# Example: list images
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images
```

### Détails du token

- **Algorithme** : HMAC-SHA256
- **Expiration** : 24 heures à partir de l'émission
- **Secret** : généré aléatoirement à chaque démarrage du serveur (tous les tokens sont invalidés au redémarrage)
- **Claims** : nom d'utilisateur, statut admin, heure d'émission, expiration

### Vérifier les backends d'authentification disponibles

```bash
# No authentication required
curl http://localhost:8081/api/auth-info
```

Réponse :
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

Bootimus supporte l'authentification LDAP comme backend additionnel. Une fois configuré, les utilisateurs peuvent choisir entre l'authentification locale et LDAP sur la page de login. Les comptes locaux fonctionnent toujours en fallback.

### Fonctionnement

1. L'utilisateur sélectionne « LDAP » sur la page de login et entre ses identifiants
2. Bootimus se connecte au serveur LDAP avec le compte de service (bind DN)
3. Recherche l'utilisateur avec le filtre configuré
4. Tente un bind en tant qu'utilisateur trouvé avec le mot de passe fourni
5. En cas de succès, vérifie l'appartenance au groupe pour l'accès admin
6. Émet un token JWT (comme pour l'auth locale)

### Exemple Active Directory

```bash
# Environment variables
export BOOTIMUS_LDAP_HOST=dc.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="service-account-password"
export BOOTIMUS_LDAP_USER_FILTER="(sAMAccountName=%s)"
export BOOTIMUS_LDAP_GROUP_FILTER="cn=bootimus-admins"
```

### Exemple OpenLDAP

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

### Exemple Docker Compose

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

### Appartenance au groupe admin

Si `BOOTIMUS_LDAP_GROUP_FILTER` est défini, seuls les utilisateurs membres du groupe correspondant obtiennent l'accès admin. L'appartenance au groupe est vérifiée via :

1. L'attribut `memberOf` sur l'objet utilisateur
2. Une requête de recherche de groupe si `memberOf` n'est pas disponible

Si `BOOTIMUS_LDAP_GROUP_FILTER` n'est **pas défini**, tous les utilisateurs LDAP obtiennent l'accès admin.

### Login via l'API avec LDAP

```bash
# Specify auth_method: "ldap"
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"jdoe","password":"ldap-password","auth_method":"ldap"}' | jq -r '.data.token')
```

## Référence de configuration

### Flags CLI

| Flag | Défaut | Description |
|------|---------|-------------|
| `--ldap-host` | *(vide)* | Hostname du serveur LDAP (active l'auth LDAP) |
| `--ldap-port` | `389` | Port du serveur LDAP |
| `--ldap-tls` | `false` | Utiliser LDAPS (TLS à la connexion) |
| `--ldap-starttls` | `false` | Utiliser StartTLS après la connexion |
| `--ldap-skip-verify` | `false` | Sauter la vérification du certificat TLS |
| `--ldap-bind-dn` | *(vide)* | DN du compte de service pour la recherche utilisateur |
| `--ldap-bind-password` | *(vide)* | Mot de passe du compte de service |
| `--ldap-base-dn` | *(vide)* | Base DN pour la recherche utilisateur |
| `--ldap-user-filter` | `(sAMAccountName=%s)` | Filtre de recherche utilisateur (`%s` = username) |
| `--ldap-group-filter` | *(vide)* | CN du groupe pour l'accès admin |
| `--ldap-group-base-dn` | *(vide)* | Base DN pour la recherche de groupe (par défaut le base DN) |

### Variables d'environnement

Tous les flags peuvent être définis via des variables d'environnement avec le préfixe `BOOTIMUS_` :

| Variable | Correspond à |
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

### Fichier de config (bootimus.yaml)

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

## Dépannage

### Connexion LDAP échouée

Vérifie la connectivité et les paramètres TLS :
```bash
# Test LDAP connection
ldapsearch -H ldap://dc.example.com -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"

# Test LDAPS
ldapsearch -H ldaps://dc.example.com:636 -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"
```

### Utilisateur introuvable

Vérifie que le filtre utilisateur renvoie des résultats :
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" dn
```

Filtres courants :
- Active Directory : `(sAMAccountName=%s)`
- OpenLDAP : `(uid=%s)`
- Basé sur l'email : `(mail=%s)`

### L'utilisateur LDAP n'est pas admin

Vérifie l'appartenance au groupe :
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" memberOf
```

### Token expiré

Les tokens JWT sont valables 24 heures. Après expiration, la page de login s'affiche automatiquement. Les tokens sont aussi invalidés au redémarrage du serveur (le secret de signature est regénéré).

### Admin local verrouillé

Réinitialise le mot de passe admin :
```bash
./bootimus serve --reset-admin-password
```

Ou, si démarrer le serveur est peu pratique (ex. conflits de ports), définir le
mot de passe directement dans la base de données :
```bash
./bootimus user set-password admin
```

Ça marche toujours, quelle que soit la config LDAP.
