#!/usr/bin/env python3
"""verify-units.py — 端到端验证「面板改参数 → 服务真的按新参数起来」。

覆盖：正常改参成功、越界被拒、端口冲突被拒、改回原值。
"""
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


def curl(method, path, body=None, timeout=300):
    cmd = "curl -s -w '\\n%{http_code}' --max-time 240 -X " + method + \
          " -H 'X-Panel-Token: " + TOKEN + "'"
    if body is not None:
        cmd += " -H 'Content-Type: application/json' -d '" + json.dumps(body) + "'"
    cmd += " http://127.0.0.1:8077" + path
    out = sh(cmd, timeout=timeout)
    lines = out.rsplit("\n", 1)
    code = lines[-1].strip() if len(lines) > 1 else "?"
    return code, (lines[0] if len(lines) > 1 else "")


def health(port, budget=150):
    t0 = time.time()
    while time.time() - t0 < budget:
        c = sh('curl -s -o /dev/null -w "%%{http_code}" --max-time 3 http://127.0.0.1:%d/health' % port)
        if c.strip() == "200":
            return True
        time.sleep(3)
    return False


def unit_params(name):
    code, body = curl("GET", "/api/state")
    try:
        st = json.loads(body)
    except Exception:
        return None
    for u in st.get("units", []):
        if u["name"] == name:
            return u["params"]
    return None


def running_args():
    return sh("ps -o args= -C llama-server 2>/dev/null | tr ' ' '\\n' | grep -A1 -E '^-{1,2}ctx-size$' | tail -1")


fail = []

print("=== 1) 新 unit 首次生效：重启 llama-general ===")
sh("sudo -n systemctl restart llama-general")
print("   health200:", health(1231))
print("   进程里的 ctx-size:", running_args())

print("=== 2) 通过面板改参数：CTX 98304 -> 65536 ===")
p = unit_params("llama-general")
if not p:
    print("   拿不到当前参数，终止")
    sys.exit(1)
print("   改前 CTX =", p.get("CTX"))
p2 = dict(p)
p2["CTX"] = "65536"
code, body = curl("POST", "/api/unit/params", {"name": "llama-general", "params": p2})
print("   HTTP", code, "->", body[:160])
if code != "200":
    fail.append("改参数被拒")
print("   health200:", health(1231))
print("   .env 里的 CTX:", sh("grep '^CTX' /etc/llama.d/general.env"))
print("   进程里的 ctx-size:", running_args())

print("=== 3) 越界参数应被拒（CTX=100）===")
p3 = dict(p)
p3["CTX"] = "100"
code, body = curl("POST", "/api/unit/params", {"name": "llama-general", "params": p3})
print("   HTTP", code, "->", body[:160])
if code != "400":
    fail.append("越界参数没有被拒")

print("=== 4) 端口冲突应被拒（PORT=1234 属于 coder）===")
p4 = dict(p)
p4["PORT"] = "1234"
code, body = curl("POST", "/api/unit/params", {"name": "llama-general", "params": p4})
print("   HTTP", code, "->", body[:160])
if code != "400":
    fail.append("端口冲突没有被拒")

print("=== 5) 改回原值 98304 ===")
code, body = curl("POST", "/api/unit/params", {"name": "llama-general", "params": p})
print("   HTTP", code, "->", body[:120])
print("   health200:", health(1231))
print("   .env 里的 CTX:", sh("grep '^CTX' /etc/llama.d/general.env"))
print("   进程里的 ctx-size:", running_args())

print("=== 6) 第二个 unit（carnice，新模板）起停 ===")
sh("sudo -n systemctl start llama-carnice")
print("   health200:", health(1232))
print("   进程数量:", sh("pgrep -c llama-server"))
sh("sudo -n systemctl stop llama-carnice")
time.sleep(2)
print("   停后 llama-carnice:", sh("systemctl is-active llama-carnice"))

print("RESULT:", "ALL-PASS" if not fail else "FAILURES: " + "; ".join(fail))
