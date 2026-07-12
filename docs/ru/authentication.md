# Руководство по аутентификации

Bootimus использует JWT (JSON Web Token) для аутентификации в админ-панели. По желанию можно подключить LDAP- или Active Directory-сервер как бэкенд аутентификации.

## Оглавление

- [Локальная аутентификация](#локальная-аутентификация)
- [Процесс входа](#процесс-входа)
- [Аутентификация API](#аутентификация-api)
- [LDAP / Active Directory](#ldap--active-directory)
- [Справочник по конфигурации](#справочник-по-конфигурации)
- [Диагностика](#диагностика)

## Локальная аутентификация

По умолчанию Bootimus использует локальные учётные записи, хранящиеся в базе данных (SQLite или PostgreSQL).

### Учётка администратора по умолчанию

При первом запуске генерируется случайный пароль и печатается в логи сервера:

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

### Сброс пароля администратора

```bash
./bootimus serve --reset-admin-password
# или через Docker
docker exec bootimus /bootimus serve --reset-admin-password
```

### Управление пользователями

Дополнительных пользователей можно создать во вкладке **Users** админ-панели. У каждого пользователя есть:
- **Username**: уникальное имя для входа
- **Password**: хранится как bcrypt-хеш
- **Admin**: есть ли у пользователя права администратора
- **Enabled**: можно отключить без удаления

## Процесс входа

1. Откройте `http://your-server:8081`
2. Отображается страница входа с полями имени пользователя и пароля
3. Если настроен LDAP, появляется выпадающий список для выбора бэкенда
4. При успешном входе выдаётся JWT-токен (действует 24 часа)
5. Токен сохраняется в браузере и отправляется со всеми API-запросами
6. При выходе или истечении токена снова показывается страница входа

## Аутентификация API

Все API-эндпоинты (кроме `/api/login` и `/api/auth-info`) требуют действительный JWT-токен.

### Получить токен

```bash
# Логин и получение токена
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.data.token')

echo $TOKEN
```

### Использовать токен

```bash
# Включайте в каждый API-запрос
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients

# Пример: список образов
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images
```

### Детали токена

- **Алгоритм**: HMAC-SHA256
- **Срок жизни**: 24 часа с момента выдачи
- **Секрет**: генерируется случайно при каждом запуске сервера (все токены инвалидируются при перезапуске)
- **Claims**: имя пользователя, статус администратора, время выдачи, срок истечения

### Проверка доступных auth-бэкендов

```bash
# Аутентификация не требуется
curl http://localhost:8081/api/auth-info
```

Ответ:
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

Bootimus поддерживает LDAP как дополнительный бэкенд аутентификации. Когда он настроен, пользователи могут выбрать на странице входа между локальной и LDAP-аутентификацией. Локальные учётки всегда работают как резерв.

### Как это работает

1. Пользователь выбирает «LDAP» на странице входа и вводит креды
2. Bootimus подключается к LDAP-серверу через сервисную учётку (bind DN)
3. Ищет пользователя по заданному фильтру
4. Пытается забиндиться от имени найденного пользователя с указанным паролем
5. При успехе проверяет членство в группе для админ-доступа
6. Выдаёт JWT-токен (так же, как при локальной аутентификации)

### Пример Active Directory

```bash
# Переменные окружения
export BOOTIMUS_LDAP_HOST=dc.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="service-account-password"
export BOOTIMUS_LDAP_USER_FILTER="(sAMAccountName=%s)"
export BOOTIMUS_LDAP_GROUP_FILTER="cn=bootimus-admins"
```

### Пример OpenLDAP

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

# Или StartTLS на порту 389
export BOOTIMUS_LDAP_HOST=ldap.example.com
export BOOTIMUS_LDAP_STARTTLS=true

# Пропустить проверку сертификата (не рекомендуется для продакшена)
export BOOTIMUS_LDAP_SKIP_VERIFY=true
```

### Пример Docker Compose

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

### Членство в админ-группе

Если задан `BOOTIMUS_LDAP_GROUP_FILTER`, права администратора получают только пользователи — члены подходящей группы. Членство проверяется через:

1. Атрибут `memberOf` на объекте пользователя
2. Запрос поиска по группе, если `memberOf` недоступен

Если `BOOTIMUS_LDAP_GROUP_FILTER` **не задан**, все LDAP-пользователи получают права администратора.

### Логин через API с LDAP

```bash
# Укажите auth_method: "ldap"
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"jdoe","password":"ldap-password","auth_method":"ldap"}' | jq -r '.data.token')
```

## Справочник по конфигурации

### Флаги CLI

| Флаг | По умолчанию | Описание |
|------|---------|-------------|
| `--ldap-host` | *(пусто)* | Хост LDAP-сервера (включает LDAP-аутентификацию) |
| `--ldap-port` | `389` | Порт LDAP-сервера |
| `--ldap-tls` | `false` | Использовать LDAPS (TLS при подключении) |
| `--ldap-starttls` | `false` | Использовать StartTLS после подключения |
| `--ldap-skip-verify` | `false` | Пропустить проверку TLS-сертификата |
| `--ldap-bind-dn` | *(пусто)* | DN сервисной учётки для поиска пользователей |
| `--ldap-bind-password` | *(пусто)* | Пароль сервисной учётки |
| `--ldap-base-dn` | *(пусто)* | Base DN для поиска пользователей |
| `--ldap-user-filter` | `(sAMAccountName=%s)` | Фильтр поиска пользователей (`%s` = имя) |
| `--ldap-group-filter` | *(пусто)* | CN группы для админ-доступа |
| `--ldap-group-base-dn` | *(пусто)* | Base DN для поиска групп (по умолчанию — base DN) |

### Переменные окружения

Все флаги задаются через переменные окружения с префиксом `BOOTIMUS_`:

| Переменная | Соответствует |
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

### Файл конфигурации (bootimus.yaml)

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

## Диагностика

### Сбой подключения к LDAP

Проверьте связность и настройки TLS:
```bash
# Тест подключения LDAP
ldapsearch -H ldap://dc.example.com -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"

# Тест LDAPS
ldapsearch -H ldaps://dc.example.com:636 -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"
```

### Пользователь не найден

Проверьте, что фильтр возвращает результаты:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" dn
```

Типичные фильтры:
- Active Directory: `(sAMAccountName=%s)`
- OpenLDAP: `(uid=%s)`
- По email: `(mail=%s)`

### LDAP-пользователь не админ

Проверьте членство в группе:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" memberOf
```

### Токен истёк

JWT-токены действительны 24 часа. После истечения автоматически показывается страница входа. Токены также инвалидируются при перезапуске сервера (секрет подписи генерируется заново).

### Локальный админ заблокирован

Сбросьте пароль администратора:
```bash
./bootimus serve --reset-admin-password
```

Это всегда работает, независимо от конфигурации LDAP.
