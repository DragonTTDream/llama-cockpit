#!/bin/bash
# deploy.sh — 构建并把面板装到 GPU 机。
#
# 与旧版的三点不同（旧版会把「非 root 运行」的加固反复拆掉）：
#   1. 二进制用 sudo install 装成 root:root，不再 chown 目录给 user
#      否则任何能登录 user 的人都能替换二进制 —— 那就是本地提权
#   2. systemd unit 只在【不存在时】才写。已有则原样保留，
#      否则会把「运行身份（User=）」每次部署都重置回 root
#   3. 令牌路径跟着状态目录走（/var/lib/llama-panel 优先）
set -uo pipefail
P=/home/user/.openclaw/workspace/projects/llama-panel
KEY=/home/user/.ssh/id_ed25519
SSH=(ssh -o BatchMode=yes -o IdentitiesOnly=yes -i "$KEY" user@192.0.2.10)
SCP=(scp -q -o BatchMode=yes -o IdentitiesOnly=yes -i "$KEY")

echo "== 1) 构建 =="
bash "$P/harness/gate.sh" | grep -E "^BUILD=|^BIN:" | head -5

echo "== 2) 安置二进制（root:root，0755）=="
"${SSH[@]}" 'sudo -n install -o root -g root -m 0755 /tmp/lp-bin /opt/llama-panel/llama-panel && sudo -n chown root:root /opt/llama-panel && ls -la /opt/llama-panel/llama-panel'

echo "== 3) systemd unit（已存在则不动，避免重置运行身份）=="
if "${SSH[@]}" 'test -f /etc/systemd/system/llama-panel.service'; then
  echo "   已存在，保留当前配置："
  "${SSH[@]}" 'grep -E "^(User|Group|StateDirectory|ExecStart)=" /etc/systemd/system/llama-panel.service | sed "s/^/     /"'
  "${SSH[@]}" 'sudo -n systemctl daemon-reload && sudo -n systemctl restart llama-panel'
else
  cat > /tmp/llama-panel.service <<'EOF'
[Unit]
Description=llama-panel - llama.cpp 控制台
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/llama-panel/llama-panel
WorkingDirectory=/opt/llama-panel
Restart=on-failure
RestartSec=3
# 注意：绝不要加 StateDirectory=
# systemd(init_t) 会去 relabel 状态目录树，而树里的 SSH 私钥带 ssh_home_t 标签，
# SELinux 强制拒绝 ⇒ 服务卡在重启循环（2026-09-16 踩过）

[Install]
WantedBy=multi-user.target
EOF
  "${SCP[@]}" /tmp/llama-panel.service user@192.0.2.10:/tmp/llama-panel.service
  "${SSH[@]}" 'sudo -n install -o root -g root -m 0644 /tmp/llama-panel.service /etc/systemd/system/llama-panel.service && sudo -n systemctl daemon-reload && sudo -n systemctl enable --now llama-panel'
fi
sleep 3

echo "== 4) 冒烟 =="
"${SSH[@]}" 'echo -n "  服务: "; systemctl is-active llama-panel; echo -n "  身份: "; echo "[$(systemctl show llama-panel -p User --value)]（空=root）"'
PORT=$("${SSH[@]}" 'sudo -n sh -c "for f in /var/lib/llama-panel/settings.json /opt/llama-panel/settings.json; do [ -f \$f ] && python3 -c \"import json,sys;print(json.load(open(sys.argv[1])).get(\\\"listen_port\\\",8077))\" \$f && break; done" 2>/dev/null' | tail -1)
PORT=${PORT:-8077}
echo "   端口: $PORT"
echo -n "  根路径 HTTP: "; "${SSH[@]}" "curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:$PORT/"
echo -n "  style.css  : "; "${SSH[@]}" "curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:$PORT/style.css"
echo -n "  app.js     : "; "${SSH[@]}" "curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:$PORT/app.js"
echo -n "  /api/state : "; "${SSH[@]}" "curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:$PORT/api/state"
echo "  模型数与身份自检:"
"${SSH[@]}" "curl -s http://127.0.0.1:$PORT/api/state | python3 -c \"import json,sys; j=json.load(sys.stdin); print('    模型数:', len(j['units']), '| 模式:', j['openclaw']['mode'], '| OpenClaw 可达:', j['openclaw']['reachable'])\""
echo "DEPLOY-DONE"
