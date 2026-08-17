# Руководство по настройке DHCP

Полное руководство по настройке различных DHCP-серверов для работы с Bootimus и сетевой PXE-загрузки.

## Оглавление

- [Встроенный proxyDHCP (standalone-режим)](#встроенный-proxydhcp-standalone-режим)
- [Обзор](#обзор)
- [ISC DHCP Server](#isc-dhcp-server)
- [Dnsmasq](#dnsmasq)
- [MikroTik RouterOS](#mikrotik-routeros)
- [Ubiquiti EdgeRouter](#ubiquiti-edgerouter)
- [pfSense](#pfsense)
- [OPNsense](#opnsense)
- [Windows Server DHCP](#windows-server-dhcp)
- [Диагностика](#диагностика)
- [PiHole](#pi-hole-dnsmasq)

## Встроенный proxyDHCP (standalone-режим)

Bootimus включает встроенный proxyDHCP-ответчик (RFC 4578). Когда он включён, Bootimus сам отвечает на PXE-специфичные DHCP-опции — **вашему существующему DHCP-серверу вообще не нужна настройка PXE**. Он продолжает раздавать IP как обычно; Bootimus отвечает PXE-клиентам только `next-server`, bootfile и PXE vendor class — и никогда не предлагает собственный IP-адрес.

Это самый простой способ запустить Bootimus в любом окружении, где вы не владеете основным DHCP-сервером или не хотите его трогать.

### Как это работает

1. Клиент шлёт широковещательный `DHCPDISCOVER`.
2. Существующий DHCP в LAN отвечает IP-лизой (PXE-инфо не нужна).
3. Bootimus отвечает на тот же broadcast только PXE boot-инфой — без IP.
4. PXE ROM клиента объединяет оба ответа: IP от основного DHCP, bootfile от Bootimus.

Поскольку Bootimus никогда не выдаёт IP, конфликта с существующим DHCP-сервером нет и пул лиз не нужно координировать.

### Включение

```bash
# Флаг CLI
bootimus serve --proxy-dhcp

# Переменная окружения
BOOTIMUS_PROXY_DHCP_ENABLED=true

# YAML-конфиг
proxy_dhcp:
  enabled: true
  bootfile_bios: undionly.kpxe           # legacy BIOS PXE
  bootfile_uefi: bootimus.efi            # UEFI x64
  bootfile_arm64: bootimus-arm64.efi     # UEFI ARM64
```

Выключено по умолчанию, чтобы существующие установки с dnsmasq/ISC-DHCP/etc. не получили сюрприза.

### Требования и оговорки

- **Биндит UDP/67 и UDP/4011.** UDP/67 требует `CAP_NET_BIND_SERVICE` или root; UDP/4011 — порт обнаружения PXE boot-сервера, к которому некоторые UEFI ROM (особенно AMI/Supermicro) обращаются после первоначального оффера. В Docker дефолтный образ уже запускается от root, так что дополнительной capability не нужно; добавляйте `--cap-add NET_BIND_SERVICE` только для rootless-установок.
- **Тот же широковещательный домен.** proxyDHCP опирается на то, что видит DHCP-бродкасты клиентов. Работает в плоской LAN или одной VLAN. Если ваша сеть использует DHCP relay (`ip helper-address`) для проброски DHCP через VLAN, relay перенаправляет к основному DHCP, но не к Bootimus — добавьте Bootimus как дополнительную цель relay или держите цели в той же VLAN, что и Bootimus.
- **Сеть в Docker.** Используйте `macvlan`, `ipvlan` или `network_mode: host`, чтобы контейнер стал полноценным участником широковещательного домена. Дефолтная bridge-сеть не подойдёт — бродкасты застрянут внутри `docker0`.
- **Два proxyDHCP-сервера в одной LAN — это законно, но отладка — кошмар.** Если вы включаете встроенный proxyDHCP Bootimus, отключите любой существующий dnsmasq proxyDHCP, раздающий PXE в той же сети.

### Пример docker-compose

```yaml
services:
  bootimus:
    image: garybowers/bootimus:latest
    environment:
      BOOTIMUS_PROXY_DHCP_ENABLED: "true"
      BOOTIMUS_SERVER_ADDR: 10.76.42.41    # macvlan-IP контейнера
    ports:
      - "67:67/udp"                         # proxyDHCP
      - "4011:4011/udp"                     # обнаружение PXE boot-сервера
      - "69:69/udp"                         # TFTP
      - "8080:8080/tcp"
      - "8081:8081/tcp"
    networks:
      lan:
        ipv4_address: 10.76.42.41
networks:
  lan:
    driver: macvlan
    driver_opts:
      parent: eth0
    ipam:
      config:
        - subnet: 10.76.42.0/24
```

### Проверка

Логи контейнера должны показать:

```
proxyDHCP: listening on UDP/67 + UDP/4011, advertising next-server=10.76.42.41 (BIOS=undionly.kpxe, UEFI=bootimus.efi, ARM64=bootimus-arm64.efi)
```

И построчно при PXE-загрузке:

```
proxyDHCP: DISCOVER -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
proxyDHCP: REQUEST  -> 6c:24:08:0c:bb:6b arch=7 bootfile=bootimus.efi
TFTP: Client requesting file: bootimus.efi
```

Панель **Server Information** в админ-UI также показывает текущее состояние proxyDHCP.

---

## Обзор

Чтобы включить сетевую PXE-загрузку, ваш DHCP-сервер должен быть настроен:

1. **Выдавать IP-адреса** клиентам (стандартный DHCP)
2. **Указывать на boot-сервер** (`next-server` или DHCP option 66)
3. **Указывать имя файла загрузчика** (DHCP option 67)
4. **Определять iPXE** и переключаться на HTTP-меню (опционально, но рекомендуется)

**Замените `192.168.1.10` на IP-адрес вашего сервера Bootimus во всех примерах ниже.**

### Имена файлов загрузчиков

| Тип клиента | Имя файла в DHCP | Заметки |
|-------------|---------------|-------|
| UEFI (x86_64) | `bootimus.efi` (или `ipxe.efi`) | Кастомно собранный iPXE со встроенным скриптом |
| UEFI (ARM64) | `bootimus-arm64.efi` (или `ipxe-arm64.efi`) | Кастомно собранный iPXE со встроенным скриптом |
| Legacy BIOS | `undionly.kpxe` | Стандартный PXE-загрузчик |

> **Secure Boot:** для клиентов с включённым Secure Boot активируйте встроенный набор загрузчиков `secureboot-official` и анонсируйте вместо обычного UEFI-bootfile `ipxe-shimx64.efi` (x86_64) или `ipxe-shimaa64.efi` (ARM64). Встроенный proxyDHCP делает это автоматически, когда набор активен. См. [руководство по Secure Boot](secure-boot.md).

### Поток загрузки

```
Клиент → DHCP-запрос
       ← DHCP-оффер (IP, next-server, имя файла загрузчика)
Клиент → TFTP-запрос загрузчика (bootimus.efi или undionly.kpxe)
       ← Загрузчик скачан
Клиент → HTTP-запрос menu.ipxe
       ← Загрузочное меню отображается
Клиент → Загрузка выбранного ISO
```

## ISC DHCP Server

ISC DHCP — стандартный DHCP-сервер в большинстве Linux-дистрибутивов.

### Конфигурация

Отредактируйте `/etc/dhcp/dhcpd.conf`:

```conf
# Базовая конфигурация DHCP
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    option routers 192.168.1.1;
    option domain-name-servers 8.8.8.8, 8.8.4.4;

    # Boot-сервер PXE
    next-server 192.168.1.10;  # IP сервера Bootimus

    # Определить, что клиент уже работает в iPXE
    if exists user-class and option user-class = "iPXE" {
        # У клиента iPXE — переключаем на HTTP-меню
        filename "http://192.168.1.10:8080/menu.ipxe";
    }
    # UEFI-системы (x86_64)
    elsif option arch = 00:07 {
        filename "ipxe.efi";
    }
    # UEFI-системы (альтернативный вариант)
    elsif option arch = 00:09 {
        filename "ipxe.efi";
    }
    # Legacy BIOS
    else {
        filename "undionly.kpxe";
    }
}
```

### Продвинутое: конфигурация под конкретного клиента

```conf
subnet 192.168.1.0 netmask 255.255.255.0 {
    range 192.168.1.100 192.168.1.200;
    next-server 192.168.1.10;

    # Конфигурация конкретного клиента
    host lab-server-1 {
        hardware ethernet 00:11:22:33:44:55;
        fixed-address 192.168.1.50;
        filename "ipxe.efi";
    }

    # Конфигурация по умолчанию для остальных
    if exists user-class and option user-class = "iPXE" {
        filename "http://192.168.1.10:8080/menu.ipxe";
    }
    elsif option arch = 00:07 or option arch = 00:09 {
        filename "ipxe.efi";
    }
    else {
        filename "undionly.kpxe";
    }
}
```

### Перезапуск сервиса

```bash
# Проверить конфигурацию
sudo dhcpd -t -cf /etc/dhcp/dhcpd.conf

# Перезапустить DHCP-сервис
sudo systemctl restart isc-dhcp-server

# Проверить статус
sudo systemctl status isc-dhcp-server

# Посмотреть логи
sudo journalctl -u isc-dhcp-server -f
```

## Dnsmasq

Dnsmasq — лёгкий DHCP- и DNS-сервер, популярен на встраиваемых системах и роутерах.

### Конфигурация

Отредактируйте `/etc/dnsmasq.conf`:

```conf
# Диапазон DHCP
dhcp-range=192.168.1.100,192.168.1.200,12h

# Шлюз по умолчанию
dhcp-option=3,192.168.1.1

# DNS-серверы
dhcp-option=6,8.8.8.8,8.8.4.4

# Конфигурация PXE-загрузки
dhcp-boot=tag:!ipxe,undionly.kpxe,192.168.1.10
dhcp-boot=tag:ipxe,http://192.168.1.10:8080/menu.ipxe

# Поддержка UEFI
dhcp-match=set:efi-x86_64,option:client-arch,7
dhcp-match=set:efi-x86_64,option:client-arch,9
dhcp-boot=tag:efi-x86_64,tag:!ipxe,ipxe.efi,192.168.1.10

# Поддержка Legacy BIOS
dhcp-match=set:bios,option:client-arch,0
dhcp-boot=tag:bios,tag:!ipxe,undionly.kpxe,192.168.1.10

# Включить TFTP-сервер (опционально, если dnsmasq используется как TFTP-сервер)
# enable-tftp
# tftp-root=/var/lib/tftpboot
```

### Минимальная конфигурация

Если нужен только базовый PXE без определения iPXE:

```conf
dhcp-range=192.168.1.100,192.168.1.200,12h
dhcp-option=3,192.168.1.1
dhcp-option=6,8.8.8.8

# TFTP-сервер и boot-файл
dhcp-boot=undionly.kpxe,192.168.1.10
```

### Перезапуск сервиса

```bash
# Проверить конфигурацию
sudo dnsmasq --test

# Перезапустить сервис
sudo systemctl restart dnsmasq

# Проверить статус
sudo systemctl status dnsmasq

# Посмотреть логи
sudo journalctl -u dnsmasq -f
```

## MikroTik RouterOS

MikroTik-роутеры популярны для сетевой загрузки благодаря гибкости и производительности.

### Через веб-интерфейс (WebFig)

#### 1. Определите DHCP-опции
* Откройте **IP** > **DHCP Server** > **Options**.
* Нажмите **Add New** для каждого из следующих (значение **обязательно** в **одинарных кавычках**):
    * **Option 66 (Server)**: Name: `tftp-server` | Code: `66` | Value: `'<BOOT_SERVER_IP>'`.
    * **Option 67 (BIOS)**: Name: `boot-bios` | Code: `67` | Value: `'undionly.kpxe'`.
    * **Option 67 (UEFI)**: Name: `boot-uefi` | Code: `67` | Value: `'ipxe.efi'`.

#### 2. Создайте Option Sets
* Откройте **IP** > **DHCP Server** > **Option Sets**.
* **BIOS Set**: нажмите **Add New**, Name: `set-bios`, добавьте `tftp-server` и `boot-bios`.
* **UEFI Set**: нажмите **Add New**, Name: `set-uefi`, добавьте `tftp-server` и `boot-uefi`.

#### 3. Настройте Option Matcher (логика определения)
* Откройте **IP** > **DHCP Server** > **Option Matcher**.
* **BIOS Entry**: Name: `match-bios` | Code: `93` | Value: `0x0000` | Option Set: `set-bios` | Server: `<DHCP_SERVER_NAME>`.
* **UEFI Entry**: Name: `match-uefi-7` | Code: `93` | Value: `0x0007` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.
* **UEFI Alt Entry**: Name: `match-uefi-9` | Code: `93` | Value: `0x0009` | Option Set: `set-uefi` | Server: `<DHCP_SERVER_NAME>`.

#### 4. Конфигурация сети DHCP
* Откройте **IP** > **DHCP Server** > **Networks**.
* Откройте запись вашей подсети (например, `192.168.88.0/24`).
* **Next Server**: введите ваш `<BOOT_SERVER_IP>`.
* **Boot File Name**: **ОСТАВЬТЕ ПУСТЫМ** (Option Matcher динамически подставляет имя файла).

---

### Через CLI



Замените плейсхолдеры `<BOOT_SERVER_IP>`, `<DHCP_SERVER_NAME>` и `<YOUR_SUBNET>` своими значениями перед запуском.

```routeros
# 1. Определить DHCP-опции
/ip dhcp-server option
add code=66 name=tftp-server value="'<BOOT_SERVER_IP>'"
add code=67 name=boot-bios value="'undionly.kpxe'"
add code=67 name=boot-uefi value="'ipxe.efi'"

# 2. Создать Option Sets
/ip dhcp-server option sets
add name=set-bios options=tftp-server,boot-bios
add name=set-uefi options=tftp-server,boot-uefi

# 3. Создать Option Matchers для определения архитектуры
/ip dhcp-server option-matcher
add code=93 name=match-bios option-set=set-bios server=<DHCP_SERVER_NAME> value=0x0000
add code=93 name=match-uefi-7 option-set=set-uefi server=<DHCP_SERVER_NAME> value=0x0007
add code=93 name=match-uefi-9 option-set=set-uefi server=<DHCP_SERVER_NAME> value=0x0009

# 4. Применить к сети DHCP
/ip dhcp-server network
set [find address="<YOUR_SUBNET>"] boot-file-name="" next-server=<BOOT_SERVER_IP>
```

## Ubiquiti EdgeRouter

Ubiquiti EdgeRouter использует EdgeOS (на базе Vyatta/VyOS).

### Через веб-UI

1. Откройте **Services > DHCP Server**
2. Выберите ваш DHCP-сервер (например, `LAN`)
3. В **Actions** нажмите **Edit**
4. Прокрутите до **PXE Settings**:
   - **Boot File**: `undionly.kpxe` (BIOS) или `ipxe.efi` (UEFI)
   - **Boot Server**: `192.168.1.10`
5. Нажмите **Save**

### Через CLI

```bash
configure

# Задать TFTP-сервер для сетевой загрузки
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-server 192.168.1.10
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 bootfile-name undionly.kpxe

# Продвинутое: поддержка UEFI
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "option arch code 93 = unsigned integer 16;"
set service dhcp-server shared-network-name LAN subnet 192.168.1.0/24 subnet-parameters "if option arch = 00:07 { filename &quot;ipxe.efi&quot;; } else { filename &quot;undionly.kpxe&quot;; }"

commit
save
exit
```

**Замечание**: замените `LAN` на ваше актуальное имя shared network, если оно другое.

### Проверка конфигурации

```bash
show service dhcp-server
show service dhcp-server leases
```

## pfSense

pfSense — популярный open-source файрвол и роутер-дистрибутив.

### Шаги настройки

1. Откройте **Services > DHCP Server**
2. Выберите интерфейс (например, **LAN**)
3. Прокрутите до раздела **Network Booting**
4. Настройте:
   - **Enable Network Booting**: галочка
   - **Next Server**: `192.168.1.10`
   - **Default BIOS Filename**: `undionly.kpxe`
   - **UEFI 64-bit Filename**: `ipxe.efi`
5. Нажмите **Save**

### Продвинутое: пользовательские опции

Для определения iPXE добавьте пользовательские DHCP-опции:

1. Откройте **Services > DHCP Server**
2. Выберите интерфейс
3. Прокрутите до **Additional BOOTP/DHCP Options**
4. Добавьте опции:

```
# Option 60 (Class Identifier)
60 text "PXEClient"

# Option 66 (TFTP Server)
66 text "192.168.1.10"

# Option 67 (Bootfile Name)
67 text "undionly.kpxe"
```

### Статические DHCP-привязки

Для конкретных клиентов:

1. **Services > DHCP Server > LAN**
2. Прокрутите до **DHCP Static Mappings**
3. Нажмите **Add**
4. Настройте:
   - **MAC Address**: `00:11:22:33:44:55`
   - **IP Address**: `192.168.1.50`
   - **Filename**: `ipxe.efi`
   - **Root Path**: оставьте пустым
5. Нажмите **Save**

## OPNsense

OPNsense — форк pfSense с современным интерфейсом.

### Шаги настройки

1. Откройте **Services > DHCPv4 > [Interface]**
2. Прокрутите до **Network Booting**
3. Настройте:
   - **Enable Network Booting**: галочка
   - **Next Server**: `192.168.1.10`
   - **Default BIOS Filename**: `undionly.kpxe`
   - **UEFI 64-bit Filename**: `ipxe.efi`
4. Нажмите **Save**
5. Нажмите **Apply Changes**

### Продвинутая конфигурация

1. Откройте **Services > DHCPv4 > [Interface]**
2. Перейдите на вкладку **Additional Options**
3. Добавьте пользовательские опции, аналогично pfSense

## Windows Server DHCP

DHCP-сервис Windows Server.

### Шаги настройки

1. Откройте **DHCP Manager** (`dhcpmgmt.msc`)
2. Разверните ваш DHCP-сервер
3. Разверните **IPv4**
4. Правый клик по **Scope** → **Scope Options**
5. Настройте:
   - **066 Boot Server Host Name**: `192.168.1.10`
   - **067 Bootfile Name**: `undionly.kpxe`

### Продвинутое: определение UEFI и BIOS

1. Правый клик по **Scope** → **Set Predefined Options**
2. Нажмите **Add**
3. Создайте option code 60 (Vendor Class):
   - **Code**: 60
   - **Name**: Vendor Class
   - **Data Type**: String
4. Создайте политики для UEFI/BIOS:
   - Правый клик по **Policies** → **New Policy**
   - **Condition**: Vendor Class equals "PXEClient:Arch:00007" (UEFI)
   - **Options**: задайте bootfile = `ipxe.efi`
   - Повторите для BIOS (Arch:00000) с `undionly.kpxe`

### Конфигурация через PowerShell

```powershell
# Установить опции DHCP scope
Set-DhcpServerv4OptionValue -ScopeId 192.168.1.0 -OptionId 66 -Value "192.168.1.10"
Set-DhcpServerv4OptionValue -ScopeId 192.168.1.0 -OptionId 67 -Value "undionly.kpxe"

# Создать политику для UEFI
Add-DhcpServerv4Policy -Name "UEFI" -Condition OR -VendorClass EQ "PXEClient:Arch:00007"
Set-DhcpServerv4OptionValue -PolicyName "UEFI" -OptionId 67 -Value "ipxe.efi"

# Создать политику для BIOS
Add-DhcpServerv4Policy -Name "BIOS" -Condition OR -VendorClass EQ "PXEClient:Arch:00000"
Set-DhcpServerv4OptionValue -PolicyName "BIOS" -OptionId 67 -Value "undionly.kpxe"
```

## Диагностика

### Клиент не получает DHCP-оффер

```bash
# Посмотреть логи DHCP-сервера
sudo journalctl -u isc-dhcp-server -f   # ISC DHCP
sudo journalctl -u dnsmasq -f           # Dnsmasq

# Убедиться, что DHCP-сервер запущен
sudo systemctl status isc-dhcp-server
sudo systemctl status dnsmasq

# Проверить сетевую связность
ping 192.168.1.10

# Захват DHCP-трафика
sudo tcpdump -i eth0 port 67 or port 68
```

### Клиент не скачивает загрузчик

```bash
# Проверить, что TFTP-сервер Bootimus запущен
sudo netstat -ulnp | grep :69

# Тест TFTP вручную
tftp 192.168.1.10
> get undionly.kpxe
> quit

# Посмотреть логи Bootimus
docker logs bootimus | grep TFTP
```

### iPXE грузится, но нет меню

```bash
# Проверить, что HTTP-сервер запущен
curl http://192.168.1.10:8080/menu.ipxe

# Проверить, что ISO есть
curl -u admin:password http://192.168.1.10:8081/api/images

# Убедиться, что у клиента есть доступ к HTTP-порту
telnet 192.168.1.10 8080

# Посмотреть логи Bootimus
docker logs bootimus -f
```

### Не тот загрузчик (UEFI vs BIOS)

```bash
# Проверить режим прошивки клиента в логах DHCP
sudo journalctl -u isc-dhcp-server | grep -i "arch"

# UEFI-клиенты шлют option 93 со значением 00:07 или 00:09
# BIOS-клиенты шлют option 93 со значением 00:00

# Убедитесь, что конфигурация DHCP корректно обрабатывает определение архитектуры
```

### Клиент грузится, но показывает «No Bootable Device»

Возможные причины:
1. **DHCP option 67 неверная**: должна быть `undionly.kpxe` или `ipxe.efi`
2. **Не задан next-server**: DHCP option 66 должна указывать на Bootimus
3. **Заблокирован TFTP-порт**: файрвол блокирует порт 69
4. **iPXE chainloading упал**: HTTP-порт 8080 недоступен

**Решение**:
```bash
# Убедитесь, что все порты доступны
sudo ufw allow 69/udp    # TFTP
sudo ufw allow 8080/tcp  # HTTP boot
sudo ufw allow 8081/tcp  # Admin (опционально)

# Проверьте, что Bootimus слушает
sudo netstat -tulpn | grep -E '69|8080|8081'
```

### Конфликты DHCP-серверов

Если в сети несколько DHCP-серверов:

```bash
# Найти все DHCP-серверы
sudo nmap --script broadcast-dhcp-discover

# Отключить конфликтующие DHCP-серверы
# Или настроить DHCP relay/helper по необходимости
```

## Дальше

- Настройте [управление образами](images.md), чтобы добавить ISO
- Настройте [админ-консоль](admin.md) для управления
- Настройте [управление клиентами](clients.md) для контроля доступа


## Pi-hole (dnsmasq)

Pi-hole использует `dnsmasq` как DHCP-движок, что позволяет гранулярно определять архитектуру через файлы конфигурации.

### Через веб-интерфейс

#### 1. Включить DHCP
* Откройте **Settings** > **DHCP**.
* Поставьте галочку **DHCP server enabled**.
* Задайте **IP range**, **Gateway** и **Lease duration**.
* *Замечание: подробные PXE-опции недоступны в веб-UI и должны настраиваться через CLI.*

---

### Через CLI



Чтобы поддерживать BIOS и UEFI одновременно, нужно создать пользовательский файл конфигурации в директории `dnsmasq.d`.

#### 1. Создайте конфиг
* Откройте терминал на Pi-hole и создайте файл:
    `sudo nano /etc/dnsmasq.d/07-pxe.conf`

#### 2. Опишите логику
* Вставьте следующий блок в файл, заменив `<BOOT_SERVER_IP>` на ваш реальный IP сервера:

```bash
# 1. Определить архитектуру клиента (Option 93)
dhcp-match=set:bios,option:client-arch,0
dhcp-match=set:efi-x64,option:client-arch,7
dhcp-match=set:efi-x64-alt,option:client-arch,9

# 2. Задать IP TFTP-сервера (Option 66)
dhcp-option=option:server-ip,<BOOT_SERVER_IP>

# 3. Назначить имена файлов по архитектуре (Option 67)
dhcp-boot=tag:bios,undionly.kpxe,,<BOOT_SERVER_IP>
dhcp-boot=tag:efi-x64,ipxe.efi,,<BOOT_SERVER_IP>
dhcp-boot=tag:efi-x64-alt,ipxe.efi,,<BOOT_SERVER_IP>

# 4. Опционально: определения PXE Menu/Service
pxe-service=tag:bios,x86PC,"Network Boot BIOS",undionly.kpxe,<BOOT_SERVER_IP>
pxe-service=tag:efi-x64,x86-64_EFI,"Network Boot UEFI",ipxe.efi,<BOOT_SERVER_IP>
pxe-service=tag:efi-x64-alt,x86-64_EFI,"Network Boot UEFI (Alt)",ipxe.efi,<BOOT_SERVER_IP>

```
