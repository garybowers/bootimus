# Руководство по управлению образами

Полное руководство по управлению ISO-образами, извлечению загрузочных файлов и работе со специальными случаями вроде Debian/Ubuntu netboot.

## Оглавление

- [Добавление образов](#добавление-образов)
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
| **TrueNAS SCALE** | Да | Нет | `/vmlinuz` + `/initrd.img` (корень) |
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
```

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
