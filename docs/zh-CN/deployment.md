#  部署指南

在各种环境中部署 Bootimus、配置网络与存储的完整指南。

##  目录

- [快速开始](#快速开始)
- [Docker 部署](#docker-部署)
- [二进制部署](#二进制部署)
- [网络配置](#网络配置)
- [存储配置](#存储配置)
- [数据库选项](#数据库选项)
- [远程更新与隐私](#远程更新与隐私)
- [生产环境部署](#生产环境部署)

## 快速开始

### Docker(推荐)

```bash
# Create data directory
mkdir -p data

# Run with SQLite (no database container needed)
docker run -d \
  --name bootimus \
  --cap-add NET_BIND_SERVICE \
  -p 69:69/udp \
  -p 8080:8080/tcp \
  -p 8081:8081/tcp \
  -v $(pwd)/data:/data \
  garybowers/bootimus:latest

# Check logs for admin password
docker logs bootimus | grep "Password"

# Access admin interface
open http://localhost:8081
```

### 独立二进制

```bash
# Download binary
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64
chmod +x bootimus-amd64

# Create data directory
mkdir -p data

# Run (SQLite mode - no database required)
./bootimus-amd64 serve

# Admin panel: http://localhost:8081
# Admin password shown in startup logs
```

## Docker 部署

### 使用 PostgreSQL 的 Docker Compose

```bash
# Clone repository
git clone https://github.com/garybowers/bootimus
cd bootimus

# Start with PostgreSQL
docker-compose up -d

# View logs
docker-compose logs -f bootimus
```

Docker Compose 栈包含:
- **Bootimus server**:主 PXE/HTTP 引导服务器
- **PostgreSQL**:用于客户端/镜像管理的数据库
- **Health checks**:自动服务监控
- **Persistent storage**:用于 ISO 和数据库的数据卷

### 目录结构

Bootimus 会自动创建子目录:
- `/data/isos/` — ISO 镜像文件和提取的引导文件(按 ISO 放在各自子目录中)
- `/data/bootloaders/` — 自定义 bootloader 文件(可选)
- `/data/bootimus.db` — SQLite 数据库(如使用 SQLite 模式)

## 网络配置

### 默认内部桥接网络

默认情况下,容器使用内部桥接网络并通过端口转发暴露服务:

```yaml
networks:
  bootimus_net:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
          gateway: 172.20.0.1
```

- **Bootimus server**:`172.20.0.3`
- **PostgreSQL**:`172.20.0.2`
- **从主机访问**:通过端口转发(例如 `localhost:8081`)

### 在局域网上拥有静态 IP 的桥接网络

对于生产 PXE 环境,你可能希望容器直接位于你的局域网上并拥有静态 IP。

#### 第 1 步:找到你的网络接口

```bash
ip addr show  # Linux
# Look for your primary interface (e.g., eth0, ens33, enp0s3)
```

#### 第 2 步:编辑 docker-compose.yml

取消注释 `host_bridge` 网络部分:

```yaml
services:
  bootimus:
    networks:
      # Comment out internal bridge
      # bootimus_net:
      #   ipv4_address: 172.20.0.3
      # Enable host bridge
      host_bridge:
        ipv4_address: 192.168.1.100  # Your desired static IP
    environment:
      BOOTIMUS_SERVER_ADDR: 192.168.1.100  # Set static server address

networks:
  # Uncomment and configure for your LAN
  host_bridge:
    driver: macvlan
    driver_opts:
      parent: eth0  # Your network interface
    ipam:
      config:
        - subnet: 192.168.1.0/24      # Your LAN subnet
          gateway: 192.168.1.1         # Your LAN gateway
          ip_range: 192.168.1.100/32   # Container static IP
```

#### 第 3 步:配置网络细节

为你的网络更新以下值:
- `parent`:你主机的网络接口(例如 `eth0`、`ens33`)
- `subnet`:你的局域网子网(例如 `192.168.1.0/24`)
- `gateway`:你路由器的 IP(例如 `192.168.1.1`)
- `ip_range`:Bootimus 的静态 IP(例如 `192.168.1.100/32`)
- `BOOTIMUS_SERVER_ADDR`:与静态 IP 相同

#### 第 4 步:启动容器

```bash
docker-compose down
docker-compose up -d
```

#### 第 5 步:验证连通性

```bash
# From another machine on the LAN
curl http://192.168.1.100:8081

# Ping the container
ping 192.168.1.100
```

###  Macvlan 网络的重要说明

- **Macvlan 网络**:容器在你的局域网中表现为独立设备
- **主机无法直接访问容器**:主机无法直接与 macvlan 容器通信。使用独立的 VM/容器进行管理访问,或在主机上创建 macvlan 接口。
- **DHCP 冲突**:确保静态 IP 在 DHCP 范围之外,或在 DHCP 服务器中预留
- **防火墙规则**:容器绕过主机防火墙 — 如需要请单独配置容器防火墙

### 从主机访问 Macvlan 容器

如果你需要从主机访问 macvlan 容器:

```bash
# Create a macvlan interface on the host
sudo ip link add macvlan0 link eth0 type macvlan mode bridge
sudo ip addr add 192.168.1.101/32 dev macvlan0
sudo ip link set macvlan0 up
sudo ip route add 192.168.1.100/32 dev macvlan0

# Now you can access the container from the host
curl http://192.168.1.100:8081
```

## 二进制部署

### 系统要求

- **操作系统**:Linux(amd64、arm64、armv7)
- **权限**:端口 69(TFTP)需要 root 权限,或使用非特权端口
- **磁盘**:10GB+ 用于 ISO 存储
- **内存**:最少 512MB,推荐 2GB+

### 安装

```bash
# Download binary for your architecture
wget https://github.com/garybowers/bootimus/releases/latest/download/bootimus-amd64

# Make executable
chmod +x bootimus-amd64

# Move to system location
sudo mv bootimus-amd64 /usr/local/bin/bootimus

# Create data directory
sudo mkdir -p /opt/bootimus/data

# Create systemd service
sudo nano /etc/systemd/system/bootimus.service
```

### Systemd 服务

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
# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable bootimus
sudo systemctl start bootimus

# Check status
sudo systemctl status bootimus

# View logs
sudo journalctl -u bootimus -f
```

## 存储配置

### 数据目录结构

```
/opt/bootimus/data/
├── isos/                           # ISO files
│   ├── ubuntu-24.04.iso           # ISO file
│   ├── ubuntu-24.04/              # Extracted boot files
│   │   ├── vmlinuz
│   │   ├── initrd
│   │   └── casper/
│   │       └── filesystem.squashfs
│   └── debian-12.iso
├── bootloaders/                    # Custom bootloaders (optional)
├── bootimus.db                     # SQLite database (if using SQLite)
└── .admin_password                 # Generated admin password
```

### 磁盘空间要求

- **ISO**:每个 ISO 1-10GB
- **提取后文件**:每个 ISO 100MB-3GB
- **数据库**:< 100MB
- **推荐**:50GB+ 用于多个 ISO

### 存储最佳实践

1. **使用 SSD**:让客户端的引导时间更快
2. **定期备份**:备份数据库和 ISO
3. **监控磁盘空间**:为低磁盘空间设置告警
4. **清理旧 ISO**:删除未使用的 ISO 以释放空间

## 数据库选项

### SQLite 模式(默认)

SQLite **默认启用** — 无需配置!

```bash
# Run with SQLite (default)
./bootimus serve

# Database automatically created at: <data_dir>/bootimus.db
```

**好处**:
-  零配置
-  单文件数据库
-  完美适用于单服务器部署
-  备份方便(只需复制文件)

**限制**:
-  比 PostgreSQL 并发性能低
-  仅支持单服务器(无集群)

### PostgreSQL 模式

适用于高并发的企业部署:

#### 配置文件方式

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

#### 环境变量方式

```bash
export BOOTIMUS_DB_HOST=postgres.example.com
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secretpassword
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=require

./bootimus serve
```

**好处**:
-  高并发
-  多服务器部署
-  高级复制
-  规模化下性能更佳

**要求**:
- PostgreSQL 12+ 服务器
- 到数据库的网络连通
- 额外的基础设施

## 远程更新与隐私

Bootimus 是自托管的,**不会**在后台悄悄回传数据。它随附一份完整的发行版和工具 profile 目录,嵌入在二进制文件中,因此在没有对外网络访问的情况下也完全可用。

Bootimus **唯一**一次联系外部服务,是在运维人员**显式**触发 profile/工具更新时 — 通过管理界面中的 "Check for Updates" 按钮、`bootimus profiles update` 命令行命令,或 `POST /api/profiles/update` 和 `POST /api/tools/update` 端点。上述每种方式都会对 GitHub 上的一个静态 JSON 文件(`raw.githubusercontent.com/garybowers/bootimus/main/...`)执行一次无需认证的 `GET` 请求,且不会发送任何系统信息或标识符。

为确保绝不发生任何远程联系(例如隔离网络 air-gapped 部署),请在启动时禁用远程更新:

```bash
bootimus serve --disable-remote-profiles
# or in bootimus.yaml:  disable_remote_profiles: true
# or via env:           BOOTIMUS_DISABLE_REMOTE_PROFILES=true
```

完整细节请参阅[发行版 Profile 指南](distro-profiles.md#远程更新与隐私)。

## 生产环境部署

### Docker + SQLite(最简)

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

### Docker Compose + PostgreSQL

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

### 配置选项

Bootimus 使用合理的默认值,只需极少配置。

#### 配置优先级

1. 命令行标志(最高优先级)
2. 环境变量(带 `BOOTIMUS_` 前缀)
3. 配置文件(`bootimus.yaml`)

#### 示例配置文件

```yaml
# bootimus.yaml
tftp_port: 69
http_port: 8080
admin_port: 8081
data_dir: ./data          # Base data directory
server_addr: ""           # Auto-detected if not specified

# Database configuration (optional)
# If no db.host is specified, SQLite is used automatically
db:
  host: localhost       # Leave empty for SQLite
  port: 5432
  user: bootimus
  password: bootimus
  name: bootimus
  sslmode: disable
```

#### 环境变量

```bash
# Server settings
export BOOTIMUS_TFTP_PORT=69
export BOOTIMUS_HTTP_PORT=8080
export BOOTIMUS_ADMIN_PORT=8081
export BOOTIMUS_DATA_DIR=/var/lib/bootimus/data
export BOOTIMUS_SERVER_ADDR=192.168.1.100

# Database settings (PostgreSQL only)
export BOOTIMUS_DB_HOST=postgres      # Empty = SQLite
export BOOTIMUS_DB_PORT=5432
export BOOTIMUS_DB_USER=bootimus
export BOOTIMUS_DB_PASSWORD=secret
export BOOTIMUS_DB_NAME=bootimus
export BOOTIMUS_DB_SSLMODE=disable

./bootimus serve
```

## 故障排查

### 端口 69 权限被拒

```bash
# Run as root
sudo ./bootimus serve

# Or use Docker with NET_BIND_SERVICE capability
docker run --cap-add NET_BIND_SERVICE ...

# Or use non-privileged port
./bootimus serve --tftp-port 6969
```

### 数据库连接失败

```bash
# Check SQLite database
ls -la data/bootimus.db

# For PostgreSQL, test connection
psql -h localhost -U bootimus -d bootimus

# Check PostgreSQL logs
docker logs bootimus-db
```

### 容器无法在局域网上被访问

```bash
# Verify macvlan configuration
docker network inspect bootimus_host_bridge

# Check IP address assignment
docker exec bootimus ip addr show

# Verify routing
ip route | grep 192.168.1.100

# Check firewall
sudo iptables -L -n | grep 192.168.1.100
```

### 磁盘空间不足

```bash
# Check disk usage
df -h /opt/bootimus/data

# Find large files
du -sh /opt/bootimus/data/*

# Clean up old ISOs
rm /opt/bootimus/data/isos/old-image.iso

# Scan for new ISOs to update database
curl -u admin:password -X POST http://localhost:8081/api/scan
```

## 下一步

-  阅读 [镜像管理指南](images.md) 了解 ISO 处理
-  查看 [管理控制台指南](admin.md) 进行管理
-  配置 [DHCP 服务器](dhcp.md) 实现 PXE 引导
