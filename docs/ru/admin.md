# Руководство по админ-консоли

Полное руководство по использованию админ-интерфейса Bootimus и REST API.

## Оглавление

- [Доступ к админ-панели](#доступ-к-админ-панели)
- [Дашборд](#дашборд)
- [Управление клиентами](#управление-клиентами)
- [Управление образами](#управление-образами)
- [Логи загрузок](#логи-загрузок)
- [REST API](#rest-api)
- [Примеры автоматизации](#примеры-автоматизации)
- [Безопасность — лучшие практики](#безопасность--лучшие-практики)

## Доступ к админ-панели

### Веб-интерфейс

```
http://your-server:8081/
```

**Требования**:
- Админ-интерфейс работает на отдельном порту (по умолчанию 8081)
- Работает с SQLite или PostgreSQL
- Аутентификация по JWT-токену (с опциональным LDAP/AD-бэкендом)

### Первый вход

При первом запуске Bootimus генерирует случайный пароль администратора:

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

Откройте `http://your-server:8081` — появится отдельная страница входа. Введите учётные данные администратора, чтобы попасть в панель.

**Учётные данные для входа**:
- **Имя пользователя**: `admin`
- **Пароль**: смотрите в логах запуска сервера

Если настроен LDAP, на странице входа появится выпадающий список — можно выбрать между локальной и LDAP-аутентификацией. Подробности — в [руководстве по аутентификации](authentication.md).

### Быстрый старт

1. Запустите Bootimus:
   ```bash
   docker-compose up -d
   # ИЛИ
   ./bootimus serve
   ```

2. Скопируйте пароль администратора из логов сервера

3. Откройте в браузере `http://localhost:8081/`

4. Войдите под именем `admin` со сгенерированным паролем

## Дашборд

Дашборд показывает статистику в реальном времени:

- **Всего клиентов** — все зарегистрированные клиенты
- **Активные клиенты** — включённые клиенты, которым разрешена загрузка
- **Всего образов** — все ISO-образы
- **Включённые образы** — образы, доступные в загрузочном меню
- **Всего загрузок** — количество попыток загрузки

Вся статистика обновляется в реальном времени через WebSocket/SSE.

## Управление клиентами

### Добавить клиента

1. Нажмите кнопку **«Add Client»**
2. Введите MAC-адрес (формат: `00:11:22:33:44:55`)
3. По желанию укажите имя и описание
4. Поставьте галочку **«Enabled»**, чтобы разрешить загрузку
5. Нажмите **«Create Client»**

**Через API**:
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

### Редактировать клиента

1. Нажмите **«Edit»** в строке клиента
2. Измените имя, описание или статус включения
3. Выберите, к каким ISO у этого клиента есть доступ (множественный выбор)
4. Нажмите **«Update Client»**

**Через API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Name",
    "enabled": false
  }'
```

### Удалить клиента

Нажмите **«Delete»** в строке клиента и подтвердите удаление.

**Через API**:
```bash
curl -u admin:password -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

### Назначить образы клиенту

**Через веб-интерфейс**:
1. Нажмите **«Edit»** на клиенте
2. Выберите образы из выпадающего списка
3. Нажмите **«Update Client»**

**Через API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "image_filenames": ["ubuntu-24.04.iso", "debian-12.iso"]
  }'
```

## Управление образами

### Загрузить ISO

**Через веб-интерфейс**:
1. Нажмите кнопку **«Upload ISO»**
2. Перетащите ISO-файл или кликните, чтобы выбрать
3. По желанию добавьте описание
4. Поставьте галочку **«Public»**, чтобы сделать образ доступным всем клиентам
5. Нажмите **«Upload»**

**Лимит загрузки**: 10 ГБ на файл

**Через API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Скачать по URL

Загрузка ISO напрямую на сервер:

**Через веб-интерфейс**:
1. Нажмите кнопку **«Download from URL»**
2. Введите URL загрузки ISO
3. Добавьте описание
4. Нажмите **«Download»**

**Через API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'

# Отслеживание прогресса
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Извлечение kernel/initrd

Извлеките загрузочные файлы для ускоренной загрузки и снижения трафика:

**Через веб-интерфейс**:
1. Найдите образ во вкладке **Images**
2. Нажмите кнопку **«Extract»**
3. Дождитесь завершения извлечения

**Через API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'
```

**Преимущества**:
- Быстрая загрузка (качаем 100 МБ вместо 6 ГБ)
- Меньше трафика (критично при множестве клиентов)
- Лучшая совместимость (некоторые ISO не поддерживают sanboot)

См. [руководство по управлению образами](images.md) для подробностей по извлечению.

### Скачать netboot-файлы

Для установщиков Debian/Ubuntu, требующих netboot:

**Через веб-интерфейс**:
1. Найдите образ с бейджем **«Netboot Required»**
2. Нажмите кнопку **«Download Netboot»**
3. Дождитесь загрузки и извлечения

**Через API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

**Что такое netboot-файлы?**
- Официальные минимальные загрузочные файлы Debian/Ubuntu
- ~30–50 МБ (вместо полного ISO)
- Установщик сам тянет пакеты из интернета во время установки
- Всегда свежие пакеты

См. [поддержку netboot](images.md#netboot-support) для подробностей.

### Сканирование ISO

Сканирование data-директории на наличие добавленных вручную ISO:

**Через веб-интерфейс**:
1. Скопируйте ISO-файлы в директорию `/data/isos/`
2. Нажмите кнопку **«Scan for ISOs»**
3. Bootimus обнаружит и зарегистрирует новые ISO

**Через API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### Включить/выключить образ

**Через веб-интерфейс**:
- Нажмите **«Enable»** или **«Disable»** на любом образе
- Выключенные образы не появятся в загрузочных меню

**Через API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

### Сделать публичным/приватным

**Через веб-интерфейс**:
- Нажмите **«Make Public»**, чтобы открыть доступ всем клиентам
- Нажмите **«Make Private»**, чтобы ограничить доступ только назначенными клиентами

**Через API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": true}'
```

### Удалить образ

**Через веб-интерфейс**:
- Нажмите **«Delete»** в строке образа
- Подтвердите удаление
- Образ удалится из базы данных
- ISO-файл остаётся на диске (удалите вручную, если нужно)

**Через API**:
```bash
# Удалить только из базы
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso"

# Удалить из базы и с диска
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso&delete_file=true"
```

## Логи загрузок

Просмотр недавних попыток загрузки с потоковым обновлением:

**Что показывается**:
- Метка времени
- MAC-адрес клиента
- Имя образа
- IP-адрес
- Статус успех/ошибка
- Сообщения об ошибках (если есть)

**Авто-обновление**: логи обновляются в реальном времени через SSE (Server-Sent Events)

**Через API**:
```bash
# Последние 100 логов (по умолчанию)
curl -u admin:password http://localhost:8081/api/logs

# Последние 10 логов
curl -u admin:password http://localhost:8081/api/logs?limit=10

# Последние 500 логов (макс. 1000)
curl -u admin:password http://localhost:8081/api/logs?limit=500
```

## REST API

Все функции админ-панели доступны через REST API для автоматизации.

### Аутентификация

Для всех эндпоинтов требуется HTTP Basic Authentication:
- **Имя пользователя**: `admin`
- **Пароль**: автогенерируется при первом запуске

```bash
curl -u admin:your-password http://localhost:8081/api/stats
```

### Эндпоинты API

#### Stats

```bash
GET /api/stats
```

**Ответ**:
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

| Метод | Эндпоинт | Описание |
|--------|----------|-------------|
| `GET` | `/api/clients` | Список всех клиентов |
| `GET` | `/api/clients?mac=<MAC>` | Клиент по MAC |
| `POST` | `/api/clients` | Создать клиента |
| `PUT` | `/api/clients?mac=<MAC>` | Обновить клиента |
| `DELETE` | `/api/clients?mac=<MAC>` | Удалить клиента |
| `POST` | `/api/clients/assign` | Назначить образы клиенту |

#### Images

| Метод | Эндпоинт | Описание |
|--------|----------|-------------|
| `GET` | `/api/images` | Список всех образов |
| `GET` | `/api/images?filename=<name>` | Получить образ |
| `PUT` | `/api/images?filename=<name>` | Обновить образ |
| `DELETE` | `/api/images?filename=<name>` | Удалить образ |
| `POST` | `/api/images/upload` | Загрузить ISO |
| `POST` | `/api/images/download` | Скачать ISO по URL |
| `POST` | `/api/images/extract` | Извлечь kernel/initrd |
| `POST` | `/api/images/netboot/download` | Скачать netboot-файлы |
| `POST` | `/api/scan` | Сканировать новые ISO |

#### Downloads

| Метод | Эндпоинт | Описание |
|--------|----------|-------------|
| `GET` | `/api/downloads` | Активные загрузки |
| `GET` | `/api/downloads/progress?filename=<name>` | Прогресс загрузки |

#### Logs

| Метод | Эндпоинт | Описание |
|--------|----------|-------------|
| `GET` | `/api/logs?limit=<N>` | Получить логи загрузок |
| `GET` | `/api/logs/stream` | SSE-поток логов в реальном времени |

## Примеры автоматизации

### Массовое добавление клиентов

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

### Сделать все образы публичными

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

### Мониторинг попыток загрузки

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

### Экспорт статистики

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

## Безопасность — лучшие практики

### Изоляция сети

Держите админ-порт отдельно от сети загрузки:

```bash
# Разрешить трафик загрузки (TFTP/HTTP) на одном интерфейсе
# Разрешить админ-трафик на другом интерфейсе или только на localhost
```

### Правила файрвола

```bash
# Разрешить админ-доступ только из конкретного диапазона IP
sudo ufw allow from 192.168.1.0/24 to any port 8081

# Или вовсе закрыть админ-порт извне
sudo ufw deny 8081
```

### SSH-туннель

Безопасный доступ к админ-интерфейсу через SSH-туннель:

```bash
# Создать SSH-туннель
ssh -L 8081:localhost:8081 user@bootimus-server

# Открыть админ-панель
open http://localhost:8081/
```

### VPN-доступ

- Разместите админ-порт Bootimus только в VPN-сети
- Требуйте VPN для доступа к админке
- Порты загрузки (69, 8080) держите в отдельном сегменте

### Управление паролями

- Храните пароль администратора надёжно (менеджер паролей)
- Периодически меняйте пароль, удалив `.admin_password` и перезапустив сервер
- Рассмотрите дополнительный слой аутентификации (nginx с клиентскими сертификатами)

## Диагностика

### Админ-интерфейс не открывается

```bash
# Проверьте, что сервис запущен
docker ps | grep bootimus

# Проверьте логи
docker logs bootimus

# Проверьте доступность порта
curl -u admin:password http://localhost:8081/api/stats

# Проверьте файрвол
sudo ufw status | grep 8081
```

### Не получается загрузить большие ISO

```bash
# Проверьте свободное место
df -h /opt/bootimus/data

# По умолчанию лимит загрузки — 10 ГБ
# Для больших ISO используйте загрузку по URL или ручное копирование + сканирование
```

### Изменения не отображаются

- Жёсткое обновление страницы (Ctrl+F5 или Cmd+Shift+R)
- Проверьте консоль браузера на ошибки (F12)
- Проверьте ответы API через curl
- Проверьте логи сервера на детальные ошибки

### API возвращает ошибки

```bash
# Проверьте формат запроса (JSON content-type для POST/PUT)
curl -v -u admin:password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","name":"Test"}'

# Убедитесь, что ресурс существует для update/delete
curl -u admin:password http://localhost:8081/api/images | jq

# Проверьте логи сервера
docker logs bootimus | tail -50
```

## Дальше

- Прочитайте [руководство по управлению образами](images.md) — как работать с ISO
- См. [руководство по развёртыванию](deployment.md) для продакшен-настройки
- Настройте [DHCP-сервер](dhcp.md) для PXE-загрузки
- Настройте [управление клиентами](clients.md) для контроля доступа
