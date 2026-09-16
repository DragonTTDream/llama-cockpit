#!/bin/bash
# health.sh — llama-panel 健康自检（只读）
set -uo pipefail
KEY="$HOME/.ssh/id_ed25519"
S="ssh -o BatchMode=yes -o IdentitiesOnly=yes -i $KEY user@192.0.2.10"

echo -n "面板服务:        "; $S 'systemctl is-active llama-panel'
echo -n "二进制时间戳:    "; $S 'stat -c %y /opt/llama-panel/llama-panel'
echo -n "开机自启:        "; $S 'systemctl is-enabled llama-panel 2>&1'

$S 'TOKEN=$(sudo -n cat /opt/llama-panel/token); \
  echo -n "根路径 HTTP:     "; curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8077/; \
  echo -n "无令牌 HTTP:     "; curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8077/api/state; \
  echo "state 摘要:"; \
  curl -s -H "X-Panel-Token: $TOKEN" http://127.0.0.1:8077/api/state | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(\"  ok=\", d[\"ok\"])
print(\"  openclaw=\", d[\"openclaw\"])
print(\"  gpu=\", d[\"gpu\"][\"mem_used_mib\"], \"/\", d[\"gpu\"][\"mem_total_mib\"], \"MiB\")
for u in d[\"units\"]:
    print(\"   \", u[\"name\"].ljust(16), u[\"state_label\"], str(u[\"vram_pct\"]) + \"%\", u[\"health\"])
"'
echo -n "残留生成进程:    "; pgrep -f "harness/gen.py" >/dev/null && echo "有" || echo "无"
