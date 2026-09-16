#!/usr/bin/env python3
"""gen.py <task> [--model coder|general] [--max-repair 2]

tasks/<task>.md 取规格 → 本地模型整文件成稿 → 写入 src/ → 机器验收门。
监工只看到一行结构化摘要；代码永远落盘，不进上下文。
"""
import argparse
import json
import os
import re
import subprocess
import sys
import time
import urllib.request

ROOT = "/home/user/.openclaw/workspace/projects/llama-panel"
BUILD_DIR = "/opt/llama-panel/.build"
HOST = "192.0.2.10"
KEY = "/home/user/.ssh/id_ed25519"
SSH = ["ssh", "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-i", KEY, f"user@{HOST}"]
UNIT = {"coder": "llama-coder", "general": "llama-general"}
PORT = {"coder": 1234, "general": 1231}

SYS = ("你是资深 Go 工程师。严格按规格实现。只输出完整文件内容本身；"
       "不要解释、不要 markdown 代码围栏、不要任何省略（禁止 // ... 之类占位）。")


def sh(cmd, timeout=900):
    p = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
    return p.stdout + p.stderr


def http_ok(port):
    try:
        with urllib.request.urlopen(f"http://{HOST}:{port}/health", timeout=4) as r:
            return r.status == 200
    except Exception:
        return False


def ensure_model(kind):
    if http_ok(PORT[kind]):
        return True
    subprocess.run(SSH + [f"sudo -n systemctl start {UNIT[kind]}"], capture_output=True, timeout=120)
    for _ in range(60):
        if http_ok(PORT[kind]):
            return True
        time.sleep(4)
    return False


def ask(kind, prompt, max_tokens=8000, timeout=1800):
    body = json.dumps({"model": UNIT[kind], "messages": [
        {"role": "system", "content": SYS}, {"role": "user", "content": prompt}],
        "max_tokens": max_tokens, "temperature": 0.0}).encode()
    req = urllib.request.Request(f"http://{HOST}:{PORT[kind]}/v1/chat/completions",
                                 data=body, headers={"Content-Type": "application/json"})
    t0 = time.time()
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        p = json.load(resp)
    return (p["choices"][0]["message"].get("content") or "",
            time.time() - t0, p.get("usage", {}), p["choices"][0].get("finish_reason"))


def strip_fences(t):
    t = t.strip()
    m = re.search(r"```(?:go|html|css|js|javascript)?\s*\n(.*?)```", t, re.S)
    if m and t.startswith("```"):
        t = m.group(1).strip()
    return t


def block(text, tag):
    m = re.search(rf"{tag}-BEGIN\n(.*?)\n{tag}-END", text, re.S)
    return m.group(1).strip() if m else ""


def run_gate():
    raw = sh([os.path.join(ROOT, "harness", "gate.sh")])
    if "GATE=" in raw and "FMT-BEGIN" not in raw:
        return {"fmt": "?", "vet": "?", "build": "?", "err": raw[:300], "raw": raw}
    bm = re.search(r"BUILD=(\w+)", raw)
    return {"fmt": block(raw, "FMT"), "vet": block(raw, "VET"),
            "build": bm.group(1) if bm else "?", "err": block(raw, "ERR"), "raw": raw}


def run_web_gate(rel=None):
    out = sh([os.path.join(ROOT, "harness", "gate-web.sh")])
    ext = re.search(r"EXTERNAL=(-?\d+)", out)
    mh = re.search(r"MISSING_HTML=\[(.*?)\]", out)
    ji = re.search(r"JS_IDS_NOT_IN_HTML=\[(.*?)\]", out)
    n = int(ext.group(1)) if ext else -1
    miss = mh.group(1).strip() if mh else "?"
    bad = ji.group(1).strip() if ji else "?"
    base = os.path.basename(rel or "")
    hc = re.search(r"HELPER_CONTRACT=(YES|NO)", out)
    helper_bad = (hc is not None) and (hc.group(1) == "NO")
    # 跨文件 id 检查只对负责该职责的文件生效：html 只管自己 id 齐不齐，js 只管引用的 id 存不存在
    check_ids = base == "index.html"
    check_js = base == "app.js"
    ok = (n == 0) and (not check_ids or miss == "") and (not check_js or bad == "") \
        and (not (helper_bad and check_js))
    note = []
    if helper_bad and check_js:
        note.append("$() 辅助函数与调用点风格不一致")
    if check_ids:
        note.append("缺id=[" + miss + "]")
    if check_js:
        note.append("js引用了不存在的id=[" + bad + "]")
    return {"ok": ok, "summary": f"ext={n} " + (" ".join(note) if note else "id检查本文件不适用"),
            "detail": ("前端验收失败：外部资源数=" + str(n) +
                       "；index.html 缺少的必需 id=[" + miss + "]" +
                       "；app.js 引用但 index.html 没有的 id=[" + bad + "]")}


