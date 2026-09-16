#!/bin/bash
# tier1-apply.sh — Tier 1：KV q8_0 → q4_0；MoE 窗口 131072 → 163840。
# 依据：deep_needle.py 实测（90% 深处定位 HIT）
#   A q8_0 131072 → 15793 MiB   ← 只剩 583 MiB 余量，危险
#   B q4_0 131072 → 12721 MiB   ← 同深度 HIT，省 3.1 GiB
#   C q4_0 163840 → 13681 MiB   ← 窗口 +25%，仍有余量
# 在 GPU 机上以 root 执行。
set -uo pipefail

declare -A PORT=([general]=1231 [carnice]=1232 [heretic]=1233 [coder]=1234 [qwen38]=1236)
CHAT_UNITS="general carnice heretic coder qwen38"

BAK=/etc/llama.d.bak-$(date +%s)
cp -a /etc/llama.d "$BAK"
echo "备份: $BAK"

# MoE 30B/35B：窗口扩到 160k；dense 27B：暂不动窗口，只换 KV（纯省显存）
for u in general coder carnice; do
  sed -i 's/^CTKV=.*/CTKV=q4_0/; s/^CTVV=.*/CTVV=q4_0/; s/^CTX=.*/CTX=163840/' /etc/llama.d/$u.env
done
for u in heretic qwen38; do
  sed -i 's/^CTKV=.*/CTKV=q4_0/; s/^CTVV=.*/CTVV=q4_0/' /etc/llama.d/$u.env
done

echo "改动后："
for u in $CHAT_UNITS; do
  printf "  %-9s " "$u"
  grep -hE '^(CTX|CTKV|CTVV)=' /etc/llama.d/$u.env | tr '\n' ' '
  echo
done

echo
echo "逐个验证装载（互斥，一次一个）："
FAILED=""
for u in $CHAT_UNITS; do
  p=${PORT[$u]}
  for c in $CHAT_UNITS; do systemctl stop "llama-$c" 2>/dev/null; done
  sleep 1
  systemctl start "llama-$u"
  ok=no
  for _ in $(seq 1 90); do
    if systemctl is-active --quiet "llama-$u" && curl -sf -o /dev/null "http://127.0.0.1:$p/health"; then ok=yes; break; fi
    sleep 1
  done
  if [ "$ok" = yes ]; then
    ctx=$(curl -s "http://127.0.0.1:$p/props" 2>/dev/null | python3 -c "
import json,sys
try:
    d=json.load(sys.stdin)
except Exception:
    print('?'); raise SystemExit
g=d.get('default_generation_settings',{})
print(g.get('n_ctx') or d.get('n_ctx') or '?')" 2>/dev/null)
    pvram=$(nvidia-smi --query-compute-apps=pid,used_memory --format=csv,noheader,nounits 2>/dev/null | paste -sd' ' -)
    printf "  %-9s OK    n_ctx=%-8s GPU进程: %s\n" "$u" "${ctx:-?}" "${pvram:-无}"
  else
    FAILED="$FAILED $u"
    printf "  %-9s 装载失败！\n" "$u"
    journalctl -u "llama-$u" -n 6 --no-pager 2>/dev/null | sed 's/^/       /'
  fi
  systemctl stop "llama-$u"
  sleep 1
done

if [ -n "$FAILED" ]; then
  echo
  echo "有 unit 失败：$FAILED"
  echo "回滚命令：sudo cp -a $BAK/. /etc/llama.d/ && sudo systemctl restart llama-general"
fi
echo "TIER1-DONE"
