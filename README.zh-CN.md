# my-backend

> **语言：** [English](README.md) | 简体中文

这是一个基于 Go 的轻量级 HTTP 后端，配合 Vue 3 前端使用。它提供注册、登录、资料修改、社区帖子、评论、收藏和管理员管理接口。

## 功能特性

- 用户注册：密码使用 bcrypt 加密，支持 QQ / Gmail 邮箱验证码。
- 用户登录：登录成功后返回用户信息和 `token`。
- Token 鉴权：前端后续请求需要携带 `Authorization: Bearer <token>`。
- 资料更新：支持修改昵称、头像、密码、登录账号；登录账号 180 天内只能修改一次。
- 社区功能：发帖、获取帖子、评论、收藏、删除自己的帖子或评论。
- 管理员功能：超级管理员可以查看用户列表、删除普通用户、删除任意帖子或评论。
- SQLite 持久化：首次运行会自动创建 `data.db`。
- 本地开发友好：CORS 默认允许所有来源，可通过环境变量收紧。

## 技术栈

| 依赖 | 用途 |
|------|------|
| Go 1.26.4+ | 后端运行时 |
| `net/http` | HTTP 服务和路由 |
| GORM | ORM 和自动迁移 |
| `glebarez/sqlite` | 纯 Go SQLite 驱动 |
| `golang.org/x/crypto/bcrypt` | 密码哈希 |

## 快速开始

```bash
cd my-backend
go mod download
go run .
```

服务默认运行在：

```text
http://localhost:8080
```

首次启动会在后端目录生成 `data.db`。这个文件是本地数据库，不应该提交到仓库。

## 环境变量

本项目默认能直接本地运行；如果准备部署或给别人访问，请务必配置下面这些环境变量。

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `APP_TOKEN_SECRET` | `dev-only-change-me` | token 签名密钥；上线必须改成随机长字符串 |
| `CORS_ALLOWED_ORIGIN` | `*` | 允许访问后端的前端来源；上线建议填真实前端域名 |
| `SUPER_ADMIN_EMAIL` | `2672172829@qq.com` | 首次启动自动创建的超级管理员邮箱 |
| `SUPER_ADMIN_PASSWORD` | `ASDasd5201314.` | 首次启动自动创建的超级管理员密码；上线必须修改 |
| `SMTP_USER` | 空 | 发验证码用的邮箱账号；为空时验证码会打印在后端控制台 |
| `SMTP_PASS` | 空 | 邮箱授权码或密码 |
| `SMTP_HOST` | `smtp.qq.com` | SMTP 服务器 |
| `SMTP_PORT` | `587` | SMTP 端口 |

示例：

```powershell
$env:APP_TOKEN_SECRET="please-change-to-a-long-random-secret"
$env:CORS_ALLOWED_ORIGIN="http://localhost:5173"
$env:SUPER_ADMIN_EMAIL="admin@example.com"
$env:SUPER_ADMIN_PASSWORD="your-strong-password"
go run .
```

## 鉴权说明

登录成功后，后端会返回一个自定义 HMAC token。它不是标准 JWT，结构大致是：

```text
base64(payload).signature
```

`payload` 只保存用户 UID 和过期时间，`signature` 用 `APP_TOKEN_SECRET` 计算。后端收到请求后会重新计算签名，如果签名不一致或 token 过期，就拒绝请求。

需要登录的接口必须携带请求头：

```text
Authorization: Bearer <token>
```

## 主要 API

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/send-code` | 发送注册验证码 |
| `POST` | `/api/register` | 注册用户 |
| `POST` | `/api/login` | 登录并返回 token |
| `GET` | `/api/posts` | 获取帖子列表；登录后会额外返回当前用户收藏状态 |

登录成功响应示例：

```json
{
  "message": "登录成功！欢迎回来，alice",
  "uid": 1,
  "username": "alice",
  "nickname": "user_1",
  "avatar": "https://api.dicebear.com/7.x/adventurer/svg?seed=user_1",
  "role": 0,
  "token": "base64payload.signature"
}
```

### 需要登录的接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/update` | 修改当前登录用户资料 |
| `POST` | `/api/create-post` | 发布帖子 |
| `POST` | `/api/delete-post` | 删除自己的帖子；管理员可删除任意帖子 |
| `POST` | `/api/create-comment` | 发表评论 |
| `POST` | `/api/delete-comment` | 删除自己的评论；管理员可删除任意评论 |
| `POST` | `/api/toggle-favorite` | 收藏或取消收藏帖子 |

发帖请求示例：

```bash
curl -X POST http://localhost:8080/api/create-post \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"content":"这是一条新帖子"}'
```

### 需要超级管理员的接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/users` | 获取全部用户 |
| `POST` | `/api/delete-user` | 删除普通用户 |

管理员接口同样使用 `Authorization: Bearer <token>`，后端会根据用户 `role` 判断是否为超级管理员。`role = 2` 表示超级管理员，`role = 0` 表示普通用户。

## 数据模型简述

- `User`：用户账号、邮箱、头像、昵称、角色、密码哈希。
- `Post`：帖子内容、作者账号、作者昵称和头像快照。
- `Comment`：评论内容、所属帖子、评论者信息。
- `Favorite`：用户和帖子的收藏关系，使用联合主键避免重复收藏。
- `VerifyCode`：保存在内存中的临时验证码，默认 5 分钟过期。

## 测试

```powershell
$env:GOCACHE='G:\newproject\.tmp\gocache'
go test ./...
```

测试使用内存 SQLite，不会读取或修改项目里的 `data.db`。

## 开发提醒

- 本项目适合学习前后端分离、登录鉴权和基础 CRUD。
- 默认管理员账号和默认 token 密钥只适合本地开发。
- 上线前必须配置 `APP_TOKEN_SECRET`、`SUPER_ADMIN_EMAIL`、`SUPER_ADMIN_PASSWORD` 和 `CORS_ALLOWED_ORIGIN`。
- 邮件服务未配置时，验证码会输出在后端控制台，方便本地调试。

## 许可证

本项目为学习用途，尚未添加 LICENSE 文件。

## AI 撰写声明

本项目约有 70% 的代码由 AI 协助撰写。
