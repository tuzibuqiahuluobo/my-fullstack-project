# SunShine Ubuntu 部署说明

本目录用于把 SunShine 部署到阿里云轻量应用服务器，方案是 `Nginx + systemd + SQLite`。

## 1. 服务器准备

在阿里云控制台的防火墙里放行：

- `22`：SSH 登录服务器
- `80`：HTTP，申请 HTTPS 证书前需要用到
- `443`：HTTPS，正式访问使用

不要对公网放行 `8080`。后端只监听本机，由 Nginx 代理访问。

## 2. 安装基础软件

```bash
sudo apt update
sudo apt install -y nginx certbot python3-certbot-nginx curl ca-certificates rsync
```

Node.js 需要 `>= 20.19.0` 或 `>= 22.12.0`，Go 建议使用 `go.mod` 中的版本或兼容版本。

## 3. 放置项目

推荐目录：

```text
/opt/sunshine/my-backend
/opt/sunshine/my-frontend
/var/www/sunshine
```

可以用 `git clone` 拉取两个仓库，也可以先在本地打包后上传。上线使用全新数据库时，不要上传本地的 `data.db`。

## 4. 配置后端环境变量

复制模板：

```bash
cd /opt/sunshine/my-backend
cp deploy/backend.env.production.example .env
nano .env
```

必须修改：

- `APP_TOKEN_SECRET`：改成长随机字符串
- `CORS_ALLOWED_ORIGIN`：改成 `https://你的域名`
- `SUPER_ADMIN_USERNAME`、`SUPER_ADMIN_EMAIL`、`SUPER_ADMIN_PASSWORD`
- `SMTP_PASS`：填写 QQ 邮箱授权码

## 5. 构建并安装

先配置前端线上 API 地址：

```bash
cd /opt/sunshine/my-frontend
cp .env.production.example .env.production
nano .env.production
```

然后运行部署脚本：

```bash
cd /opt/sunshine/my-backend
sudo DOMAIN=你的域名 bash deploy/deploy-ubuntu.sh
```

脚本会做这些事：

- 编译 Go 后端为 `sunshine-backend`
- 构建 Vue 前端并复制到 `/var/www/sunshine`
- 安装 `sunshine-backend.service`
- 安装 Nginx 站点配置
- 安装 SQLite 每日备份 timer

## 6. 启动服务

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now sunshine-backend
sudo systemctl enable --now sunshine-backup.timer
sudo nginx -t
sudo systemctl reload nginx
```

检查后端：

```bash
sudo systemctl status sunshine-backend
curl http://127.0.0.1:8080/api/posts
```

## 7. 启用 HTTPS

先确认域名已经解析到服务器公网 IP，再执行：

```bash
sudo certbot --nginx -d 你的域名
```

完成后访问：

```text
https://你的域名
```

## 8. 验证清单

- 能打开 `https://你的域名/login`
- 刷新 `/main/community` 不出现 404
- 普通用户能注册、登录、发帖、收藏
- 超级管理员能登录 `/admin`
- 邮箱验证码能发送到用户填写的 QQ/Gmail 邮箱
- 浏览器 Network 里的接口地址是 `https://你的域名/api/...`

## 9. 数据库备份

备份脚本默认把数据库备份到：

```text
/opt/backups/sunshine
```

查看定时器：

```bash
systemctl list-timers sunshine-backup.timer
```
