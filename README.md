# my-backend

> **Languages:** English | [简体中文](README.zh-CN.md)

A lightweight Go HTTP backend providing user registration, login, and profile update APIs.

## Features

- User registration with bcrypt password hashing
- User login (returns `uid` and `avatar`)
- Profile updates (username, avatar, password — each field optional)
- SQLite auto-migration on startup
- CORS enabled for frontend development

## Tech Stack

| Dependency | Purpose |
|------------|---------|
| [Go](https://go.dev/) 1.26.4+ | Runtime |
| `net/http` | HTTP server and routing |
| [GORM](https://gorm.io/) | ORM and database migrations |
| [glebarez/sqlite](https://github.com/glebarez/sqlite) | Pure-Go SQLite driver (no CGO) |
| [golang.org/x/crypto/bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt) | Password hashing |

## Quick Start

### Prerequisites

- Go 1.26.4 or later

### Install and Run

```bash
git clone https://github.com/tuzibuqiahuluobo/my-fullstack-project
cd my-fullstack-project
go mod download
go run main.go
```

The server starts at `http://localhost:8080`. You should see:

```
🚀 服务器已启动！运行在 http://localhost:8080
```

On first run, a `data.db` SQLite file is created automatically in the project root.

## Data Model

The `User` struct in `main.go`:

| Field | Type | Description |
|-------|------|-------------|
| `uid` | uint | Primary key, auto-increment |
| `username` | string | Unique username |
| `password_hash` | string | bcrypt hash (not exposed via API) |
| `avatar` | string | Avatar URL |
| `gender` | string | Gender (reserved, no API yet) |
| `age` | int | Age (reserved, no API yet) |

## API Reference

All endpoints use `POST` with `Content-Type: application/json`. CORS is enabled (`Access-Control-Allow-Origin: *`).

### Register

**`POST /api/register`**

Request body:

```json
{
  "username": "alice",
  "password": "secret123"
}
```

Success response (`200`):

```json
{"message": "注册成功！欢迎加入。"}
```

Error responses (`400`):

- Invalid JSON format
- Username already taken or system error

```bash
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret123"}'
```

### Login

**`POST /api/login`**

Request body:

```json
{
  "username": "alice",
  "password": "secret123"
}
```

Success response (`200`):

```json
{
  "message": "登录成功！欢迎回来，alice",
  "uid": 1,
  "avatar": ""
}
```

Error responses (`401`):

- Username does not exist
- Incorrect password

```bash
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret123"}'
```

### Update Profile

**`POST /api/update`**

Request body:

```json
{
  "uid": 1,
  "username": "alice_new",
  "avatar": "https://example.com/avatar.png",
  "password": "newpassword"
}
```

`username`, `avatar`, and `password` are optional — leave empty or omit to skip updating that field.

Success response (`200`):

```json
{"message": "资料更新成功！"}
```

Error responses:

- `404` — User not found
- `500` — Update failed (e.g. username already taken)

```bash
curl -X POST http://localhost:8080/api/update \
  -H "Content-Type: application/json" \
  -d '{"uid":1,"avatar":"https://example.com/avatar.png"}'
```

## Project Structure

```
my-backend/
├── main.go          # Entry point: routes, handlers, DB init
├── go.mod
├── go.sum
├── data.db          # Generated at runtime (gitignored)
├── README.md        # English (default)
└── README.zh-CN.md  # Chinese
```

## Development Notes

- `data.db` is listed in `.gitignore` and will not be committed. Clone the repo and run the server to generate it locally.
- There is no JWT or session mechanism. After login, the frontend should store `uid` (and other fields) as needed.
- CORS is set to `*` for easy local development. Restrict this in production.
- `gender` and `age` fields exist in the model but have no API endpoints yet.

## License

This is a learning project. No license file has been added yet.
