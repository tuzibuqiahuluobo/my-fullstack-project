# my-backend

> **语言：** [English](README.md) | 简体中文

基于 Go 的轻量级 HTTP 后端，提供用户注册、登录与资料更新 API。

## 功能特性

- 用户注册，密码使用 bcrypt 加密存储，支持邮箱验证（QQ & Gmail）
- 用户登录（返回 `uid`, `avatar`, `username`, `nickname`）
- 资料更新（用户名、头像、密码、昵称 — 各字段可选更新，用户名更新有 180 天时间限制）
- 帖子管理（发布、获取全部、作者或管理员删除）
- 用户管理（获取全部用户、管理员强制注销用户）
- 注册时自动生成默认昵称和头像
- 首次运行时自动创建超级管理员账户（如果不存在）

## 技术栈

| 依赖 | 用途 |
|------|------|
| [Go](https://go.dev/) 1.26.4+ | 运行时 |
| `net/http` | HTTP 服务器与路由 |
| [GORM](https://gorm.io/) | ORM 与数据库迁移 |
| [glebarez/sqlite](https://github.com/glebarez/sqlite) | 纯 Go SQLite 驱动（无需 CGO） |
| [golang.org/x/crypto/bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt) | 密码哈希 |

## 快速开始

### 环境要求

- Go 1.26.4 或更高版本

### 安装与运行

```bash
git clone https://github.com/tuzibuqiahuluobo/my-fullstack-project
cd my-fullstack-project
go mod download
go run main.go
```

服务器在 `http://localhost:8080` 启动，控制台会输出：

```
🚀 服务器已启动！运行在 http://localhost:8080
```

首次运行时，会在项目根目录自动创建 `data.db` SQLite 数据库文件。

## 数据模型

`models.go` 中的 `User` 结构体：

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `uid` | uint | 主键，自增 |
| `username` | string | 唯一用户名 |
| `password_hash` | string | bcrypt 哈希（不通过 API 暴露） |
| `avatar` | string | 头像 URL，未提供时由 DiceBear 自动生成 |
| `email` | string | 用户邮箱，用于注册和验证 |
| `role` | int | 用户角色 (0: 普通用户, 2: 超级管理员) |
| `username_updated_at` | time.Time | 上次修改用户名的时间戳（180 天内限制修改一次） |
| `nickname` | string | 用户显示昵称，默认为 `user_{UID}` |

`models.go` 中的 `Post` 结构体：

| 字段 | 类型 | 说明 |
|-------|------|-------------|
| `id` | uint | 主键，自增 |
| `username` | string | 帖子作者的用户名 |
| `nickname` | string | 帖子作者的昵称（创建时缓存） |
| `avatar` | string | 帖子作者的头像 URL（创建时缓存） |
| `content` | string | 帖子内容 |
| `created_at` | time.Time | 帖子创建时间戳 |
## API 文档

所有接口均使用 `POST` 请求，`Content-Type: application/json`。已开启 CORS（`Access-Control-Allow-Origin: *`）。

### 注册

**`POST /api/register`**

请求体：

```json
{
  "username": "alice",
  "password": "secret123",
  "email": "test@qq.com",
  "code": "123456"
}
```

成功响应（`200`）：

```json
{"message": "注册成功！欢迎加入。"}
```

错误响应：

- `400` — JSON 格式错误，用户名或邮箱已被注册
- `401` — 验证码已过期或错误
- `500` — 密码哈希失败

```bash
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret123","email":"test@qq.com","code":"123456"}'
```

### 登录

**`POST /api/login`**

请求体：

```json
{
  "username": "alice",
  "password": "secret123"
}
```

成功响应（`200`）：

```json
{
  "message": "登录成功！欢迎回来，alice",
  "uid": 1,
  "avatar": "https://api.dicebear.com/7.x/adventurer/svg?seed=user_1",
  "username": "alice",
  "nickname": "user_1"
}
```

错误响应（`401`）：

- 用户名不存在
- 密码错误

```bash
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret123"}'
```

### 更新资料

**`POST /api/update`**

请求体：

```json
{
  "uid": 1,
  "username": "alice_new",
  "avatar": "https://example.com/avatar.png",
  "password": "newpassword",
  "nickname": "爱丽丝仙境",
  "current_password": "secret123" 
}
```

`username`、`avatar`、`password`、`nickname` 均为可选字段。修改 `username` 时必须提供 `current_password`，且 180 天内仅可修改一次。

成功响应（`200`）：

```json
{"message": "资料更新成功！核心凭证已同步。"}
```

错误响应：

- `400` — JSON 格式错误，用户名已被占用
- `401` — 安全验证失败（当前密码错误）
- `403` — 用户名更新受限（180 天），或权限不足
- `404` — 找不到该用户
- `500` — 更新失败（数据库错误，密码哈希错误）

```bash
curl -X POST http://localhost:8080/api/update \
  -H "Content-Type: application/json" \
  -d '{"uid":1,"nickname":"爱丽丝仙境","current_password":"secret123"}'
```

### 发送验证码

**`POST /api/send-code`**

请求体：

```json
{
  "email": "test@qq.com"
}
```

成功响应（`200`）：

```json
{"message": "验证码发送成功，请注意查收！"}
```

错误响应：

- `400` — JSON 格式错误或不支持的邮箱域名（仅支持 QQ 和 Gmail）
- `500` — 邮件发送失败

```bash
curl -X POST http://localhost:8080/api/send-code \
  -H "Content-Type: application/json" \
  -d '{"email":"test@qq.com"}'
```

### 获取所有帖子

**`GET /api/posts`**

无请求体。

成功响应（`200`）：

```json
[
  {
    "id": 1,
    "username": "alice",
    "nickname": "爱丽丝仙境",
    "avatar": "https://example.com/avatar.png",
    "content": "你好，世界！",
    "created_at": "2023-10-27T10:00:00Z"
  }
]
```

错误响应：

- `500` — 数据库错误

```bash
curl http://localhost:8080/api/posts
```

### 发布帖子

**`POST /api/create-post`**

请求体：

```json
{
  "username": "alice",
  "nickname": "爱丽丝仙境",
  "avatar": "https://example.com/avatar.png",
  "content": "这是一篇新帖子。"
}
```

成功响应（`200`）：

```json
{"message": "发布成功！"}
```

错误响应：

- `400` — JSON 格式错误或内容为空
- `500` — 数据库错误

```bash
curl -X POST http://localhost:8080/api/create-post \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","nickname":"爱丽丝仙境","avatar":"https://example.com/avatar.png","content":"这是一篇新帖子。"}'
```

### 删除帖子

**`POST /api/delete-post`**

请求体：

```json
{
  "post_id": 1,
  "username": "alice"
}
```

成功响应（`200`）：

```json
{"message": "帖子已永久销毁"}
```

错误响应：

- `400` — JSON 格式错误
- `403` — 无权操作（只有帖子作者或“超级管理员”可以删除）
- `404` — 帖子未找到
- `500` — 数据库错误

```bash
curl -X POST http://localhost:8080/api/delete-post \
  -H "Content-Type: application/json" \
  -d '{"post_id":1,"username":"alice"}'
```

### 获取所有用户 (仅管理员)

**`GET /api/users?admin=超级管理员`**

无请求体。需要 `admin=超级管理员` 查询参数进行授权。

成功响应（`200`）：

```json
[
  {
    "uid": 1,
    "username": "alice",
    "avatar": "https://example.com/avatar.png",
    "email": "test@qq.com",
    "role": 0,
    "username_updated_at": "2023-10-27T10:00:00Z",
    "nickname": "爱丽丝仙境"
  }
]
```

错误响应：

- `403` — 未授权（非“超级管理员”）
- `500` — 数据库错误

```bash
curl "http://localhost:8080/api/users?admin=超级管理员"
```

### 删除用户 (仅管理员)

**`POST /api/delete-user`**

请求体：

```json
{
  "admin_name": "超级管理员",
  "target_uid": 2
}
```

成功响应（`200`）：

```json
{"message": "该用户已被强制剥夺权限并注销账号"}
```

错误响应：

- `400` — JSON 格式错误
- `403` — 未授权（非“超级管理员”）
- `500` — 数据库错误

```bash
curl -X POST http://localhost:8080/api/delete-user \
  -H "Content-Type: application/json" \
  -d '{"admin_name":"超级管理员","target_uid":2}'
```

## 项目结构

```
my-backend/
├── main.go          # 入口：路由、处理器、数据库初始化
├── go.mod
├── go.sum
├── data.db          # 运行时生成（已 gitignore）
├── README.md        # 英文（默认）
└── README.zh-CN.md  # 中文
```

## 开发说明

- `data.db` 已加入 `.gitignore`，不会提交到 Git。克隆仓库后需本地运行服务以生成数据库。
- 当前未实现 JWT 或 Session 机制。登录成功后，前端应自行保存 `uid`、`username`、`nickname` 和 `avatar` 等字段。
- CORS 设置为 `*`，便于本地开发，生产环境建议收紧。
- 首次运行时，如果不存在，会自动创建超级管理员账户（“超级管理员”，默认密码为 “ASDasd5201314.”，可在 `models.go` 中修改），其角色为 `2`。您可以使用此账户进行管理员 API 测试。

## 许可证

本项目为学习用途，尚未添加 LICENSE 文件。

## AI 撰写声明

本项目约有 70% 的代码由 AI 撰写。
