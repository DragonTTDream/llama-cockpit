#!/bin/bash
# migrate-nonroot.sh — 把 llama-panel 从「以 root 运行、目录归 user 所有」
# 改成「以专用非特权用户运行 + 极小提权面」。
#
# 为什么要改：
#   /opt/llama-panel 原归 user:user，而服务以 root 运行
#   ⇒ 任何能登录 user 的人都能替换那个二进制，下次重启即 root 代码执行（本地提权）
#
# 改完之后：
#   - /opt/llama-panel      归 root:root，面板用户只读 → 无法替换二进制
#   - 服务以 llama-panel 用户运行，无 shell
#   - 状态文件移到 /var/lib/llama-panel（归面板用户）
#   - 唯一提权通道：sudo 调用 /opt/llama-panel/llama-panel helper <verb>，逐条校验参数
#
# 用法：sudo bash migrate-nonroot.sh
# 回滚：sudo bash migrate-nonroot.sh --rollback

set -euo pipefail

PANEL_USER=llama-panel
BIN_DIR=/opt/llama-panel
STATE_DIR=/var/lib/llama-panel
BIN=$BIN_DIR/llama-panel
UNIT=/etc/systemd/system/llama-panel.service
SUDOERS=/etc/sudoers.d/llama-panel
OLD_SSH_KEY=/root/.ssh/id_ed25519_panel

if [ "$(id -u)" != "0" ]; then
  echo "必须以 root 运行：sudo bash $0" >&2
  exit 1
fi

say() { printf '\n\033[1m== %s ==\033[0m\n' "$*"; }
ok()  { printf '   \033[32m✓\033[0m %s\n' "$*"; }
warn(){ printf '   \033[33m!\033[0m %s\n' "$*"; }

# ---------------------------------------------------------------- 回滚
if [ "${1:-}" = "--rollback" ]; then
  say "回滚到 root 运行"
  rm -f "$SUDOERS"
  if [ -f "$UNIT" ]; then
    sed -i '/^User=/d;/^Group=/d;/^StateDirectory=/d' "$UNIT"
  fi
  systemctl daemon-reload
  systemctl restart llama-panel.service
  sleep 2
  ok "已回滚（服务重新以 root 运行）"
  systemctl status llama-panel.service --no-pager -l | head -8
  exit 0
fi

# ---------------------------------------------------------------- 前置检查
say "前置检查"
[ -x "$BIN" ] || { echo "找不到 $BIN" >&2; exit 1; }
ok "二进制存在：$BIN"
$BIN helper 2>&1 | head -1 || true
if ! $BIN helper daemon-reload >/dev/null 2>&1 && [ "$(id -u)" = "0" ]; then
  warn "helper 自检异常（继续，稍后会实测）"
fi
ok "SELinux: $(getenforce 2>/dev/null || echo 未启用)"
if [ "$(getenforce 2>/dev/null)" = "Enforcing" ]; then
  echo "   注意：Enforcing 环境下，SSH 私钥带 ssh_home_t 标签，"
  echo "   本脚本已通过「不使用 StateDirectory」规避 systemd 的 relabel 动作。"
fi

# ---------------------------------------------------------------- 1. 建用户
say "1) 创建系统用户 $PANEL_USER"
if id -u "$PANEL_USER" >/dev/null 2>&1; then
  ok "已存在，跳过"
else
  useradd --system --no-create-home --shell /usr/sbin/nologin "$PANEL_USER"
  ok "已创建（无 home、无 shell）"
fi

# ---------------------------------------------------------------- 2. 状态目录
say "2) 状态目录 $STATE_DIR（二进制只读，状态可写）"
mkdir -p "$STATE_DIR"
for f in token units.json settings.json master.key secrets.enc; do
  if [ -f "$BIN_DIR/$f" ]; then
    cp -p "$BIN_DIR/$f" "$STATE_DIR/$f"
    ok "迁移 $f"
  fi
