# Руководство по управлению образами

Полное руководство по управлению ISO-образами, извлечению загрузочных файлов и работе со специальными случаями вроде Debian/Ubuntu netboot.

## Оглавление

- [Добавление образов](#добавление-образов)
- [Контрольные суммы и уведомления об обновлениях](#контрольные-суммы-и-уведомления-об-обновлениях)
- [Унифицированные образы ядра (UKI)](#унифицированные-образы-ядра-uki)
- [OpenBSD и NetBSD](#openbsd-и-netbsd)
- [WinPE и спасательные носители](#winpe-и-спасательные-носители)
- [Извлечение kernel](#извлечение-kernel)
- [Поддержка netboot](#поддержка-netboot)
- [Оптимизация Ubuntu Desktop](#оптимизация-ubuntu-desktop)
- [Поддерживаемые дистрибутивы](#поддерживаемые-дистрибутивы)
- [Диагностика](#диагностика)

## Добавление образов

### Загрузка через веб-интерфейс

1. Откройте админ-панель: `http://your-server:8081`
2. Нажмите кнопку **«Upload ISO»**
3. Перетащите ISO-файл или кликните, чтобы выбрать
4. По желанию добавьте описание
5. Поставьте галочку **«Public»**, чтобы сделать доступным всем клиентам
6. Нажмите **«Upload»**

**Лимиты загрузки**: 10 ГБ на файл

### Загрузка через API

```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### Скачать по URL

Скачивание ISO прямо на сервер без локальной загрузки:

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
```

**Отслеживание прогресса**:
```bash
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### Контрольные суммы и уведомления об обновлениях

**Проверка при скачивании.** Когда ISO скачивается через **Get Images** (встроенный каталог), Bootimus ищет рядом с ним опубликованный файл контрольных сумм (`SHA256SUMS`, `sha256sum.txt` и подобные) и считает хеш по мере скачивания. При несовпадении скачивание завершается ошибкой, а файл удаляется, так что обрезанный или подменённый образ никогда не попадёт в меню загрузки. Скачивания по обычному URL тоже можно проверять, передав `sha256` или `checksum_url` в `/api/images/download`:

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{"url": "https://releases.ubuntu.com/24.04.4/ubuntu-24.04.4-live-server-amd64.iso",
       "checksum_url": "https://releases.ubuntu.com/24.04.4/SHA256SUMS"}'
```

Проверенные образы отмечаются значком **✓ Verified** в списке образов, а записанный SHA-256 показывается в панели свойств образа. Образы, скачанные без опубликованной контрольной суммы или загруженные вручную, помечаются как **Unverified**; вставьте SHA-256 или URL файла контрольных сумм в панель свойств и нажмите **Verify**, чтобы посчитать хеш файла в фоне (`POST /api/images/verify?filename=…`, опрашивайте тот же URL через `GET`).

**Новые релизы.** Раз в сутки (а также по **Settings → Housekeeping → Check now** или `POST /api/catalogue/check`) Bootimus сравнивает установленные образы с каталогом. Образ, скачанный из каталога, сопоставляется по дистрибутиву и линейке релизов, так что `ubuntu-24.04.3-desktop-amd64.iso` помечается, когда появляется `24.04.4`; образы, добавленные до появления этой функции, сопоставляются по форме имени файла. Помеченные образы получают значок **Update available** и кнопку **Download newer version** в панели свойств, инструменты показывают тот же значок с кнопкой **Download newer**, а дашборд считает ожидающие обновления. Проверка использует онлайн-каталог, когда удалённые обновления профилей разрешены, и откатывается к каталогу, встроенному в бинарник, когда задан `--disable-remote-profiles`. Снимите флажок **Check the catalogue daily** в разделе Housekeeping, чтобы отключить её.

### Организация по папкам

ISO, размещённые в поддиректориях, автоматически группируются в загрузочном меню:

```
/data/isos/
├── ubuntu-24.04.iso              # без группы
├── linux/                        # группа «linux»
│   ├── debian-12.iso
│   └── servers/                  # подгруппа «servers» под «linux»
│       └── truenas-scale.iso
└── windows/                      # группа «windows»
    └── win11.iso
```

Группы автоматически создаются при запуске и при сканировании. Их также можно вручную управлять через вкладку Groups в админ-UI.

### Сканирование существующих ISO

Если вы вручную копируете ISO в data-директорию (в том числе в поддиректории):

1. Скопируйте ISO-файлы в директорию `/data/isos/` (или поддиректории)
2. Нажмите кнопку **«Scan for ISOs»** в админ-панели
3. Bootimus обнаружит и зарегистрирует новые ISO и создаст группы из папок

**Через API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## Унифицированные образы ядра (UKI)

Унифицированный образ ядра (UKI) — это один исполняемый EFI-файл, объединяющий ядро Linux, его initrd и командную строку ядра. Bootimus загружает UKI напрямую, без извлечения.

**Добавление UKI**: загрузите файл `.efi`, скачайте его по URL или скопируйте в `/data/isos/` и запустите сканирование — так же, как для ISO. Bootimus регистрирует только те файлы `.efi`, которые являются UKI (EFI-приложения с секцией `.linux`); другие EFI-программы отклоняются, а файлы внутри извлечённой папки ISO игнорируются.

**Параметры загрузки**: командная строка из секции `.cmdline` образа при первом добавлении копируется в параметры загрузки, поэтому её можно увидеть и изменить в свойствах образа. Bootimus загружает образ так:

```
kernel http://<server>:8080/isos/<image>.efi <boot parameters>
```

При выключенном Secure Boot эти параметры заменяют встроенную командную строку, поэтому встроенная заполнена заранее. При включённом Secure Boot systemd-stub внутри большинства UKI игнорирует их и всегда использует встроенную командную строку.

**Только UEFI**: UKI — это EFI-программы, поэтому клиенты с legacy BIOS не могут их загрузить. При выборе такого образа на BIOS-клиенте появится сообщение о том, что нужен UEFI.

## OpenBSD и NetBSD

OpenBSD и NetBSD не загружаются из ядра и initrd в стиле Linux. У каждой есть собственный сетевой загрузчик второй стадии, и этот загрузчик умеет только TFTP: он берёт свой файл конфигурации и установочное ядро из корневой директории TFTP-сервера. Bootimus поддерживает обе системы, извлекая эти файлы из установочного ISO и отдавая их через свой TFTP-сервер, поэтому TFTP-порт должен быть доступен клиентам (для самого PXE он и так доступен).

**Добавление образа**: загрузите `install77.iso` (или `cd77.iso`) для OpenBSD либо `NetBSD-10.1-amd64.iso` для NetBSD, затем выполните **Extract Kernel/Initrd**, как для любого другого ISO. Извлечение копирует загрузчики и установочное ядро в папку `bsd/` рядом с ISO и распаковывает полное дерево ISO, чтобы установщик мог брать свои наборы с `http://<server>:8080/boot/<image>/iso/`.

| Дистрибутив | Загрузчики, берущиеся из ISO | Ядро |
|-------------|------------------------------|------|
| OpenBSD | `pxeboot` (BIOS), `BOOTX64.EFI`, `BOOTIA32.EFI`, `BOOTAA64.EFI` (UEFI) из `/<version>/<arch>/` | `bsd.rd` |
| NetBSD | `pxeboot_ia32.bin` (BIOS) из `/<arch>/installation/misc/`, `bootx64.efi` (UEFI), если он есть в ISO | `netbsd-INSTALL.gz` |

**Как происходит загрузка**: пункт меню передаёт управление небольшому iPXE-скрипту на HTTP-порту, который запоминает, что этот клиент (по IP-адресу) выбрал данный образ, и затем загружает подходящий загрузчик по TFTP из `bsd/<image>/`. В течение следующих 15 минут запросы с этого IP на файлы, которые загрузчики ищут в корне TFTP (`etc/boot.conf`, `etc/random.seed`, `bsd.rd`, `bsd` для OpenBSD; `boot.cfg`, `EFI/NetBSD/boot.cfg`, `netbsd` и `netbsd-INSTALL.gz` для NetBSD), обслуживаются из этого образа. Файл конфигурации генерируется на лету и указывает загрузчику на ядро в `bsd/<image>/`, так что явные пути тоже работают. Если в ISO нет загрузчика для прошивки клиента, клиент видит сообщение об этом и возвращается в меню.

**Последовательная консоль и другие настройки загрузчика**: добавьте дополнительные строки конфигурации загрузчика в **Boot Parameters** образа, разделяя их точкой с запятой. Для OpenBSD они дописываются в сгенерированный `boot.conf`, например `stty com0 115200;set tty com0`. Для NetBSD они дописываются в сгенерированный `boot.cfg`, например `consdev=com0`.

**Известные ограничения**:

- Эта поддержка новая и ещё не проверена на реальном железе. Пожалуйста, сообщайте, что работает, а что нет.
- `pxeboot_ia32.bin` в NetBSD после загрузки выполняет собственный обмен DHCP и по полю DHCP `filename` решает, использовать NFS или TFTP. Без префикса `tftp:` в нём он по умолчанию выбирает NFS, который Bootimus для этой цели не предоставляет. Если загрузчик сообщает, что не может открыть `boot.cfg` или ядро, введите `boot tftp:bsd/<image>/netbsd-INSTALL.gz` в его приглашении или задайте для клиентов NetBSD `filename "tftp:boot.cfg";` на своём DHCP-сервере. Загрузчики OpenBSD всегда используют TFTP, и такой шаг им не нужен.
- Привязка к клиенту ведётся по IP-адресу, поэтому два клиента за одним NAT-адресом, загружающие разные BSD-образы в течение 15 минут, получат одну и ту же конфигурацию.
- Автоустановка для установщиков BSD пока не подключена. Установщики по-прежнему могут брать наборы из распакованного дерева ISO по HTTP.

## WinPE и спасательные носители

Bootimus загружает установщики Windows через [wimboot](https://ipxe.org/wimboot), и тот же путь теперь покрывает спасательные носители на основе WinPE, такие как WinPE Сергея Стрельца, Active@ Boot Disk и Hiren's BootCD PE.

**Поддерживаемые раскладки**: любой ISO, в котором есть `BCD` (в `boot/` или `efi/microsoft/boot/`), `boot/boot.sdi` и хотя бы один файл `.wim`. У установочных носителей есть ещё `sources/install.wim` или `install.esd`; носители без него относятся к профилю **WinPE / Rescue Media** (`winpe`), а не **Windows**, поэтому автоустановка и обработка SMB к ним не применяются. WIM-файлы ищутся на первых четырёх уровнях директорий, так что `SSTR/*.wim` у Стрельца и подобные раскладки работают без переименований.

**Выбор WIM**: после извлечения панель свойств образа перечисляет все найденные WIM с размерами. Выберите тот, который должен загружать wimboot, и, если WIM содержит несколько образов, индекс образа. Индекс добавляется в строку wimboot только тогда, когда он больше единицы, а явный `index=` в параметрах загрузки имеет приоритет. То же самое можно задать через API:

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"wim_path":"SSTR/strelec10x64Eng.wim","wim_index":1}' \
  "http://localhost:8081/api/images/winpe?filename=WinPE_Strelec.iso"
```

**Как происходит загрузка**: клиенту отправляется только выбранный WIM; wimboot сам синтезирует BCD и `boot.sdi`, что позволяет избежать чёрного экрана, который некоторые носители показывают при использовании BCD с DVD. Параметры загрузки ограничены собственными опциями wimboot (`rawbcd`, `rawwim`, `gui`, `linear`, `quiet`, `pause`, `index`, `initrdfile`), ровно как для установщиков Windows.

**Ограничения**:

- Программы, которые спасательная среда ожидает найти на своём загрузочном носителе (например, папка `SSTR` с портативными инструментами у Стрельца), находятся вне WIM, поэтому после PXE-загрузки они отсутствуют, если среда не может сама получить их по сети. Базовый рабочий стол WinPE и инструменты, встроенные в WIM, работают.
- Носители, которым нужны собственные записи BCD (пользовательские меню `bootmgr`, меню выбора из нескольких WIM), загружаются сразу в выбранный WIM вместо показа этого меню.
- Отдельные файлы `.wim` пока нельзя загрузить сами по себе; поместите их в ISO с `BCD` и `boot.sdi` или используйте полный спасательный ISO.

## Извлечение kernel

Большинство современных ISO поддерживают прямую HTTP-загрузку через команду `sanboot` в iPXE, которая скачивает и грузит ISO целиком. Однако извлечение kernel и initrd даёт значительные преимущества:

### Преимущества извлечения kernel

- **Быстрая загрузка**: качаем только kernel/initrd (~100 МБ) вместо всего ISO (1–10 ГБ)
- **Меньше трафика**: критично для сетей со множеством клиентов
- **Лучшая совместимость**: некоторые ISO некорректно поддерживают `sanboot`
- **Сетевая установка**: используйте netboot-файлы для установщиков Debian/Ubuntu
- **UEFI Secure Boot**: извлечение обнаруживает подписанный shim в ISO, позволяя клиентам с Secure Boot проверить kernel — см. [руководство по Secure Boot](secure-boot.md)

### Как извлечь

**Через веб-интерфейс**:
1. Откройте вкладку **Images**
2. Найдите свой ISO-образ
3. Нажмите кнопку **«Extract»**
4. Дождитесь завершения извлечения

**Через API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04-live-server-amd64.iso"}'
```

### Ручное извлечение

Если встроенный экстрактор не поддерживает ваш ISO, можно извлечь загрузочные файлы вручную, и bootimus автоматически их обнаружит.

1. Создайте директорию с таким же именем, как у ISO (без расширения `.iso`):
   ```bash
   mkdir -p data/isos/my-custom-distro/
   ```

2. Поместите kernel и initrd в эту директорию с такими именами:
   ```
   data/isos/
   ├── my-custom-distro.iso
   └── my-custom-distro/
       ├── vmlinuz          # kernel
       └── initrd           # initrd/initramfs
   ```

3. Нажмите **«Scan for ISOs»** в админ-панели (или перезапустите bootimus). Образ автоматически распознается как извлечённый и переключится на метод загрузки kernel.

Это также работает для ISO в поддиректориях:
```
data/isos/linux/my-custom-distro.iso
data/isos/linux/my-custom-distro/vmlinuz
data/isos/linux/my-custom-distro/initrd
```

### Что извлекается

Bootimus автоматически определяет дистрибутив и извлекает:

- **Kernel**: `vmlinuz` (или `linux`, `bzImage`)
- **Initrd**: `initrd`, `initrd.gz`, `initrd.lz`
- **Squashfs** (Ubuntu/Debian live): `filesystem.squashfs`
- **Метаданные дистрибутива**: тип ОС, параметры загрузки

**Расположение извлечённых файлов**:
```
/data/isos/
├── ubuntu-24.04.iso                    # Исходный ISO
└── ubuntu-24.04/                       # Директория с извлечённым
    ├── vmlinuz                         # Kernel
    ├── initrd                          # Initrd
    └── casper/
        └── filesystem.squashfs         # Squashfs-файловая система
```

### Автоматический выбор метода загрузки

После извлечения Bootimus автоматически выбирает оптимальный метод загрузки:

| Дистрибутив | Метод загрузки | Скачивается |
|--------------|-------------|-----------|
| Ubuntu Desktop (извлечён) | `fetch=` | ~2,8 ГБ (только squashfs) |
| Ubuntu Desktop (не извлечён) | `url=` | ~18 ГБ (ISO × 3) |
| Ubuntu Server (netboot) | Netboot | ~50 МБ (netboot-файлы) |
| Debian Installer (netboot) | Netboot | ~30 МБ (netboot-файлы) |
| Arch Linux | HTTP boot | ~100 МБ (kernel/initrd) |
| Fedora/RHEL | HTTP boot | ~150 МБ (kernel/initrd + stage2) |

## Поддержка netboot

Некоторые ISO-установщики (Debian, Ubuntu Server) не содержат полную ОС — они спроектированы качать пакеты во время установки. Для них Bootimus поддерживает скачивание официальных netboot-файлов.

### Определение требования netboot

Когда вы извлекаете ISO установщика Debian или Ubuntu Server, Bootimus определяет, что нужен netboot:

**Индикаторы**:
- ISO содержит директорию `/install/` (а не `/casper/`)
- Тип «installer» (не live/desktop)
- Маленький размер ISO (< 1 ГБ)

**Админ-панель показывает**:
- Бейдж «Netboot Required»
- Кнопку «Download Netboot»

### Скачивание netboot-файлов

**Через веб-интерфейс**:
1. Откройте вкладку **Images**
2. Найдите ISO-установщик с бейджем «Netboot Required»
3. Нажмите кнопку **«Download Netboot»**
4. Дождитесь загрузки и извлечения

**Через API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

### Что такое netboot-файлы?

Netboot-файлы — это официальные минимальные загрузочные файлы дистрибутивов:

**Debian netboot**:
- Источник: `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`
- Размер: ~30 МБ
- Содержит: `vmlinuz`, `initrd.gz`, файлы установщика

**Ubuntu netboot**:
- Источник: `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`
- Размер: ~50 МБ
- Содержит: `vmlinuz`, `initrd.gz`, файлы установщика

### Как работает netboot

1. **Клиент грузится**: скачивает netboot kernel/initrd (~50 МБ)
2. **Установщик стартует**: netboot initrd запускает сетевой установщик
3. **Скачивание пакетов**: установщик качает пакеты с зеркал Ubuntu/Debian
4. **Установка**: ОС ставится прямо из интернет-репозиториев

**Преимущества**:
- Всегда самые свежие пакеты (а не залежавшиеся из ISO)
- Минимум трафика к PXE-серверу (без скачивания ISO)
- Меньше требования к хранилищу
- Официальные, подписанные загрузочные файлы

### Debian Installer Netboot

**Поддерживаемые ISO**:
- `debian-*-netinst.iso` — сетевой установщик
- Маленькие ISO-установщики Debian с директорией `/install/`

**Определение**:
```
Структура ISO:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**Netboot URL**: `http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz`

**Параметры загрузки**: `priority=critical ip=dhcp`

### Ubuntu Server Netboot

**Поддерживаемые ISO**:
- `ubuntu-*-live-server-*.iso` — live-server установщик с директорией `/install/`
- Старые установщики Ubuntu Server

**Определение**:
```
Структура ISO:
├── install/
│   ├── vmlinuz
│   └── initrd.gz
```

**Netboot URL**: `http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz`

**Параметры загрузки**: `ip=dhcp`

### Важно: Ubuntu Desktop vs Server

Есть **два типа** ISO Ubuntu с разными методами загрузки:

| Тип | Шаблон имени ISO | Директория | Метод загрузки | Netboot? |
|------|------------------|-----------|-------------|----------|
| **Desktop/Live** | `ubuntu-*-desktop-*.iso` | `/casper/` | `fetch=` или `url=` | Нет |
| **Server Installer** | `ubuntu-*-live-server-*.iso` (с `/install/`) | `/install/` | Netboot | Да |

**Ubuntu Desktop** (`/casper/`):
- Содержит полную live-ОС
- Загрузка через casper с `fetch=` или `url=`
- Извлеките kernel, чтобы использовать `fetch=` (качается только squashfs)
- Поддержки netboot нет

**Ubuntu Server Installer** (`/install/`):
- Минимальный сетевой установщик
- Требует netboot-файлы
- Качает пакеты во время установки
- Намного эффективнее

## Оптимизация Ubuntu Desktop

ISO Ubuntu Desktop используют live-систему загрузки casper. Без оптимизации они скачивают весь ISO **трижды** (~18 ГБ для 6 ГБ ISO).

### Проблема: тройная загрузка ISO

**Поведение по умолчанию** (без извлечения):
```
Параметр загрузки: url=http://server/ubuntu.iso

Результат:
- Загрузка 1: kernel проверяет ISO (6 ГБ)
- Загрузка 2: initrd проверяет ISO (6 ГБ)
- Загрузка 3: casper монтирует ISO (6 ГБ)
Итого: ~18 ГБ скачано
```

### Решение 1: извлечь и использовать параметр `fetch=`

**После извлечения**:
```
Параметр загрузки: fetch=http://server/ubuntu/casper/filesystem.squashfs

Результат:
- Загрузка: только squashfs (~2,8 ГБ)
Итого: ~2,8 ГБ скачано
```

**Как включить**:
1. Извлеките kernel/initrd из ISO
2. Bootimus автоматически использует параметр `fetch=`
3. Качается только squashfs (не весь ISO)

**Экономия**: сокращение на 85% (18 ГБ → 2,8 ГБ)

### Решение 2: использовать Ubuntu Server Netboot

Для серверных развёртываний используйте Ubuntu Server installer с netboot:

**Подход netboot**:
```
1. Загрузить ubuntu-server.iso
2. Извлечь kernel/initrd
3. Скачать netboot-файлы
4. Загрузка через netboot (~50 МБ скачивания)
5. Установка из репозиториев Ubuntu
```

**Экономия**: сокращение на 99% (18 ГБ → 50 МБ)

### Справочник параметров загрузки

**Ubuntu Desktop (casper)**:
```bash
# По умолчанию (без извлечения) — качает ISO 3 раза
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp url=http://server/ubuntu.iso

# Оптимизировано (с извлечением) — качает squashfs один раз
boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ip=dhcp fetch=http://server/ubuntu/casper/filesystem.squashfs
```

**Ubuntu Server (netboot)**:
```bash
# Netboot — минимальное скачивание
ip=dhcp
```

## Поддерживаемые дистрибутивы

### Полностью протестировано

| Дистрибутив | Извлечение kernel | Netboot | Заметки |
|--------------|-------------------|---------|-------|
| **Arch Linux** | Да | N/A | `/arch/boot/x86_64/vmlinuz-linux` |
| **Fedora Workstation** | Да | N/A | `/isolinux/vmlinuz` |
| **Rocky Linux** | Да | N/A | `/isolinux/vmlinuz` |
| **Debian (installer)** | Да | Да | `/install/vmlinuz` + netboot |
| **Debian Live** | Да | Нет | `/live/vmlinuz` |
| **Ubuntu Desktop** | Да | Нет | `/casper/vmlinuz` + оптимизация fetch |
| **Ubuntu Server** | Да | Да | `/install/vmlinuz` + netboot |
| **Pop!_OS** | Да | Нет | `/casper/vmlinuz` |
| **TrueNAS SCALE** | Да | Нет | 25.10.2.1: установка и загрузка с диска в QEMU UEFI; загрузка всего ISO |
| **Proxmox VE** | Да | Нет | `/boot/linux26` + `/boot/initrd.img` |
| **openSUSE** | Да | N/A | `/boot/x86_64/loader/linux` |
| **NixOS** | N/A | N/A | Sanboot |

### Шаблоны определения

Bootimus определяет дистрибутивы, сканируя на наличие определённых файловых шаблонов:

**Arch Linux**:
```
/arch/boot/x86_64/vmlinuz-linux
/arch/boot/x86_64/initramfs-linux.img
```

**Fedora/RHEL/Rocky**:
```
/isolinux/vmlinuz
/isolinux/initrd.img
```

**Ubuntu Desktop (casper)**:
```
/casper/vmlinuz или /casper/vmlinuz.efi
/casper/initrd или /casper/initrd.gz или /casper/initrd.lz
/casper/filesystem.squashfs
```

**Ubuntu Server Installer**:
```
/install/vmlinuz или /install.amd/vmlinuz
/install/initrd.gz или /install.amd/initrd.gz
```

**Debian Installer**:
```
/install/vmlinuz или /install.amd/vmlinuz
/install/initrd.gz или /install.amd/initrd.gz
```

**TrueNAS SCALE**:
```
/vmlinuz
/initrd.img
/live/filesystem.squashfs
/TrueNAS-SCALE.update
```

Bootimus распознаёт SCALE по файлу `TrueNAS-SCALE.update` и выбирает отдельный профиль. Извлеките ядро/initrd как обычно: профиль загружает весь ISO и монтирует его в `/cdrom` для установщика. ISO не изменяется. В установщике требуется systemd 254 или новее.

Проверено со SCALE 25.10.2.1 в QEMU UEFI с 8 ГиБ ОЗУ: установка на новый виртуальный диск и последующая загрузка с него. Во время установки весь ISO остаётся в ОЗУ. Другие версии, физическое оборудование и Secure Boot не проверены. Профиль не предназначен для TrueNAS CORE на основе FreeBSD.

Повторно извлеките ранее извлечённые образы, чтобы заменить старые параметры Debian. Затем проверьте собственные параметры загрузки.

**Proxmox VE**:
```
/boot/linux26
/boot/initrd.img
```

## Диагностика

### Извлечение упало

**Симптомы**: ошибка «Extraction failed» в админ-панели

**Типичные причины**:
1. **Битый ISO**: перекачайте ISO
2. **Не поддерживаемый ISO**: проверьте, поддерживается ли дистрибутив
3. **Место на диске**: убедитесь, что места хватает для извлечения
4. **Права**: проверьте права на data-директорию

**Отладка**:
```bash
# Посмотреть логи извлечения
docker logs bootimus | grep -i extract

# Проверить целостность ISO
sha256sum ubuntu.iso

# Проверить место
df -h /data/isos/

# Тест ручного монтирования
sudo mount -o loop ubuntu.iso /mnt
ls /mnt/casper/
sudo umount /mnt
```

### Не получилось скачать netboot

**Симптомы**: ошибка «Netboot download failed»

**Типичные причины**:
1. **Сетевая связность**: нет связи с зеркалами Debian/Ubuntu
2. **URL поменялся**: URL зеркала мог обновиться
3. **Не получилось распаковать тарбол**: повреждённое скачивание

**Решения**:
```bash
# Тест связности с зеркалом
curl -I http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz

# Проверить логи сервера
docker logs bootimus | grep -i netboot

# Вручную проверить netboot URL
wget http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz
tar -tzf netboot.tar.gz | grep vmlinuz
```

### В меню загрузки не тот тип образа

**Симптомы**: образ показывает бейдж «[kernel]», но не грузится методом kernel

**Причина**: рассинхронизация базы и файловой системы

**Решение**:
```bash
# Перезапустить извлечение kernel/initrd
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'

# Или пересканировать ISO
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### Клиент скачивает ISO несколько раз

**Симптомы**: ISO Ubuntu Desktop качается 3 раза

**Причина**: используется параметр `url=` без извлечения

**Решение**:
1. Извлеките kernel/initrd из ISO
2. Bootimus автоматически переключится на параметр `fetch=`
3. Качается только squashfs (не весь ISO)

**Проверка**:
```bash
# Проверить, что извлечено
ls -la /data/isos/ubuntu-24.04/casper/filesystem.squashfs

# Посмотреть логи сервера во время загрузки
docker logs -f bootimus
# Ищите: «fetch=...» вместо «url=...»
```

### Требуется netboot, но кнопки скачивания нет

**Симптомы**: образ показывает «Netboot Required», но кнопки скачивания нет

**Причина**: URL netboot не настроен или определение упало

**Решение**:
```bash
# Проверить детали образа
curl -u admin:password http://localhost:8081/api/images | jq '.data[] | select(.filename=="debian-13.2.0-amd64-netinst.iso")'

# Проверьте поля netboot_required и netboot_url
# Если netboot_url пустой, определение ISO могло провалиться

# Попробуйте переизвлечь
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

## Дальше

- См. [руководство по админ-консоли](admin.md) для управления образами
- Прочитайте [руководство по развёртыванию](deployment.md) для настройки хранилища
- Настройте [DHCP-сервер](dhcp.md) для PXE-загрузки
- Настройте [управление клиентами](clients.md) для контроля доступа
