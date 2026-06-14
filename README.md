# my-backend

> **Languages:** English | [简体中文](README.zh-CN.md)

A lightweight Go HTTP backend for the Vue 3 frontend. It supports registration, login, account/password recovery, profile updates, community posts, post details, comments, favorites, and super-admin management.

## Features

- Registration with bcrypt password hashing and QQ / Gmail email verification.
- Login returns public user fields plus a `token`.
- Account recovery and password reset through email verification codes.
- Token authentication via `Authorization: Bearer <token>`.
- Profile updates for nickname, avatar, password, and login username.
- Community APIs for posts, post details, comments, favorites, and deletion permissions.
- Admin APIs for listing users, deleting normal users, editing admin profile, and moderating posts/comments.
- SQLite persistence with automatic migration.
- Development-friendly CORS, configurable for production.

## Quick Start

```bash
cd my-backend
go mod download
go run .
```

The server starts at:

```text
http://localhost:8080
```

On first run, the backend creates a local `data.db` SQLite file.

To send real verification emails, copy `.env.example` to `.env`, then fill `SMTP_PASS` with your QQ mail authorization code.

## Environment Variables

The defaults are convenient for local learning. For deployment, configure these values explicitly.

| Name | Default | Purpose |
|------|---------|---------|
| `APP_TOKEN_SECRET` | `dev-only-change-me` | HMAC token signing secret; change this in production |
| `CORS_ALLOWED_ORIGIN` | `*` | Allowed frontend origin; use your real frontend URL in production |
| `SUPER_ADMIN_USERNAME` | `superadmin` | Login username for the initial super admin |
| `SUPER_ADMIN_EMAIL` | `2672172829@qq.com` | Email for the initial super admin |
| `SUPER_ADMIN_PASSWORD` | `ASDasd5201314.` | Password for the initial super admin; change this in production |
| `SMTP_USER` | `2672172829@qq.com` | Email account used to send verification codes |
| `SMTP_PASS` | empty | SMTP password or authorization code |
| `SMTP_HOST` | `smtp.qq.com` | SMTP host |
| `SMTP_PORT` | `587` | SMTP port |

PowerShell example:

```powershell
$env:APP_TOKEN_SECRET="please-change-to-a-long-random-secret"
$env:CORS_ALLOWED_ORIGIN="http://localhost:5173"
$env:SUPER_ADMIN_USERNAME="superadmin"
$env:SUPER_ADMIN_EMAIL="admin@example.com"
$env:SUPER_ADMIN_PASSWORD="your-strong-password"
go run .
```

## Authentication

This project keeps a simple custom HMAC token for learning purposes. It is not standard JWT. The token format is:

```text
base64(payload).signature
```

The payload contains only the user UID and expiration time. The signature is calculated with `APP_TOKEN_SECRET`. Protected endpoints require:

```text
Authorization: Bearer <token>
```

## API Overview

### Public

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/send-code` | Send registration verification code |
| `POST` | `/api/recover-account` | Recover username by email verification code |
| `POST` | `/api/reset-password` | Reset password by email verification code |
| `POST` | `/api/register` | Register a user |
| `POST` | `/api/login` | Login and return token |
| `GET` | `/api/posts` | List posts; logged-in users also receive favorite state |
| `GET` | `/api/post-detail?id=<id>` | Read one post with comments and favorite state |

### Login Response Example

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

### Requires Login

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/update` | Update current user's profile |
| `POST` | `/api/create-post` | Create a post |
| `POST` | `/api/delete-post` | Delete own post; admins can delete any post |
| `POST` | `/api/create-comment` | Create a comment |
| `POST` | `/api/delete-comment` | Delete own comment; admins can delete any comment |
| `POST` | `/api/toggle-favorite` | Favorite or unfavorite a post |
| `GET` | `/api/my-favorites` | List current user's favorite posts |

### Requires Super Admin

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/users` | List all users |
| `POST` | `/api/delete-user` | Delete a normal user |
| `POST` | `/api/update-admin-profile` | Update super-admin username, password, avatar, or email |

## Tests

```powershell
$env:GOCACHE='G:\newproject\.tmp\gocache'
go test ./...
```

Tests use an in-memory SQLite database and do not touch the local `data.db`.

## Development Notes

- `role = 0` means normal user, `role = 2` means super admin.
- If SMTP is not configured, verification codes are printed to the backend console for local development.
- CORS is applied through a top-level middleware so browser preflight requests for new API routes also receive CORS headers.
- Change `APP_TOKEN_SECRET`, `SUPER_ADMIN_USERNAME`, `SUPER_ADMIN_EMAIL`, `SUPER_ADMIN_PASSWORD`, and `CORS_ALLOWED_ORIGIN` before production deployment.

## Changing the Super Admin Account

For local development, set these variables before starting the backend:

```powershell
$env:SUPER_ADMIN_USERNAME="your-admin-username"
$env:SUPER_ADMIN_EMAIL="admin@example.com"
$env:SUPER_ADMIN_PASSWORD="your-new-password"
go run .
```

The backend finds the super admin by `SUPER_ADMIN_EMAIL`, then syncs username, nickname, password, and `role = 2`. Login with `SUPER_ADMIN_USERNAME`, not the email address.

## License

This is a learning project. No license file has been added yet.

## AI Attribution

Approximately 70% of this project was written with AI assistance.

## Alibaba Cloud Deployment

For Ubuntu 22.04/24.04 deployment on Alibaba Cloud Simple Application Server, see:

```text
deploy/README.zh-CN.md
```

The deploy folder includes Nginx, systemd, SQLite backup, and Ubuntu deployment templates. Do not commit real `.env` secrets or the production `data.db`.
