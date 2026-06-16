# my-backend

> **Languages:** English | [简体中文](README.zh-CN.md)

A lightweight Go backend for a Vue 3 frontend. It provides user registration, login, account recovery, profile updates, community posts, comments, favorites, and super-admin management.

## Features

- User registration with bcrypt password hashing and QQ / Gmail email verification codes.
- Login returns public user fields plus a custom HMAC token.
- Account recovery and password reset through email verification codes.
- Profile updates for nickname, avatar, signature, password, and login username.
- Community APIs for posts, comments, favorites, and post details.
- Admin APIs for listing users, deleting users, and updating the super-admin profile.
- SQLite persistence with automatic schema migration.
- CORS enabled for local frontend development, configurable via environment variables.
- .env support for local development.

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

On first run, the backend creates `data.db` in the project root.

## Environment Variables

Copy `.env.example` to `.env` and fill in any values you want to override.

| Name | Default | Purpose |
|------|---------|---------|
| `APP_TOKEN_SECRET` | `dev-only-change-me` | HMAC token signing secret; change this in production |
| `CORS_ALLOWED_ORIGIN` | `*` | Allowed frontend origin; set to your real frontend URL in production |
| `SUPER_ADMIN_USERNAME` | `superadmin` | Super admin login username |
| `SUPER_ADMIN_EMAIL` | `2672172829@qq.com` | Super admin email used to initialize the account |
| `SUPER_ADMIN_PASSWORD` | `ASDasd5201314.` | Super admin password used on first run; change it before deploying |
| `SMTP_USER` | `2672172829@qq.com` | Email account used to send verification codes |
| `SMTP_PASS` | empty | SMTP password or authorization code |
| `SMTP_HOST` | `smtp.qq.com` | SMTP server hostname |
| `SMTP_PORT` | `587` | SMTP server port |

Example:

```powershell
$env:APP_TOKEN_SECRET="please-change-to-a-long-random-secret"
$env:CORS_ALLOWED_ORIGIN="http://localhost:5173"
$env:SUPER_ADMIN_USERNAME="superadmin"
$env:SUPER_ADMIN_EMAIL="admin@example.com"
$env:SUPER_ADMIN_PASSWORD="your-strong-password"
go run .
```

## Authentication

This project uses a custom HMAC token for learning purposes. It is not a standard JWT.

Token format:

```text
base64(payload).signature
```

The payload contains only the user UID and expiration time, and the signature is verified with `APP_TOKEN_SECRET`.

Protected endpoints require:

```text
Authorization: Bearer <token>
```

## API Overview

### Public Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/send-code` | Send registration or recovery email verification code |
| `POST` | `/api/recover-account` | Recover username by email verification code |
| `POST` | `/api/reset-password` | Reset password by email verification code |
| `POST` | `/api/register` | Register a new user |
| `POST` | `/api/login` | Login and return token |
| `GET` | `/api/posts` | List all posts |
| `GET` | `/api/post-detail?id=<id>` | Get details for a single post |

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

### Authenticated Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/me` | Get current logged-in user info |
| `POST` | `/api/update` | Update current user profile |
| `POST` | `/api/create-post` | Create a post |
| `POST` | `/api/update-post` | Edit a post |
| `POST` | `/api/delete-post` | Delete a post |
| `POST` | `/api/create-comment` | Create a comment |
| `POST` | `/api/delete-comment` | Delete a comment |
| `POST` | `/api/toggle-favorite` | Toggle post favorite state |
| `GET` | `/api/my-favorites` | List current user's favorite posts |

### Super Admin Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/users` | List all users |
| `POST` | `/api/delete-user` | Delete a normal user |
| `POST` | `/api/update-admin-profile` | Update super-admin profile |

## Data Model Summary

- `User`: uid, username, email, avatar, role, nickname, signature, username update timestamp, password hash.
- `Post`: id, username, nickname, avatar, title, content, image, images, created_at, comment list, favorite count, favorite state.
- `Comment`: id, post_id, username, nickname, avatar, content, created_at.
- `Favorite`: uid + post_id unique pair to track favorites.
- `VerifyCode`: in-memory verification code storage with 5-minute expiry.

## Testing

```powershell
$env:GOCACHE='G:\newproject\.tmp\gocache'
go test ./...
```

Tests use in-memory SQLite and do not modify the local `data.db`.

## Development Notes

- `role = 0` means normal user, `role = 2` means super admin.
- If SMTP is not configured, verification codes are printed to the backend console for local development.
- CORS is applied globally via middleware, so browser preflight requests are handled consistently.
- Update `APP_TOKEN_SECRET`, `SUPER_ADMIN_USERNAME`, `SUPER_ADMIN_EMAIL`, `SUPER_ADMIN_PASSWORD`, and `CORS_ALLOWED_ORIGIN` before production deployment.

## Super Admin Setup

Set environment variables before startup to initialize or update the super admin account:

```powershell
$env:SUPER_ADMIN_USERNAME="your-admin-username"
$env:SUPER_ADMIN_EMAIL="admin@example.com"
$env:SUPER_ADMIN_PASSWORD="your-new-password"
go run .
```

The backend finds the super admin by `SUPER_ADMIN_EMAIL` and ensures `role = 2`.

## License

This is a learning project. No license file has been added yet.

## AI Attribution

Approximately 70% of this project was written with AI assistance.
