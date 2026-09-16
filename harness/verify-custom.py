#!/usr/bin/env python3
"""verify-custom.py — 验证自定义加载、对话流式、开机自启开关。"""
import json
import subprocess
import sys
import time

KEY = "/home/user/.ssh/id_ed25519"
SSH = ["ssh", "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-i", KEY, "user@192.0.2.10"]


def sh(cmd, timeout=300):
    p = subprocess.run(SSH + [cmd], capture_output=True, text=True, timeout=timeout)
    return (p.stdout + p.stderr).strip()


TOKEN = sh("sudo -n cat /opt/llama-panel/token")


def curl(method, path, body=None, max_time=240):
    cmd = "curl -s -w '\\n%%{http_code}' --max-time %d -X %s -H 'X-Panel-Token: %s'" % (
        max_time, method, TOKEN)
    if body is not None:
        cmd += " -H 'Content-Type: application/json' -d '" + json.dumps(body) + "'"
    cmd += " http://127.0.0.1:8077" + path
    out = sh(cmd, timeout=max_time + 60)
    lines = out.rsplit("\n", 1)
    return (lines[-1].strip() if len(lines) > 1 else "?"), (lines[0] if len(lines) > 1 else "")


def health(port, budget=180):
    t0 = time.time()
    while time.time() - t0 < budget:
        c = sh('curl -s -o /dev/null -w "%%{http_code}" --max-time 3 http://127.0.0.1:%d/health' % port)
        if c.strip() == "200":
            return True
        time.sleep(4)
    return False


fail = []

print("=== A) 自定义加载（30B MoE @1240, NGL=999 CTX=32768）===")
code, body = curl("POST", "/api/custom/load", {
    "model_file": "Qwen3-30B-A3B-Instruct-2507-Q4_K_M.gguf",
    "port": 1240,
    "params": {"NGL": "999", "CTX": "32768"}}, max_time=120)
print("   HTTP", code, "->", body[:200])
if code != "200":
    fail.append("自定义加载失败")
print("   /health@1240:", health(1240))
print("   llama-custom 状态:", sh("systemctl is-active llama-custom"))
print("   显存:", sh("nvidia-smi --query-gpu=memory.used --format=csv,noheader"))

print("=== B) 对话流式（走面板 /api/chat，SSE）===")
out = sh("curl -s -N --max-time 150 -X POST -H 'X-Panel-Token: %s' "
         "-H 'Content-Type: application/json' "
         "-d '{\"port\":1240,\"message\":\"用一句话说明你是什么模型\",\"max_tokens\":48}' "
         "http://127.0.0.1:8077/api/chat | head -c 700" % TOKEN, timeout=220)
events = [l for l in out.splitlines() if l.startswith("data: ")]
print("   SSE 事件块数:", len(events))
print("   前 3 块:", events[:3])
print("   末块:", events[-1] if events else "(无)")
if not events or "delta" not in "".join(events[:5]):
    fail.append("对话没有产生 delta 事件")

print("=== C) 停止自定义加载 ===")
code, body = curl("POST", "/api/custom/stop", {}, max_time=90)
print("   HTTP", code, "->", body[:120])
time.sleep(3)
print("   llama-custom 状态:", sh("systemctl is-active llama-custom"))

print("=== D) 开机自启开关 ===")
code, body = curl("POST", "/api/panel/autostart", {"enabled": True}, max_time=60)
print("   开启 HTTP", code, "->", body[:120], "| is-enabled =", sh("systemctl is-enabled llama-panel"))
code, body = curl("POST", "/api/panel/autostart", {"enabled": False}, max_time=60)
print("   关闭 HTTP", code, "->", body[:120], "| is-enabled =", sh("systemctl is-enabled llama-panel 2>&1"))

print("RESULT:", "ALL-PASS" if not fail else "FAILURES: " + "; ".join(fail))
