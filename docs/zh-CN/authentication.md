# 身份认证指南

Bootimus 使用 JWT (JSON Web Token) 对管理面板进行身份认证。你也可以选择连接 LDAP 或 Active Directory 服务器作为认证后端。

## 目录

- [本地认证](#本地认证)
- [登录流程](#登录流程)
- [API 认证](#api-认证)
- [LDAP / Active Directory](#ldap--active-directory)
- [配置参考](#配置参考)
- [故障排查](#故障排查)

## 本地认证

默认情况下,Bootimus 使用存储在数据库中的本地用户账号(SQLite 或 PostgreSQL)。

### 默认管理员账号

首次启动时,系统会生成一个随机密码并打印到服务器日志中:

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

### 重置管理员密码

重置为一个新的随机密码(会启动完整的服务器实例):
```bash
./bootimus serve --reset-admin-password
# 或使用 Docker
docker exec bootimus /bootimus serve --reset-admin-password
```

或者直接在数据库中设置指定的密码,无需绑定任何端口(适合紧急恢复):
```bash
# 交互式提示(隐藏输入,避免密码进入 shell 历史)
./bootimus user set-password admin

# 或以非交互方式提供,便于脚本使用
./bootimus user set-password admin --password '新密码'
```

### 用户管理

可以从管理面板的 **Users** 标签创建额外的用户。每个用户具有:
- **Username**:唯一的登录名
- **Password**:以 bcrypt 哈希存储
- **Admin**:用户是否拥有管理员权限
- **Enabled**:可禁用而无需删除

也可以在不启动服务器的情况下通过 CLI 管理用户(便于恢复)。这些命令直接作用于
所配置的数据库(SQLite 或 PostgreSQL):
```bash
./bootimus user list                       # 列出所有本地用户
./bootimus user enable <username>          # 启用账号
./bootimus user disable <username>         # 禁用账号
./bootimus user set-admin <username>       # 授予管理员权限
./bootimus user unset-admin <username>     # 撤销管理员权限
./bootimus user set-password <username>    # 设置密码(交互提示,或使用 --password)
```

## 登录流程

1. 访问 `http://your-server:8081`
2. 显示登录页面,带用户名和密码字段
3. 如果配置了 LDAP,会出现一个认证下拉框用于选择后端
4. 登录成功后,签发一个 JWT token(有效期 24 小时)
5. token 存储在浏览器,并随所有 API 请求一起发送
6. 退出登录或 token 过期后,再次显示登录页

## API 认证

所有 API 端点(`/api/login` 和 `/api/auth-info` 除外)都需要有效的 JWT token。

### 获取 token

```bash
# Login and get token
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.data.token')

echo $TOKEN
```

### 使用 token

```bash
# Include in all API requests
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients

# Example: list images
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images
```

### Token 细节

- **算法**:HMAC-SHA256
- **过期时间**:签发后 24 小时
- **密钥**:每次服务器启动时随机生成(重启后所有 token 失效)
- **声明**:用户名、管理员状态、签发时间、过期时间

### 检查可用的认证后端

```bash
# No authentication required
curl http://localhost:8081/api/auth-info
```

响应:
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

Bootimus 支持把 LDAP 作为额外的认证后端。配置后,用户可以在登录页选择本地认证或 LDAP 认证。本地账号始终作为兜底。

### 工作原理

1. 用户在登录页选择 "LDAP" 并输入凭据
2. Bootimus 使用服务账号 (bind DN) 连接到 LDAP 服务器
3. 根据配置的过滤器搜索该用户
4. 尝试用提供的密码以找到的用户身份 bind
5. 若成功,检查组成员关系以确定管理员权限
6. 签发 JWT token(与本地认证相同)

### Active Directory 示例

```bash
# Environment variables
export BOOTIMUS_LDAP_HOST=dc.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="service-account-password"
export BOOTIMUS_LDAP_USER_FILTER="(sAMAccountName=%s)"
export BOOTIMUS_LDAP_GROUP_FILTER="cn=bootimus-admins"
```

### OpenLDAP 示例

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

### Docker Compose 示例

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

### 管理员组成员

如果设置了 `BOOTIMUS_LDAP_GROUP_FILTER`,仅匹配组的成员可获得管理员权限。组成员关系通过以下方式检查:

1. 用户对象上的 `memberOf` 属性
2. 如果没有 `memberOf`,则进行一次组搜索查询

如果**未设置** `BOOTIMUS_LDAP_GROUP_FILTER`,所有 LDAP 用户都获得管理员权限。

### 通过 API 用 LDAP 登录

```bash
# Specify auth_method: "ldap"
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"jdoe","password":"ldap-password","auth_method":"ldap"}' | jq -r '.data.token')
```

## 配置参考

### CLI 标志

| 标志 | 默认值 | 说明 |
|------|---------|-------------|
| `--ldap-host` | *(空)* | LDAP 服务器主机名(启用 LDAP 认证) |
| `--ldap-port` | `389` | LDAP 服务器端口 |
| `--ldap-tls` | `false` | 使用 LDAPS(连接时即 TLS) |
| `--ldap-starttls` | `false` | 连接后使用 StartTLS |
| `--ldap-skip-verify` | `false` | 跳过 TLS 证书校验 |
| `--ldap-bind-dn` | *(空)* | 用于用户搜索的服务账号 DN |
| `--ldap-bind-password` | *(空)* | 服务账号密码 |
| `--ldap-base-dn` | *(空)* | 用户搜索的 base DN |
| `--ldap-user-filter` | `(sAMAccountName=%s)` | 用户搜索过滤器(`%s` = 用户名) |
| `--ldap-group-filter` | *(空)* | 管理员组的 CN |
| `--ldap-group-base-dn` | *(空)* | 组搜索的 base DN(默认与 base DN 相同) |

### 环境变量

所有标志都可以通过带 `BOOTIMUS_` 前缀的环境变量来设置:

| 变量 | 对应标志 |
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

### 配置文件 (bootimus.yaml)

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

## 故障排查

### LDAP 连接失败

检查连通性和 TLS 设置:
```bash
# Test LDAP connection
ldapsearch -H ldap://dc.example.com -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"

# Test LDAPS
ldapsearch -H ldaps://dc.example.com:636 -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"
```

### 找不到用户

验证用户过滤器是否返回结果:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" dn
```

常见过滤器:
- Active Directory:`(sAMAccountName=%s)`
- OpenLDAP:`(uid=%s)`
- 基于邮箱:`(mail=%s)`

### LDAP 用户不是管理员

检查组成员关系:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" memberOf
```

### Token 过期

JWT token 有效期 24 小时。过期后会自动显示登录页。服务器重启时 token 也会失效(签名密钥会被重新生成)。

### 本地管理员被锁定

重置管理员密码:
```bash
./bootimus serve --reset-admin-password
```

或者,如果启动服务器不方便(例如端口冲突),可直接在数据库中设置密码:
```bash
./bootimus user set-password admin
```

无论 LDAP 配置如何,这总是可用。
