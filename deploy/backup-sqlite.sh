#!/usr/bin/env bash
set -euo pipefail

DB_PATH="${DB_PATH:-/opt/sunshine/my-backend/data.db}"
BACKUP_DIR="${BACKUP_DIR:-/opt/backups/sunshine}"

# SQLite 是一个普通文件，备份前先复制一份带日期的快照，避免误操作时无法恢复。
mkdir -p "$BACKUP_DIR"
if [ -f "$DB_PATH" ]; then
  cp "$DB_PATH" "$BACKUP_DIR/data-$(date +%Y%m%d-%H%M%S).db"
fi

# 只保留最近 14 天的备份，防止 40G 系统盘被旧备份慢慢占满。
find "$BACKUP_DIR" -name "data-*.db" -type f -mtime +14 -delete