done
# 面板自己生成的私钥目录
mkdir -p "$STATE_DIR/.ssh"
if [ -f "$BIN_DIR/.ssh/id_ed25519" ]; then
  cp -p "$BIN_DIR/.ssh/id_ed25519" "$STATE_DIR/.ssh/id_ed25519"
  ok "迁移已保存的私钥"
fi
chown -R "$PANEL_USER:$PANEL_USER" "$STATE_DIR"
chmod 700 "$STATE_DIR"
chmod 600 "$STATE_DIR"/token "$STATE_DIR"/master.key "$STATE_DIR"/secrets.enc 2>/dev/null || true
ok "归属 $PANEL_USER，权限 700"

# ---------------------------------------------------------------- 3. SSH 私钥
say "3) 跨机 SSH 私钥"
if [ -f "$OLD_SSH_KEY" ]; then
  cp -p "$OLD_SSH_KEY" "$STATE_DIR/.ssh/id_ed25519_panel"
  chown "$PANEL_USER:$PANEL_USER" "$STATE_DIR/.ssh/id_ed25519_panel"
  chmod 600 "$STATE_DIR/.ssh/id_ed25519_panel"
  ok "已复制到 $STATE_DIR/.ssh/id_ed25519_panel（0600，归 $PANEL_USER）"
  # 更新设置里的密钥路径
  python3 - "$STATE_DIR/settings.json" "$STATE_DIR/.ssh/id_ed25519_panel" <<'PY'
import json, os, sys
p, key = sys.argv[1], sys.argv[2]
d = {}
if os.path.exists(p):
    try:
        d = json.load(open(p))
    except Exception:
        d = {}
d["ssh_key_path"] = key
json.dump(d, open(p, "w"), ensure_ascii=False, indent=2)
print("      已更新 settings.json 的 ssh_key_path")
PY
  chown "$PANEL_USER:$PANEL_USER" "$STATE_DIR/settings.json"
else
  warn "没找到 $OLD_SSH_KEY（若面板用的是别处的密钥，请在「设置」页改「密钥路径」）"
fi

# ---------------------------------------------------------------- 4. 收紧二进制
say "4) 收紧 $BIN_DIR（关键：防二进制被替换）"
chown -R root:root "$BIN_DIR"
chmod 755 "$BIN_DIR"
chmod 755 "$BIN"
[ -d "$BIN_DIR/.build" ] && chmod 700 "$BIN_DIR/.build"
rm -rf "$BIN_DIR/.ssh"   # 已迁移，清掉旧位置
ok "$BIN_DIR → root:root，二进制 755（面板用户只读）"

# ---------------------------------------------------------------- 5. sudoers
say "5) 安装提权白名单 $SUDOERS"
cat > "$SUDOERS" <<EOF
# llama-panel：唯一提权通道。
# 面板以 $PANEL_USER 运行；只有 helper 模式可提权，且 helper 内部逐条校验参数
# （unit 名白名单、env 只允许 KEY=value、unit 文件必须是 llama-server 模板）。
$PANEL_USER ALL=(root) NOPASSWD: $BIN helper
EOF
chmod 440 "$SUDOERS"
visudo -c -f "$SUDOERS" >/dev/null && ok "sudoers 语法通过"
ok "规则：$PANEL_USER ALL=(root) NOPASSWD: $BIN helper"

# ---------------------------------------------------------------- 6. 改 unit
say "6) 修改 $UNIT（去掉 root，指定用户）"
cp -a "$UNIT" "$UNIT.bak-$(date +%s)"
if grep -q '^User=' "$UNIT"; then
  sed -i "s|^User=.*|User=$PANEL_USER|" "$UNIT"
else
  sed -i "s|^\[Service\]|[Service]\nUser=$PANEL_USER|" "$UNIT"
fi
if grep -q '^Group=' "$UNIT"; then
  sed -i "s|^Group=.*|Group=$PANEL_USER|" "$UNIT"
