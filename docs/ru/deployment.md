# Руководство по развёртыванию

Полное руководство по развёртыванию Bootimus в различных окружениях с настройкой сети и хранилища.

## Оглавление

- [Быстрый старт](#быстрый-старт)
- [Развёртывание через Docker](#развёртывание-через-docker)
- [Развёртывание из бинарника](#развёртывание-из-бинарника)
- [Настройка сети](#настройка-сети)
- [Настройка хранилища](#настройка-хранилища)
- [Варианты базы данных](#варианты-базы-данных)
- [Удалённые обновления и приватность](#удалённые-обновления-и-приватность)
- [Production-развёртывание](#production-развёртывание)

## Быстрый старт

### Docker (рекомендуется)

```bash
# Создать data-директорию
mkdir -p data

# Запуск с SQLite (контейнер с базой не нужен)
docker run -d \
  --name bootimus \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -p 8081:8081/tcp \
  -v $(pwd)/data:/data \
  garybowers/bootimus:latest

# Посмотреть пароль администратора в логах
docker logs bootimus | grep "Password"

# Открыть админ-интерфейс
open http://localhost:8081
```

### Standalone-бинарник

```bash
# Скачать бинарник
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64
chmod +x bootimus-amd64

# Создать data-директорию
mkdir -p data

# Запуск (режим SQLite — база не требуется)
./bootimus-amd64 serve

# Админ-панель: http://localhost:8081
# Пароль администратора показывается в логах запуска
```

## Развёртывание через Docker

### Docker Compose с PostgreSQL

```bash
# Клонировать репозиторий
git clone https://github.com/garybowers/bootimus
cd bootimus

# Запуск с PostgreSQL
docker-compose up -d

# Посмотреть логи
docker-compose logs -f bootimus
```

Стек Docker Compose включает:
- **Сервер Bootimus**: основной PXE/HTTP-сервер загрузки
- **PostgreSQL**: база для управления клиентами/образами
- **Health checks**: автоматический мониторинг сервисов
- **Постоянное хранилище**: тома с ISO и базой

### Структура директорий

Bootimus автоматически создаёт поддиректории:
- `/data/isos/` — ISO-образы и извлечённые загрузочные файлы (в поддиректориях по ISO)
- `/data/bootloaders/` — кастомные загрузчики (опционально)
- `/data/bootimus.db` — SQLite-база (в режиме SQLite)

## Настройка сети

### Внутренняя bridge-сеть по умолчанию

По умолчанию контейнеры используют внутреннюю bridge-сеть с проброской портов:

```yaml
networks:
  bootimus_net:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
          gateway: 172.20.0.1
```

- **Сервер Bootimus**: `172.20.0.3`
- **PostgreSQL**: `172.20.0.2`
- **Доступ с хоста**: через проброс портов (например, `localhost:8081`)

### Bridged-сеть со статическим IP в LAN

Для продакшен-окружений PXE может понадобиться разместить контейнер прямо в вашей LAN со статическим IP.

#### Шаг 1: найдите свой сетевой интерфейс

```bash
ip addr show  # Linux
# Найдите ваш основной интерфейс (например, eth0, ens33, enp0s3)
```

#### Шаг 2: отредактируйте docker-compose.yml

Раскомментируйте секции сети `host_bridge`:

```yaml
services:
  bootimus:
    networks:
      # Закомментируйте внутренний bridge
      # bootimus_net:
      #   ipv4_address: 172.20.0.3
      # Включите host bridge
      host_bridge:
        ipv4_address: 192.168.1.100  # Желаемый статический IP
    environment:
      BOOTIMUS_SERVER_ADDR: 192.168.1.100  # Задаёт статический адрес сервера

networks:
  # Раскомментируйте и настройте под вашу LAN
  host_bridge:
    driver: macvlan
    driver_opts:
      parent: eth0  # Ваш сетевой интерфейс
    ipam:
      config:
        - subnet: 192.168.1.0/24      # Подсеть вашей LAN
          gateway: 192.168.1.1         # Шлюз вашей LAN
          ip_range: 192.168.1.100/32   # Статический IP контейнера
```

#### Шаг 3: настройте параметры сети

Обновите значения под вашу сеть:
- `parent`: сетевой интерфейс хоста (например, `eth0`, `ens33`)
- `subnet`: подсеть вашей LAN (например, `192.168.1.0/24`)
- `gateway`: IP вашего роутера (например, `192.168.1.1`)
- `ip_range`: статический IP для Bootimus (например, `192.168.1.100/32`)
- `BOOTIMUS_SERVER_ADDR`: тот же, что и статический IP

#### Шаг 4: запустите контейнер

```bash
docker-compose down
docker-compose up -d
```

#### Шаг 5: проверьте связность

```bash
# С другой машины в LAN
curl http://192.168.1.100:8081

# Пинг контейнера
ping 192.168.1.100
```

### Важные замечания по macvlan-сети

- **Macvlan-сеть**: контейнер выглядит как отдельное устройство в вашей LAN
- **Хост не может достучаться до контейнера**: хост-машина не может напрямую общаться с macvlan-контейнерами. Используйте отдельную VM/контейнер для админ-доступа или создайте macvlan-интерфейс на хосте.
- **Конфликты DHCP**: убедитесь, что статический IP вне DHCP-пула или зарезервирован в DHCP-сервере
- **Правила файрвола**: контейнер обходит файрвол хоста — настраивайте файрвол контейнера отдельно при необходимости

### Доступ к macvlan-контейнерам с хоста

Если нужен доступ к macvlan-контейнеру с хоста:

```bash
# Создать macvlan-интерфейс на хосте
sudo ip link add macvlan0 link eth0 type macvlan mode bridge
sudo ip addr add 192.168.1.101/32 dev macvlan0
sudo ip link set macvlan0 up
sudo ip route add 192.168.1.100/32 dev macvlan0

# Теперь можно достучаться до контейнера с хоста
curl http://192.168.1.100:8081
```

## Развёртывание из бинарника

### Системные требования

- **ОС**: Linux (amd64, arm64, armv7)
- **Привилегии**: root для порта 69 (TFTP) или непривилегированные порты
- **Диск**: 10 ГБ+ под ISO
- **Память**: минимум 512 МБ, рекомендуется 2 ГБ+

### Установка

```bash
# Скачать бинарник под вашу архитектуру
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64

# Сделать исполняемым
chmod +x bootimus-amd64

# Переместить в системную локацию
sudo mv bootimus-amd64 /usr/local/bin/bootimus

# Создать data-директорию
sudo mkdir -p /opt/bootimus/data

# Создать systemd-сервис
sudo nano /etc/systemd/system/bootimus.service
```

### systemd-сервис

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
# Включить и запустить сервис
sudo systemctl daemon-reload
sudo systemctl enable bootimus
sudo systemctl start bootimus

# Проверить статус
sudo systemctl status bootimus

# Посмотреть логи
sudo journalctl -u bootimus -f
```

## Настройка хранилища

### Структура data-директории

```
/opt/bootimus/data/
├── isos/                           # ISO-файлы
│   ├── ubuntu-24.04.iso           # ISO-файл
│   ├── ubuntu-24.04/              # Извлечённые загрузочные файлы
│   │   ├── vmlinuz
│   │   ├── initrd
│   │   └── casper/
│   │       └── filesystem.squashfs
│   └── debian-12.iso
├── bootloaders/                    # Кастомные загрузчики (опционально)
├── bootimus.db                     # SQLite-база (в режиме SQLite)
└── .admin_password                 # Сгенерированный пароль администратора
```

### Требования к дисковому пространству

- **ISO**: 1–10 ГБ на ISO
- **Извлечённые файлы**: 100 МБ – 3 ГБ на ISO
- **База**: < 100 МБ
- **Рекомендуется**: 50 ГБ+ под несколько ISO

### Лучшие практики по хранилищу

1. **Используйте SSD**: быстрее загрузка клиентов
2. **Регулярные бэкапы**: бэкапьте базу и ISO
3. **Мониторьте место**: настройте оповещения о нехватке
4. **Чистите старые ISO**: удаляйте неиспользуемые, чтобы освободить место

## Варианты базы данных

### Режим SQLite (по умолчанию)

SQLite **включён по умолчанию** — никакой настройки не требуется!

```bash
# Запуск с SQLite (по умолчанию)
./bootimus serve

# База автоматически создаётся в: <data_dir>/bootimus.db
```

**Преимущества**:
- Ноль настроек
- База в одном файле
- Идеально для одиночных серверов
- Простые бэкапы (просто скопируйте файл)

**Ограничения**:
- Меньшая конкурентность, чем у PostgreSQL
- Только один сервер (без кластеризации)

### Режим PostgreSQL

Для корпоративных развёртываний с высокой конкурентностью:

#### Через файл конфигурации

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

#### Через переменные окружения

```bash
export BOOTIMUS_DB_HOST=postgres.example.com
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secretpassword
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=require

./bootimus serve
```

**Преимущества**:
- Высокая конкурентность
- Multi-server развёртывания
- Продвинутая репликация
- Лучшая производительность на масштабе

**Требования**:
- PostgreSQL 12+
- Сетевая связность с базой
- Дополнительная инфраструктура

## Удалённые обновления и приватность

Bootimus разворачивается самостоятельно (self-hosted) и **не** «звонит домой» в фоне. Он поставляется с полным каталогом профилей дистрибутивов и инструментов, встроенным в бинарник, поэтому полностью работоспособен без исходящего доступа в интернет.

**Единственный** момент, когда Bootimus обращается к внешнему сервису, — это когда оператор **явно** запускает обновление профилей/инструментов — через кнопки «Check for Updates» в админ-панели, команду CLI `bootimus profiles update` или эндпоинты `POST /api/profiles/update` и `POST /api/tools/update`. Каждый из них выполняет неаутентифицированный `GET` статического JSON-файла на GitHub (`raw.githubusercontent.com/garybowers/bootimus/main/...`) и не отправляет никакой системной информации или идентификаторов.

Чтобы гарантировать, что удалённого обращения не произойдёт никогда (например, для развёртываний без доступа в сеть, air-gapped), запускайте с отключёнными удалёнными обновлениями:

```bash
bootimus serve --disable-remote-profiles
# or in bootimus.yaml:  disable_remote_profiles: true
# or via env:           BOOTIMUS_DISABLE_REMOTE_PROFILES=true
```

Подробности см. в [руководстве по профилям дистрибутивов](distro-profiles.md#удалённые-обновления-и-приватность).

## Production-развёртывание

### Docker с SQLite (самый простой)

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

### Docker Compose с PostgreSQL

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

### Опции конфигурации

Bootimus использует разумные дефолты и требует минимум настройки.

#### Приоритет конфигурации

1. Флаги командной строки (высший приоритет)
2. Переменные окружения (с префиксом `BOOTIMUS_`)
3. Файл конфигурации (`bootimus.yaml`)

#### Пример файла конфигурации

```yaml
# bootimus.yaml
tftp_port: 69
http_port: 8080
admin_port: 8081
data_dir: ./data          # Базовая data-директория
server_addr: ""           # Авто-определение, если не задано

# Настройки базы (опционально)
# Если db.host не задан, автоматически используется SQLite
db:
  host: localhost       # Оставьте пустым для SQLite
  port: 5432
  user: bootimus
  password: bootimus
  name: bootimus
  sslmode: disable
```

#### Переменные окружения

```bash
# Настройки сервера
export BOOTIMUS_TFTP_PORT=69
export BOOTIMUS_HTTP_PORT=8080
export BOOTIMUS_ADMIN_PORT=8081
export BOOTIMUS_DATA_DIR=/var/lib/bootimus/data
export BOOTIMUS_SERVER_ADDR=192.168.1.100

# Настройки базы (только для PostgreSQL)
export BOOTIMUS_DB_HOST=postgres      # Пусто = SQLite
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secret
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=disable

./bootimus serve
```

## Диагностика

### Permission denied на порту 69

```bash
# Запуск от root
sudo ./bootimus serve

# Или Docker с capability NET_BIND_SERVICE
docker run --cap-add NET_BIND_SERVICE ...

# Или непривилегированный порт
./bootimus serve --tftp-port 6969
```

### Сбой подключения к базе

```bash
# Проверить SQLite-базу
ls -la data/bootimus.db

# Для PostgreSQL — проверить подключение
psql -h localhost -U bootimus -d bootimus

# Посмотреть логи PostgreSQL
docker logs bootimus-db
```

### До контейнера не достучаться из LAN

```bash
# Проверить настройки macvlan
docker network inspect bootimus_host_bridge

# Проверить присвоенный IP
docker exec bootimus ip addr show

# Проверить маршрутизацию
ip route | grep 192.168.1.100

# Проверить файрвол
sudo iptables -L -n | grep 192.168.1.100
```

### Кончилось место на диске

```bash
# Проверить использование диска
df -h /opt/bootimus/data

# Найти большие файлы
du -sh /opt/bootimus/data/*

# Удалить старые ISO
rm /opt/bootimus/data/isos/old-image.iso

# Сканировать ISO заново для обновления базы
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Дальше

- Прочитайте [руководство по управлению образами](images.md) — как работать с ISO
- См. [руководство по админ-консоли](admin.md) для управления
- Настройте [DHCP-сервер](dhcp.md) для PXE-загрузки
