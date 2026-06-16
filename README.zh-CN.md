# my-backend

> **语言：** [English](README.md) | 简体中文

这是一个基于 Go 的轻量级后端，配合 Vue 3 前端使用。它提供用户注册、登录、账号找回、资料更新、社区帖子、评论、收藏和超级管理员管理接口。

## 功能特性

- 用户注册：密码使用 bcrypt 加密，支持 QQ / Gmail 邮箱验证码。
- 用户登录：登录后返回用户公开信息和自定义 HMAC token。
- 账号找回和密码重置：通过邮箱验证码完成。
- 资料更新：支持昵称、头像、签名、密码和登录账号修改。
- 社区功能：发帖、获取帖子、帖子详情、评论、收藏、删除帖子。
- 管理员功能：超级管理员可查看用户列表、删除普通用户、修改管理员资料。
- SQLite 持久化：自动迁移并生成 `data.db`。
- 本地开发友好：支持 `.env` 配置，CORS 可配置。

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

首次启动会在项目目录生成 `data.db`。该文件是本地数据库，不应提交到仓库。

## 环境变量

复制 `.env.example` 为 `.env`，并根据需要修改值：

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `APP_TOKEN_SECRET` | `dev-only-change-me` | HMAC token 签名密钥；上线必须更改 |
| `CORS_ALLOWED_ORIGIN` | `*` | 允许访问后端的前端来源；上线请填真实域名 |
| `SUPER_ADMIN_USERNAME` | `superadmin` | 超级管理员登录账号 |
| `SUPER_ADMIN_EMAIL` | `2672172829@qq.com` | 初始化超级管理员的邮箱 |
| `SUPER_ADMIN_PASSWORD` | `ASDasd5201314.` | 初始化超级管理员密码；上线必须修改 |
| `SMTP_USER` | `2672172829@qq.com` | 发送验证码的邮箱账号 |
| `SMTP_PASS` | 空 | SMTP 授权码或密码 |
| `SMTP_HOST` | `smtp.qq.com` | SMTP 服务器 |
| `SMTP_PORT` | `587` | SMTP 端口 |

示例：

```powershell
$env:APP_TOKEN_SECRET="please-change-to-a-long-random-secret"
$env:CORS_ALLOWED_ORIGIN="http://localhost:5173"
$env:SUPER_ADMIN_USERNAME="superadmin"
$env:SUPER_ADMIN_EMAIL="admin@example.com"
$env:SUPER_ADMIN_PASSWORD="your-strong-password"
go run .
```

## 鉴权说明

登录成功后，后端返回一个自定义 HMAC token。它不是标准 JWT。

Token 结构：

```text
base64(payload).signature
```

`payload` 只包含用户 UID 和过期时间，`signature` 使用 `APP_TOKEN_SECRET` 计算。

受保护接口需要请求头：

```text
Authorization: Bearer <token>
```

## 主要 API

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/send-code` | 发送注册或找回账号验证码 |
| `POST` | `/api/recover-account` | 通过邮箱验证码找回账号 |
| `POST` | `/api/reset-password` | 通过邮箱验证码重置密码 |
| `POST` | `/api/register` | 注册用户 |
| `POST` | `/api/login` | 登录并返回 token |
| `GET` | `/api/posts` | 获取帖子列表 |
| `GET` | `/api/post-detail?id=<id>` | 获取单条帖子详情 |

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
| `GET` | `/api/me` | 获取当前登录用户信息 |
| `POST` | `/api/update` | 修改当前用户资料 |
| `POST` | `/api/create-post` | 发布帖子 |
| `POST` | `/api/update-post` | 编辑帖子 |
| `POST` | `/api/delete-post` | 删除帖子 |
| `POST` | `/api/create-comment` | 发表评论 |
| `POST` | `/api/delete-comment` | 删除评论 |
| `POST` | `/api/toggle-favorite` | 切换收藏状态 |
| `GET` | `/api/my-favorites` | 获取当前用户收藏的帖子 |

### 需要超级管理员的接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/users` | 获取全部用户 |
| `POST` | `/api/delete-user` | 删除普通用户 |
| `POST` | `/api/update-admin-profile` | 更新超级管理员资料 |

## 数据模型简述

- `User`：uid、username、email、avatar、role、nickname、signature、username_updated_at、password_hash。
- `Post`：id、username、nickname、avatar、title、content、image、images、created_at、comments、favorite_count、is_favorited。
- `Comment`：id、post_id、username、nickname、avatar、content、created_at。
- `Favorite`：uid 和 post_id 的联合主键，用于记录收藏关系。
- `VerifyCode`：内存验证码，5 分钟过期。

## 测试

```powershell
$env:GOCACHE='G:\newproject\.tmp\gocache'
go test ./...
```

测试使用内存 SQLite，不会读取或修改本地 `data.db`。

## 开发提醒

- `role = 0` 表示普通用户，`role = 2` 表示超级管理员。
- 未配置 SMTP 时，验证码会打印到后端控制台，便于本地调试。
- CORS 已放到最外层中间件，浏览器预检请求也会统一返回 CORS 响应头。
- 上线前必须配置 `APP_TOKEN_SECRET`、`SUPER_ADMIN_USERNAME`、`SUPER_ADMIN_EMAIL`、`SUPER_ADMIN_PASSWORD` 和 `CORS_ALLOWED_ORIGIN`。

## 修改超级管理员账号和密码

启动前设置环境变量即可：

```powershell
$env:SUPER_ADMIN_USERNAME="你想要的管理员账号"
$env:SUPER_ADMIN_EMAIL="admin@example.com"
$env:SUPER_ADMIN_PASSWORD="你想要的新密码"
go run .
```

后端会根据 `SUPER_ADMIN_EMAIL` 查找超级管理员，并确保其 `role = 2`。

## 许可证

本项目为学习用途，尚未添加 LICENSE 文件。

## AI 撰写声明

本项目约有 70% 的代码由 AI 协助撰写。