else
  sed -i "s|^User=$PANEL_USER|User=$PANEL_USER\nGroup=$PANEL_USER|" "$UNIT"
fi
# 关键：绝不要用 StateDirectory=
# systemd 会以 init_t 去 relabel 整个 /var/lib/llama-panel，
# 而里面那把 SSH 私钥带 ssh_home_t 标签 ⇒ AVC denied ⇒ 服务起不来。
# 面板自己会在运行期解析状态目录（见 Go 的 stateDir()），不需要 systemd 托管。
sed -i '/^StateDirectory=/d' "$UNIT"
ok "已设 User=$PANEL_USER，且已移除 StateDirectory（避免 SELinux 冲突）"
ok "备份：$UNIT.bak-*"

# ---------------------------------------------------------------- 7. 生效
say "7) 重载并重启"
systemctl stop llama-panel.service 2>/dev/null || true
systemctl daemon-reload
systemctl restart llama-panel.service
sleep 3

PORT=$(python3 - "$STATE_DIR/settings.json" <<'PY'
import json, os, sys
p = sys.argv[1]
try:
    print(json.load(open(p)).get("listen_port", 8077))
except Exception:
    print(8077)
PY
)

say "8) 验证"
echo "   运行身份：$(systemctl show llama-panel.service -p User --value)"
echo "   服务状态：$(systemctl is-active llama-panel.service)"
code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/" || echo 000)
echo "   根路径  ：HTTP $code"
echo "   模型数  ：$(curl -s "http://127.0.0.1:$PORT/api/state" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["units"]))' 2>/dev/null || echo '?')"

# ---- 自检不通过就自动回滚，绝不留服务在重启循环里 ----
if [ "$(systemctl is-active llama-panel.service)" != "active" ] || [ "$code" != "200" ]; then
  say "!! 自检未通过，自动回滚"
  journalctl -u llama-panel -n 15 --no-pager | tail -15
  rm -f "$SUDOERS"
  sed -i '/^User=/d;/^Group=/d;/^StateDirectory=/d' "$UNIT"
  systemctl daemon-reload
  systemctl restart llama-panel.service
  sleep 2
  echo "   回滚后状态：$(systemctl is-active llama-panel.service)"
  echo "   回滚后 HTTP：$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/")"
  echo "   请把上面的日志发给我。"
  exit 1
fi
ok "服务已以 $PANEL_USER 身份运行且响应正常"

say "9) 提权通道实测（以 $PANEL_USER 身份调 helper）"
if sudo -u "$PANEL_USER" sudo -n "$BIN" helper daemon-reload 2>/dev/null; then
  ok "helper 提权可用"
else
  warn "helper 提权不通！请检查 $SUDOERS，或回滚：sudo bash $0 --rollback"
fi

cat <<EOF

============================================================
完成。要点：
  - 二进制 $BIN 现在归 root:root，面板用户无法替换
  - 服务以 $PANEL_USER 运行，只在 5 类写操作上经 helper 提权
  - 状态在 $STATE_DIR（归 $PANEL_USER）
  - 回滚：sudo bash $0 --rollback

注意：SELinux 为 $(getenforce 2>/dev/null)。
若服务起不来，先看日志：
  journalctl -u llama-panel -n 40 --no-pager
常见原因：
  1) SELinux（本机为 Enforcing）拦截。查：
       journalctl -u llama-panel -n 30 --no-pager | grep -i avc
     若是 AVC denied，记下 scontext/tcontext/tclass 再定点放行，
     或临时 setenforce 0 确认是不是它。
  2) 状态目录归属被改乱：
       chown -R $PANEL_USER:$PANEL_USER $STATE_DIR
     注意：不要往 unit 里加 StateDirectory= —— 那会让 systemd(init_t)
     去 relabel 整棵树，而树里的 SSH 私钥带 ssh_home_t 标签，
     SELinux 必然拒绝（这正是上一版失败的原因）。
============================================================
EOF
