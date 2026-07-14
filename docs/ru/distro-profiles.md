# Руководство по профилям дистрибутивов

Bootimus использует профили дистрибутивов для определения типа ISO и генерации правильных параметров загрузки. Профили основаны на данных — поддержку новых дистрибутивов можно добавить без правки кода.

## Оглавление

- [Обзор](#обзор)
- [Как это работает](#как-это-работает)
- [Просмотр профилей](#просмотр-профилей)
- [Обновление профилей](#обновление-профилей)
- [Удалённые обновления и приватность](#удалённые-обновления-и-приватность)
- [Создание пользовательских профилей](#создание-пользовательских-профилей)
- [Поля профиля](#поля-профиля)
- [Плейсхолдеры](#плейсхолдеры)
- [Примеры](#примеры)
- [Диагностика](#диагностика)

## Обзор

Профили дистрибутивов определяют:
- **Как определить**, к какому дистрибутиву относится ISO (сопоставление по шаблону имени файла)
- **Где искать** kernel, initrd и squashfs внутри ISO
- **Какие параметры загрузки** использовать при PXE-загрузке
- **Какой тип автоустановки** поддерживается (preseed, kickstart, autoinstall и т. д.)

### Типы профилей

| Тип | Описание |
|------|-------------|
| **Built-in** | Поставляется с Bootimus, обновляется из центрального репозитория |
| **Custom** | Создан пользователем, никогда не перезаписывается при обновлении |

Пользовательские профили всегда имеют приоритет над встроенными при сопоставлении имён ISO.

## Как это работает

1. Когда ISO загружается или извлекается, Bootimus сопоставляет имя файла с шаблонами профилей
2. Пути kernel/initrd подходящего профиля используются для поиска загрузочных файлов внутри ISO
3. Параметры загрузки профиля становятся дефолтом (редактируются в Properties образа)
4. На момент загрузки плейсхолдеры в параметрах разрешаются в реальные URL

### Жизненный цикл профилей

```
Сборка:           distro-profiles.json встроен в бинарник
                        ↓
Первый запуск:    Профили засеваются в базу
                        ↓
«Check for Updates»:  Последние профили подтягиваются с GitHub
                        ↓
Создание пользователем:   Custom-профили хранятся в базе (никогда не перезаписываются)
```

## Просмотр профилей

Откройте **Boot > Distro Profiles** в админ-панели, чтобы увидеть все загруженные профили с их шаблонами имён, параметрами загрузки, типом (Built-in/Custom) и версией.

## Обновление профилей

Обновление профилей — всегда **явное действие по запросу**: Bootimus никогда не обращается к удалённому каталогу самостоятельно. Пока вы не запустите обновление, используются профили, встроенные в бинарник на этапе сборки. См. [Удалённые обновления и приватность](#удалённые-обновления-и-приватность), чтобы точно узнать, к чему происходит обращение и как это отключить.

При запуске обновления:

- Новые профили добавляются
- Существующие встроенные профили обновляются до последней версии
- Пользовательские профили никогда не модифицируются

Есть три способа его запустить:

### Из админ-панели

Нажмите **«Check for Updates»** во вкладке **Boot > Distro Profiles**.

### Через CLI

```bash
bootimus profiles update
```

Использует ту же конфигурацию базы, что и `serve` (PostgreSQL, если задан `db.host`, иначе локальная база SQLite в `data_dir`). Учитывает `--disable-remote-profiles` и завершается без обращения к сети, если этот флаг установлен.

### Через API

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/profiles/update
```

Ответ:
```json
{
  "success": true,
  "message": "Updated to version 0.1.21 (2 added, 5 updated)"
}
```

## Удалённые обновления и приватность

Bootimus разворачивается самостоятельно (self-hosted) и не «звонит домой» в фоне. Единственный момент, когда он обращается к внешнему сервису за профилями, — это когда **вы** явно запускаете обновление одним из описанных выше способов.

**Адрес, к которому происходит обращение (только при явном обновлении):**

```
https://raw.githubusercontent.com/garybowers/bootimus/main/distro-profiles.json
```

Аналогичный каталог инструментов («Check for Updates» во вкладке **Tools** / `POST /api/tools/update`) работает так же и обращается к:

```
https://raw.githubusercontent.com/garybowers/bootimus/main/tools-profiles.json
```

Это обычные неаутентифицированные `GET`-запросы к хосту статических файлов GitHub. Bootimus не отправляет с ними никакой системной информации, идентификаторов или данных об использовании — он просто скачивает JSON-файл. Учтите, что, как и при любом HTTP-запросе, GitHub видит ваш исходный IP-адрес и стандартные метаданные запроса.

### Отключение удалённых обновлений

Чтобы гарантировать, что Bootimus никогда не обратится к удалённому каталогу — для развёртываний без доступа в сеть (air-gapped) или из соображений политики — запускайте его с:

```bash
bootimus serve --disable-remote-profiles
```

или задайте эквивалентное значение конфигурации/переменной окружения:

```yaml
# bootimus.yaml
disable_remote_profiles: true
```

```bash
# environment variable
BOOTIMUS_DISABLE_REMOTE_PROFILES=true
```

При отключении встроенные профили всё равно засеваются из бинарника при первом запуске, поэтому Bootimus полностью работоспособен офлайн. Кнопка «Check for Updates», эндпоинт `/api/profiles/update` и команда `bootimus profiles update` — все они откажутся выполняться.

## Создание пользовательских профилей

### Через веб-интерфейс

1. Откройте **Boot > Distro Profiles**
2. Нажмите **«+ Add Custom Profile»**
3. Заполните поля профиля
4. Нажмите **«Create Profile»**

### Через API

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/profiles/save \
  -H "Content-Type: application/json" \
  -d '{
    "profile_id": "my-distro",
    "display_name": "My Custom Distro",
    "family": "debian",
    "filename_patterns": ["mydistro", "my-distro"],
    "kernel_paths": ["/live/vmlinuz", "/boot/vmlinuz"],
    "initrd_paths": ["/live/initrd.img", "/boot/initrd"],
    "squashfs_paths": ["/live/filesystem.squashfs"],
    "default_boot_params": "boot=live initrd=initrd ip=dhcp",
    "boot_params_with_squashfs": "boot=live initrd=initrd fetch={{SQUASHFS}}",
    "auto_install_type": "preseed"
  }'
```

### Удаление пользовательских профилей

Удалить можно только пользовательские профили. Встроенные восстанавливаются при следующем обновлении.

```bash
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/profiles/delete?id=my-distro"
```

## Поля профиля

| Поле | Обязательное | Описание |
|-------|----------|-------------|
| `profile_id` | Да | Уникальный идентификатор (например, `ubuntu`, `my-distro`) |
| `display_name` | Да | Человекочитаемое имя в UI |
| `family` | Нет | Семейство дистрибутивов (например, `debian`, `arch`, `redhat`) — для группировки |
| `filename_patterns` | Да | Подстроки для поиска в именах ISO-файлов (без учёта регистра) |
| `kernel_paths` | Нет | Пути для поиска kernel внутри ISO (например, `/casper/vmlinuz`) |
| `initrd_paths` | Нет | Пути для поиска initrd внутри ISO |
| `squashfs_paths` | Нет | Пути для поиска корневой ФС squashfs |
| `default_boot_params` | Нет | Параметры загрузки kernel по умолчанию (с поддержкой плейсхолдеров) |
| `boot_params_with_squashfs` | Нет | Альтернативные параметры загрузки, когда обнаружен squashfs |
| `auto_install_type` | Нет | Формат автоустановки: `preseed`, `kickstart`, `autoinstall`, `autounattend` |
| `boot_method` | Нет | Переопределение метода загрузки (например, `wimboot` для Windows) |

## Плейсхолдеры

Параметры загрузки поддерживают эти плейсхолдеры, разрешаются на момент загрузки:

| Плейсхолдер | Разрешается в | Пример |
|-------------|-------------|---------|
| `{{BASE_URL}}` | HTTP-URL сервера | `http://192.168.1.10:8080` |
| `{{CACHE_DIR}}` | Директория извлечённых файлов | `ubuntu-24.04-server-amd64` |
| `{{FILENAME}}` | Имя ISO-файла (URL-кодированное) | `ubuntu-24.04-server-amd64.iso` |
| `{{SQUASHFS}}` | Полный URL файла squashfs | `http://192.168.1.10:8080/boot/ubuntu.../casper/filesystem.squashfs` |

### Пример с плейсхолдерами

```
boot=live initrd=initrd fetch={{SQUASHFS}} ip=dhcp
```

Разрешается в:
```
boot=live initrd=initrd fetch=http://192.168.1.10:8080/boot/debian-live-13/live/filesystem.squashfs ip=dhcp
```

## Примеры

### Live-ISO на базе Debian

```json
{
  "profile_id": "my-debian-live",
  "display_name": "My Debian Live Spin",
  "family": "debian",
  "filename_patterns": ["my-debian"],
  "kernel_paths": ["/live/vmlinuz"],
  "initrd_paths": ["/live/initrd.img"],
  "squashfs_paths": ["/live/filesystem.squashfs"],
  "default_boot_params": "initrd=initrd boot=live priority=critical",
  "boot_params_with_squashfs": "initrd=initrd boot=live priority=critical fetch={{SQUASHFS}}"
}
```

### Дистрибутив на базе Arch

```json
{
  "profile_id": "my-arch-spin",
  "display_name": "My Arch Spin",
  "family": "arch",
  "filename_patterns": ["myarch"],
  "kernel_paths": ["/arch/boot/x86_64/vmlinuz-linux", "/boot/vmlinuz-linux"],
  "initrd_paths": ["/arch/boot/x86_64/initramfs-linux.img", "/boot/initramfs-linux.img"],
  "squashfs_paths": ["/arch/x86_64/airootfs.sfs"],
  "default_boot_params": "archisobasedir=arch archiso_http_srv={{BASE_URL}}/boot/{{CACHE_DIR}}/iso/ ip=dhcp"
}
```

### Установщик на базе RHEL

```json
{
  "profile_id": "my-rhel-clone",
  "display_name": "My RHEL Clone",
  "family": "redhat",
  "filename_patterns": ["myrhel"],
  "kernel_paths": ["/images/pxeboot/vmlinuz"],
  "initrd_paths": ["/images/pxeboot/initrd.img"],
  "default_boot_params": "root=live:{{BASE_URL}}/isos/{{FILENAME}} rd.live.image inst.repo={{BASE_URL}}/boot/{{CACHE_DIR}}/iso/ rd.neednet=1 ip=dhcp",
  "auto_install_type": "kickstart"
}
```

## Диагностика

### ISO не определяется как нужный дистрибутив

Проверьте, совпадает ли имя ISO с каким-нибудь шаблоном профиля:

1. Откройте вкладку **Distro Profiles**
2. Посмотрите столбец «Filename Patterns»
3. Если ни один шаблон не подходит вашему имени ISO, создайте пользовательский профиль

### Неправильные параметры загрузки после извлечения

1. Откройте **Properties** образа
2. Нажмите **«Re-detect»** рядом с Boot Parameters
3. Или отредактируйте параметры вручную — они поддерживают плейсхолдеры

### «Check for Updates» завершился ошибкой

Обновление тянется с GitHub. Проверьте:
- У сервера есть интернет
- `raw.githubusercontent.com` не заблокирован
- Попробуйте позже, если GitHub лежит

### Пользовательский профиль не совпадает

У пользовательских профилей приоритет над встроенными. Убедитесь:
- В `filename_patterns` есть подстроки, совпадающие с именем вашего ISO (без учёта регистра)
- ID профиля уникальный
- Профиль успешно сохранён

### Контрибьютинг профилей

Чтобы добавить профиль в официальный список для всех пользователей:
1. Сделайте форк [репозитория Bootimus](https://github.com/garybowers/bootimus)
2. Отредактируйте `distro-profiles.json` в корне репозитория
3. Добавьте свой профиль в массив `profiles`
4. Отправьте pull request

Так все пользователи Bootimus получат новый профиль через «Check for Updates».
