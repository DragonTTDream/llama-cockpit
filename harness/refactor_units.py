#!/usr/bin/env python3
"""refactor_units.py — 把 6 个 llama unit 改成 EnvironmentFile 驱动，使参数可从面板调节。

安全设计：
  * 改前确认备份目录里有原始文件，缺则拒绝执行
  * Environment= 里保留**当前生效值**作为默认（万一 .env 丢失，服务仍按原样启动）
  * EnvironmentFile=- 是可选的（`-` 前缀），文件缺失不报错
  * EXTRA 用 $$ 交给 shell 处理（实测：空值会被丢掉，不会变成空参数）
  * 幂等：已经是新结构的 unit 跳过
"""
import os
import shutil
import sys

SRC = "/etc/systemd/system"
ENVD = "/etc/llama.d"
BACKUP = "/etc/systemd/system/llama-backup-2026-09-16"

# unit 名 -> 模板类型
UNITS = {
    "general": "moe",
    "carnice": "moe",
    "coder": "moe",
    "heretic": "dense",
    "qwen38": "dense",
    "embed": "embed",
}

TEMPLATES = {
    "moe": ('exec /usr/local/bin/llama-server --model "${MODEL}" --host 0.0.0.0 '
            '--port "${PORT}" --alias "${ALIAS}" --jinja --n-gpu-layers "${NGL}" '
            '--n-cpu-moe "${NCPUMOE}" --ctx-size "${CTX}" --cache-type-k "${CTKV}" '
            '--cache-type-v "${CTVV}" --load-mode none $$EXTRA'),
    "dense": ('exec /usr/local/bin/llama-server --model "${MODEL}" --host 0.0.0.0 '
              '--port "${PORT}" --alias "${ALIAS}" --jinja --n-gpu-layers "${NGL}" '
              '--ctx-size "${CTX}" --cache-type-k "${CTKV}" --cache-type-v "${CTVV}" '
              '--load-mode none $$EXTRA'),
    "embed": ('exec /usr/local/bin/llama-server --model "${MODEL}" --host 0.0.0.0 '
              '--port "${PORT}" --alias "${ALIAS}" --embeddings -c "${CTX}" '
              '--n-gpu-layers "${NGL}" $$EXTRA'),
}

KEYS = {
    "moe": ["MODEL", "PORT", "ALIAS", "NGL", "NCPUMOE", "CTX", "CTKV", "CTVV", "EXTRA"],
    "dense": ["MODEL", "PORT", "ALIAS", "NGL", "CTX", "CTKV", "CTVV", "EXTRA"],
    "embed": ["MODEL", "PORT", "ALIAS", "NGL", "CTX", "EXTRA"],
}


def parse_exec(exec_line):
    args = exec_line.split()[1:]
    d = {}
    i = 0
    while i < len(args):
        a = args[i]
        two = {"--model": "MODEL", "--port": "PORT", "--alias": "ALIAS",
               "--n-gpu-layers": "NGL", "--n-cpu-moe": "NCPUMOE",
               "--ctx-size": "CTX", "-c": "CTX",
               "--cache-type-k": "CTKV", "--cache-type-v": "CTVV"}
        if a in two and i + 1 < len(args):
            d[two[a]] = args[i + 1]
            i += 2
        elif a in ("--load-mode", "--host"):
            i += 2
        else:
            i += 1
    d.setdefault("EXTRA", "")
    return d


def main():
    plan = []
    for name, kind in UNITS.items():
        src = f"{SRC}/llama-{name}.service"
        bak = f"{BACKUP}/llama-{name}.service"
        if not os.path.exists(bak):
            print(f"拒绝执行：备份缺失 {bak}")
            return 2
        text = open(src, encoding="utf-8").read()
        if "EnvironmentFile=-/etc/llama.d/" in text:
            print(f"跳过（已是新结构）：llama-{name}")
            continue
        lines = text.splitlines()
        idx = next((i for i, l in enumerate(lines) if l.startswith("ExecStart=")), None)
        if idx is None:
            print(f"拒绝执行：{src} 里找不到 ExecStart")
            return 3
        params = parse_exec(lines[idx])
        missing = [k for k in KEYS[kind] if k not in params]
        if missing:
            print(f"拒绝执行：{src} 解析不出 {missing}")
            return 4
        plan.append((name, kind, src, lines, idx, params))

    if not plan:
        print("没有需要改的 unit")
        return 0

    os.makedirs(ENVD, exist_ok=True)
    for name, kind, src, lines, idx, params in plan:
        env_line = "Environment=" + " ".join(f'"{k}={params[k]}"' for k in KEYS[kind])
        new_exec = "ExecStart=/bin/sh -c '" + TEMPLATES[kind] + "'"
        shutil.copy2(src, f"/tmp/llama-{name}.service.pre-refactor")
        lines[idx] = new_exec
        lines.insert(idx, f"EnvironmentFile=-/etc/llama.d/{name}.env")
        lines.insert(idx, env_line)
        open(src, "w", encoding="utf-8").write("\n".join(lines) + "\n")
        with open(f"{ENVD}/{name}.env", "w", encoding="utf-8") as fh:
            for k in KEYS[kind]:
                fh.write(f"{k}={params[k]}\n")
        os.chmod(f"{ENVD}/{name}.env", 0o644)
        print(f"已改造 llama-{name}（{kind}）：默认值 {params}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
