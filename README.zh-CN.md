# my-backend

> **语言：** [English](README.md) | 简体中文

基于 Go 的轻量级 HTTP 后端，提供用户注册、登录与资料更新 API。

## 功能特性

- 用户注册，密码使用 bcrypt 加密存储
- 用户登录（返回 `uid` 和 `avatar`）
- 资料更新（用户名、头像、密码 — 各字段可选更新）
- 启动时 SQLite 自动迁移
- 开启 CORS，便于前端联调

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

`main.go` 中的 `User` 结构体：

| 字段 | 类型 | 说明 |
|------|------|------|
| `uid` | uint | 主键，自增 |
| `username` | string | 唯一用户名 |
| `password_hash` | string | bcrypt 哈希（不通过 API 暴露） |
| `avatar` | string | 头像 URL |
| `gender` | string | 性别（预留字段，暂无 API） |
| `age` | int | 年龄（预留字段，暂无 API） |

## API 文档

所有接口均使用 `POST` 请求，`Content-Type: application/json`。已开启 CORS（`Access-Control-Allow-Origin: *`）。

### 注册

**`POST /api/register`**

请求体：

```json
{
  "username": "alice",
  "password": "secret123"
}
```

成功响应（`200`）：

```json
{"message": "注册成功！欢迎加入。"}
```

错误响应（`400`）：

- JSON 格式错误
- 用户名已被注册或系统错误

```bash
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret123"}'
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
  "avatar": ""
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
  "password": "newpassword"
}
```

`username`、`avatar`、`password` 均为可选字段 — 留空或不传则跳过该字段的更新。

成功响应（`200`）：

```json
{"message": "资料更新成功！"}
```

错误响应：

- `404` — 找不到该用户
- `500` — 更新失败（如用户名已被占用）

```bash
curl -X POST http://localhost:8080/api/update \
  -H "Content-Type: application/json" \
  -d '{"uid":1,"avatar":"https://example.com/avatar.png"}'
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
- 当前无 JWT 或 Session 机制，登录成功后由前端自行保存 `uid` 等字段。
- CORS 设置为 `*`，便于本地开发，生产环境建议收紧。
- `gender` 和 `age` 字段已在模型中定义，但尚无对应 API 接口。

## 许可证

本项目为学习用途，尚未添加 LICENSE 文件。
