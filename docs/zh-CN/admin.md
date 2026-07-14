#  管理控制台指南

使用 Bootimus 管理界面和 REST API 的完整指南。

##  目录

- [访问管理面板](#访问管理面板)
- [仪表盘](#仪表盘)
- [客户端管理](#客户端管理)
- [镜像管理](#镜像管理)
- [引导日志](#引导日志)
- [REST API](#rest-api)
- [自动化示例](#自动化示例)
- [安全最佳实践](#安全最佳实践)

## 访问管理面板

### Web 界面

```
http://your-server:8081/
```

**要求**:
- 管理界面运行在独立端口(默认 8081)
- 兼容 SQLite 或 PostgreSQL
- 基于 JWT token 的身份认证(可选 LDAP/AD 后端)

### 首次登录

首次启动时,Bootimus 会生成一个随机管理员密码:

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

访问 `http://your-server:8081`,你会看到专门的登录页面。输入管理员凭据即可进入面板。

**登录凭据**:
- **用户名**:`admin`
- **密码**:查看服务器启动日志

如果配置了 LDAP,登录页面会出现下拉菜单,可在本地认证和 LDAP 认证之间选择。详见 [身份认证指南](authentication.md)。

### 快速开始

1. 启动 Bootimus:
   ```bash
   docker-compose up -d
   # OR
   ./bootimus serve
   ```

2. 从服务器日志中复制管理员密码

3. 浏览器打开 `http://localhost:8081/`

4. 用用户名 `admin` 和生成的密码登录

## 仪表盘

仪表盘提供实时统计:

-  **总客户端数** — 所有已注册的客户端
-  **活跃客户端** — 允许引导的已启用客户端
-  **总镜像数** — 所有 ISO 镜像
-  **已启用镜像** — 出现在引导菜单中的镜像
-  **总引导次数** — 引导尝试的次数

所有统计通过 WebSocket/SSE 实时更新。

## 客户端管理

### 添加客户端

1. 点击 **"Add Client"** 按钮
2. 输入 MAC 地址(格式:`00:11:22:33:44:55`)
3. 可选地添加名称和描述
4. 勾选 **"Enabled"** 以允许引导
5. 点击 **"Create Client"**

**通过 API**:
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

### 编辑客户端

1. 在任意客户端行点击 **"Edit"**
2. 修改名称、描述或启用状态
3. 选择该客户端可访问的 ISO(多选)
4. 点击 **"Update Client"**

**通过 API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Name",
    "enabled": false
  }'
```

### 删除客户端

在任意客户端行点击 **"Delete"** 并确认删除。

**通过 API**:
```bash
curl -u admin:password -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

### 为客户端分配镜像

**通过 Web 界面**:
1. 点击客户端的 **"Edit"**
2. 从多选下拉框中选择镜像
3. 点击 **"Update Client"**

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/clients/assign \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "image_filenames": ["ubuntu-24.04.iso", "debian-12.iso"]
  }'
```

## 镜像管理

### 上传 ISO

**通过 Web 界面**:
1. 点击 **"Upload ISO"** 按钮
2. 拖放 ISO 文件或点击浏览
3. 可选地添加描述
4. 勾选 **"Public"** 让所有客户端都可访问
5. 点击 **"Upload"**

**上传限制**:每个文件 10GB

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/upload \
  -F "file=@/path/to/ubuntu-24.04-live-server-amd64.iso" \
  -F "description=Ubuntu 24.04 LTS Server" \
  -F "public=true"
```

### 从 URL 下载

直接把 ISO 下载到服务器:

**通过 Web 界面**:
1. 点击 **"Download from URL"** 按钮
2. 输入 ISO 下载 URL
3. 添加描述
4. 点击 **"Download"**

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso",
    "description": "Ubuntu 24.04 LTS Server"
  }'

# Monitor progress
curl -u admin:password http://localhost:8081/api/downloads/progress?filename=ubuntu-24.04-live-server-amd64.iso
```

### 提取 Kernel/Initrd

提取引导文件,加快引导速度并降低带宽占用:

**通过 Web 界面**:
1. 在 **Images** 标签页中找到镜像
2. 点击 **"Extract"** 按钮
3. 等待提取完成

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/extract \
  -H "Content-Type: application/json" \
  -d '{"filename": "ubuntu-24.04.iso"}'
```

**好处**:
-  更快的引导(下载 100MB 而不是 6GB)
-  更少的带宽(对多客户端场景至关重要)
-  更好的兼容性(某些 ISO 不支持 sanboot)

详见 [镜像管理指南](images.md) 的提取说明。

### 下载 Netboot 文件

针对需要 netboot 的 Debian/Ubuntu 安装 ISO:

**通过 Web 界面**:
1. 找到带 **"Netboot Required"** 标记的镜像
2. 点击 **"Download Netboot"** 按钮
3. 等待下载和提取

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/images/netboot/download \
  -H "Content-Type: application/json" \
  -d '{"filename": "debian-13.2.0-amd64-netinst.iso"}'
```

**Netboot 文件是什么?**
- 来自 Debian/Ubuntu 官方的最小化引导文件
- 下载量约 30-50MB(而不是完整 ISO)
- 安装期间从互联网下载软件包
- 始终获取最新软件包

详见 [Netboot 支持](images.md#netboot-support)。

### 扫描 ISO

扫描数据目录以发现手动添加的 ISO:

**通过 Web 界面**:
1. 手动把 ISO 文件复制到 `/data/isos/` 目录
2. 点击 **"Scan for ISOs"** 按钮
3. Bootimus 检测并注册新的 ISO

**通过 API**:
```bash
curl -u admin:password -X POST http://localhost:8081/api/scan
```

### 启用/禁用镜像

**通过 Web 界面**:
- 点击任意镜像上的 **"Enable"** 或 **"Disable"** 按钮
- 禁用的镜像不会出现在引导菜单中

**通过 API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

### 设为公开/私有

**通过 Web 界面**:
- 点击 **"Make Public"** 让所有客户端可访问
- 点击 **"Make Private"** 仅限已分配的客户端访问

**通过 API**:
```bash
curl -u admin:password -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": true}'
```

### 删除镜像

**通过 Web 界面**:
- 点击任意镜像行的 **"Delete"**
- 确认删除
- 镜像从数据库移除
- ISO 文件保留在磁盘上(如需可手动删除)

**通过 API**:
```bash
# Delete from database only
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso"

# Delete from database and filesystem
curl -u admin:password -X DELETE "http://localhost:8081/api/images?filename=ubuntu.iso&delete_file=true"
```

## 引导日志

查看最近的引导尝试,带实时流式更新:

**显示的信息**:
-  时间戳
-  客户端 MAC 地址
-  镜像名称
-  IP 地址
- / 成功/失败状态
-  错误消息(如有)

**自动刷新**:日志通过 SSE (Server-Sent Events) 实时更新

**通过 API**:
```bash
# Get last 100 logs (default)
curl -u admin:password http://localhost:8081/api/logs

# Get last 10 logs
curl -u admin:password http://localhost:8081/api/logs?limit=10

# Get last 500 logs (max 1000)
curl -u admin:password http://localhost:8081/api/logs?limit=500
```

## REST API

所有管理功能均可通过 REST API 进行自动化。

### 身份认证

所有端点都要求 HTTP Basic 认证:
- **用户名**:`admin`
- **密码**:首次运行时自动生成

```bash
curl -u admin:your-password http://localhost:8081/api/stats
```

### API 端点

#### Stats

```bash
GET /api/stats
```

**响应**:
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

| 方法 | 端点 | 说明 |
|--------|----------|-------------|
| `GET` | `/api/clients` | 列出所有客户端 |
| `GET` | `/api/clients?mac=<MAC>` | 按 MAC 获取客户端 |
| `POST` | `/api/clients` | 创建客户端 |
| `PUT` | `/api/clients?mac=<MAC>` | 更新客户端 |
| `DELETE` | `/api/clients?mac=<MAC>` | 删除客户端 |
| `POST` | `/api/clients/assign` | 为客户端分配镜像 |

#### Images

| 方法 | 端点 | 说明 |
|--------|----------|-------------|
| `GET` | `/api/images` | 列出所有镜像 |
| `GET` | `/api/images?filename=<name>` | 获取镜像 |
| `PUT` | `/api/images?filename=<name>` | 更新镜像 |
| `DELETE` | `/api/images?filename=<name>` | 删除镜像 |
| `POST` | `/api/images/upload` | 上传 ISO |
| `POST` | `/api/images/download` | 从 URL 下载 ISO |
| `POST` | `/api/images/extract` | 提取 kernel/initrd |
| `POST` | `/api/images/netboot/download` | 下载 netboot 文件 |
| `POST` | `/api/scan` | 扫描新 ISO |

#### Downloads

| 方法 | 端点 | 说明 |
|--------|----------|-------------|
| `GET` | `/api/downloads` | 列出进行中的下载 |
| `GET` | `/api/downloads/progress?filename=<name>` | 获取下载进度 |

#### Logs

| 方法 | 端点 | 说明 |
|--------|----------|-------------|
| `GET` | `/api/logs?limit=<N>` | 获取引导日志 |
| `GET` | `/api/logs/stream` | 实时日志 SSE 流 |

## 自动化示例

### 批量添加客户端

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

### 将所有镜像设为公开

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

### 监控引导尝试

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

### 导出统计数据

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

## 安全最佳实践

### 网络隔离

将管理端口与引导网络分开:

```bash
# Allow boot traffic (TFTP/HTTP) on one interface
# Allow admin traffic on different interface or localhost only
```

### 防火墙规则

```bash
# Allow admin access only from specific IP range
sudo ufw allow from 192.168.1.0/24 to any port 8081

# Or block admin port from external access entirely
sudo ufw deny 8081
```

### SSH 隧道

通过 SSH 隧道安全访问管理界面:

```bash
# Create SSH tunnel
ssh -L 8081:localhost:8081 user@bootimus-server

# Access admin panel
open http://localhost:8081/
```

### VPN 访问

- 让 Bootimus 管理端口仅暴露在 VPN 网络内
- 要求管理访问通过 VPN 连接
- 把引导端口(69、8080)放在独立网段

### 密码管理

-  把管理员密码安全保存(用密码管理器)
-  通过删除 `.admin_password` 并重启来定期轮换密码
- 🛡 考虑额外的认证层(nginx 加客户端证书)

## 故障排查

### 管理界面无法加载

```bash
# Check service is running
docker ps | grep bootimus

# Check logs
docker logs bootimus

# Verify port is accessible
curl -u admin:password http://localhost:8081/api/stats

# Check firewall
sudo ufw status | grep 8081
```

### 无法上传大型 ISO

```bash
# Check available disk space
df -h /opt/bootimus/data

# Upload limit is 10GB by default
# For larger ISOs, use download from URL or manual copy + scan
```

### 改动未生效

- 强制刷新浏览器(Ctrl+F5 或 Cmd+Shift+R)
- 查看浏览器控制台报错(F12)
- 用 curl 验证 API 响应
- 查看服务器日志的详细错误

### API 返回错误

```bash
# Check request format (JSON content-type for POST/PUT)
curl -v -u admin:password -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","name":"Test"}'

# Verify resource exists for update/delete
curl -u admin:password http://localhost:8081/api/images | jq

# Check server logs
docker logs bootimus | tail -50
```

## 下一步

-  阅读 [镜像管理指南](images.md) 了解 ISO 处理
-  查看 [部署指南](deployment.md) 进行生产环境配置
-  配置 [DHCP 服务器](dhcp.md) 实现 PXE 引导
-  设置 [客户端管理](clients.md) 进行访问控制
