#!/usr/bin/env python3
"""add_opt.py — 给 llama unit 的 ExecStart 装上 $$OPT/$$EXTRA 通用参数通道（幂等）。

设计要点：
  * `$$OPT` 交给 shell：值为空时参数被丢掉，有值时正确拆词（systemd 的 ${OPT} 会产生空参数）
  * `--jinja` / `--load-mode none` 从模板里移出，改由 OPT 承载（默认值写进 Environment= 与 .env）
  * 原参数一个不动，只是换了个位置
"""
import os
import sys

SRC = "/etc/systemd/system"
ENVD = "/etc/llama.d"

UNITS = {
    "general": "moe",
    "carnice": "moe",
    "coder": "moe",
    "custom": "moe",
    "heretic": "dense",
    "qwen38": "dense",
    "embed": "embed",
}

TMPL = {
    "moe": ('exec /usr/local/bin/llama-server --model "${MODEL}" --host 0.0.0.0 '
            '--port "${PORT}" --alias "${ALIAS}" --n-gpu-layers "${NGL}" '
            '--n-cpu-moe "${NCPUMOE}" --ctx-size "${CTX}" --cache-type-k "${CTKV}" '
            '--cache-type-v "${CTVV}" $$OPT $$EXTRA'),
    "dense": ('exec /usr/local/bin/llama-server --model "${MODEL}" --host 0.0.0.0 '
              '--port "${PORT}" --alias "${ALIAS}" --n-gpu-layers "${NGL}" '
              '--ctx-size "${CTX}" --cache-type-k "${CTKV}" '
              '--cache-type-v "${CTVV}" $$OPT $$EXTRA'),
    "embed": ('exec /usr/local/bin/llama-server --model "${MODEL}" --host 0.0.0.0 '
              '--port "${PORT}" --alias "${ALIAS}" --embeddings -c "${CTX}" '
              '--n-gpu-layers "${NGL}" $$OPT $$EXTRA'),
}

OPT_DEFAULT = {"moe": "--load-mode none --jinja", "dense": "--load-mode none --jinja", "embed": ""}


def main():
    changed = []
    for name, kind in UNITS.items():
        path = f"{SRC}/llama-{name}.service"
        if not os.path.exists(path):
            print(f"跳过（不存在）：{path}")
            continue
        lines = open(path, encoding="utf-8").read().splitlines()
        out = []
        touched = False
        for line in lines:
            if line.startswith("ExecStart="):
                new = "ExecStart=/bin/sh -c '" + TMPL[kind] + "'"
                if line != new:
                    touched = True
                out.append(new)
            elif line.startswith("Environment=") and "OPT=" not in line:
                out.append(line + f' "OPT={OPT_DEFAULT[kind]}"')
                touched = True
            else:
                out.append(line)
        if touched:
            open(path, "w", encoding="utf-8").write("\n".join(out) + "\n")
            changed.append(name)

        envf = f"{ENVD}/{name}.env"
        if os.path.exists(envf):
            env = open(envf, encoding="utf-8").read().splitlines()
            if not any(l.startswith("OPT=") for l in env):
                env.append(f"OPT={OPT_DEFAULT[kind]}")
                open(envf, "w", encoding="utf-8").write("\n".join(env) + "\n")
    print("已更新 unit：" + (", ".join(changed) if changed else "（无变化）"))
    return 0


if __name__ == "__main__":
    sys.exit(main())