def required_decls(spec):
    out = []
    for line in spec.splitlines():
        m = re.match(r"^\s*(?:func|type|var|const)\s+(\w+)", line)
        if m:
            out.append(m.group(1))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("task")
    ap.add_argument("--model", default="")
    ap.add_argument("--max-repair", type=int, default=2)
    a = ap.parse_args()

    spec = open(os.path.join(ROOT, "tasks", a.task + ".md"), encoding="utf-8").read()
    fb = re.search(r"^file:\s*(\S+)", spec, re.M)
    if not fb:
        print(f"{a.task} SPEC-ERROR(no file:)")
        return 2
    rel = fb.group(1)                       # 形如 src/g01_types.go
    mb = re.search(r"^model:\s*(\w+)", spec, re.M)
    kind = a.model or (mb.group(1) if mb else "coder")
    out_path = os.path.join(ROOT, rel)
    if not ensure_model(kind):
        print(f"{a.task} MODEL-UNREACHABLE({UNIT[kind]})")
        return 3

    contract = open(os.path.join(ROOT, "CONTRACT.md"), encoding="utf-8").read()
    sigs = []
    src_dir = os.path.join(ROOT, "src")
    for f in sorted(os.listdir(src_dir)):
        if f.endswith(".go") and os.path.join("src", f) != rel:
            for line in open(os.path.join(src_dir, f), encoding="utf-8"):
                if re.match(r"^(func|type|var|const)\s", line):
                    sigs.append(line.rstrip())
    base_prompt = (f"# 全局契约（必须遵守）\n{contract}\n\n"
                   f"# 已存在文件的顶层声明（可直接调用，勿重复定义）\n" + "\n".join(sigs) +
                   f"\n\n# 本任务规格\n{spec}\n\n"
                   f"# 输出\n把 {os.path.basename(rel)} 的完整内容输出出来，不要任何额外文字。")
    want = required_decls(spec)

    total = 0.0
    prompt = base_prompt
    for attempt in range(a.max_repair + 1):
        try:
            text, el, usage, fin = ask(kind, prompt)
        except Exception as e:
            print(f"{a.task} GEN-ERROR({type(e).__name__})")
            return 4
        total += el
        code = strip_fences(text)
        os.makedirs(os.path.dirname(out_path), exist_ok=True)
        open(out_path, "w", encoding="utf-8").write(code if code.endswith("\n") else code + "\n")
        missing = [d for d in want if not re.search(r"\b" + re.escape(d) + r"\b", code)]
        g = run_gate()
        base = os.path.basename(rel)
        fmt_files = [x.strip() for x in g["fmt"].splitlines() if x.strip()]
        cur_dirty = base in fmt_files
        wg = run_web_gate(rel) if rel.startswith("src/web/") else None
        web_ok = (wg is None) or wg["ok"]
        ok = ((not cur_dirty) and (not g["vet"]) and g["build"] in ("OK", "SKIP")
              and not missing and web_ok)
        webinfo = "" if wg is None else f" web={wg['summary']}"
        print(f"{a.task} try{attempt+1} {UNIT[kind]} lines={code.count(chr(10))} tokens={usage.get('completion_tokens')} "
              f"finish={fin} gen={el:.0f}s missing_decl={len(missing)}{missing[:6]} "
              f"fmt={'ok' if not cur_dirty else 'dirty'} vet={'ok' if not g['vet'] else 'err'} "
              f"build={g['build']}{webinfo}")
        if ok:
            print(f"{a.task} PASS total={total:.0f}s -> {rel}")
            return 0
        # 只差格式：机器修，不重生成
        if (not missing) and (not g["vet"]) and g["build"] in ("OK", "SKIP") and cur_dirty:
            sh(SSH + [f"cd {BUILD_DIR} && gofmt -w {base}"], timeout=120)
            cp = subprocess.run(["scp", "-q", "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes",
                                 "-i", KEY, f"user@{HOST}:{BUILD_DIR}/{base}", out_path],
                                capture_output=True, text=True, timeout=120)
            if cp.returncode == 0:
                print(f"{a.task} PASS(fmt-autofixed) total={total:.0f}s -> {rel}")
                return 0
        errs = []
        if missing:
            errs.append("缺少声明：" + ", ".join(missing))
        if g["err"]:
            errs.append(g["err"])
        if g["vet"]:
            errs.append(g["vet"])
        if cur_dirty:
            errs.append("gofmt 报告（可能需要格式化/语法错）：" + g["fmt"])
        if wg is not None and not wg["ok"]:
            errs.append(wg["detail"])
        if not errs:
            print(f"{a.task} FAIL(no actionable error) total={total:.0f}s")
            return 1
        print(f"{a.task}   diag: " + " | ".join(e.replace(chr(10), " ")[:200] for e in errs))
        prompt = (f"# 全局契约\n{contract}\n\n# 本任务规格\n{spec}\n\n"
                  f"# 你上一次的输出有问题\n" + "\n".join(errs) +
                  f"\n\n# 上一次的完整文件\n{code}\n\n"
                  f"# 要求\n输出 {os.path.basename(rel)} 的**完整**内容（不是补丁），修掉上述全部问题。")
    print(f"{a.task} FAIL after {a.max_repair+1} attempts total={total:.0f}s")
    return 1


if __name__ == "__main__":
    sys.exit(main())
