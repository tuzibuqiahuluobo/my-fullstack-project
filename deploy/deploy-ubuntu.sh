#!/usr/bin/env bash
set -euo pipefail

DOMAIN="${DOMAIN:-}"
PROJECT_ROOT="${PROJECT_ROOT:-/opt/sunshine}"
BACKEND_DIR="${BACKEND_DIR:-$PROJECT_ROOT/my-backend}"
FRONTEND_DIR="${FRONTEND_DIR:-$PROJECT_ROOT/my-frontend}"
WEB_ROOT="${WEB_ROOT:-/var/www/sunshine}"

if [ -z "$DOMAIN" ]; then
  echo "请用 DOMAIN=你的域名 运行，例如：sudo DOMAIN=example.com bash deploy/deploy-ubuntu.sh"
  exit 1
fi

if [ ! -f "$BACKEND_DIR/.env" ]; then
  echo "缺少 $BACKEND_DIR/.env，请先复制 deploy/backend.env.production.example 并填写真实配置。"
  exit 1
fi

if [ ! -f "$FRONTEND_DIR/.env.production" ]; then
  echo "缺少 $FRONTEND_DIR/.env.production，请先复制 .env.production.example 并填写 VITE_API_BASE_URL。"
  exit 1
fi

# 这里不自动安装 Go/Node，因为不同服务器镜像版本差异较大；脚本只负责项目构建和服务安装。
command -v go >/dev/null 2>&1 || { echo "没有找到 go，请先安装 Go。"; exit 1; }
command -v npm >/dev/null 2>&1 || { echo "没有找到 npm，请先安装 Node.js 和 npm。"; exit 1; }
command -v nginx >/dev/null 2>&1 || { echo "没有找到 nginx，请先安装 nginx。"; exit 1; }
command -v rsync >/dev/null 2>&1 || { echo "没有找到 rsync，请先执行：sudo apt install -y rsync"; exit 1; }

echo "==> 构建后端"
cd "$BACKEND_DIR"
go mod download
go test ./...
go build -o sunshine-backend .

echo "==> 构建前端"
cd "$FRONTEND_DIR"
npm ci
npm run build

echo "==> 安装前端静态文件"
mkdir -p "$WEB_ROOT"
rsync -a --delete "$FRONTEND_DIR/dist/" "$WEB_ROOT/"

echo "==> 安装 systemd 服务"
cp "$BACKEND_DIR/deploy/sunshine-backend.service" /etc/systemd/system/sunshine-backend.service
cp "$BACKEND_DIR/deploy/sunshine-backup.service" /etc/systemd/system/sunshine-backup.service
cp "$BACKEND_DIR/deploy/sunshine-backup.timer" /etc/systemd/system/sunshine-backup.timer
chmod +x "$BACKEND_DIR/deploy/backup-sqlite.sh"

echo "==> 安装 Nginx 配置"
sed "s/__DOMAIN__/$DOMAIN/g" "$BACKEND_DIR/deploy/nginx.sunshine.conf" > /etc/nginx/sites-available/sunshine
ln -sfn /etc/nginx/sites-available/sunshine /etc/nginx/sites-enabled/sunshine
rm -f /etc/nginx/sites-enabled/default

echo "==> 重载服务"
systemctl daemon-reload
systemctl enable --now sunshine-backend
systemctl enable --now sunshine-backup.timer
nginx -t
systemctl reload nginx

echo "部署完成。请先访问 http://$DOMAIN 验证，再执行：certbot --nginx -d $DOMAIN"
