#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""校验仪表盘排序/隐藏、设置页 SSH 表单、OpenClaw 实时状态网格。"""
import json
import re
import subprocess
import urllib.request

EDGE = "microsoft-edge"
BASE = "http://127.0.0.1:8077"


def dom(path, tag):
    subprocess.run(["rm", "-rf", "/tmp/vp_" + tag], check=False)
    out = subprocess.run(
        [EDGE, "--headless=new", "--disable-gpu", "--no-sandbox",
         "--user-data-dir=/tmp/vp_" + tag, "--virtual-time-budget=13000",
         "--dump-dom", BASE + "/" + path],
        capture_output=True, text=True, timeout=120).stdout
    return out


def grab(html, start_attr, end_marker):
    if start_attr not in html:
        return ""
    return html.split(start_attr, 1)[1].split(end_marker, 1)[0]


def main():
    st = json.load(urllib.request.urlopen(BASE + "/api/state"))
    print("=== 当前 active 状态 ===")
    for u in st["units"]:
        print("  %-16s active=%s" % (u["name"], u["active"]))

    print("\n=== 仪表盘 ===")
    h = dom("#dash", "dash")
    box = grab(h, 'id="dash-units"', "</section>")
    summ = re.search(r"运行中 \d+ / 共 \d+ 个[^<]*", box)
    print("  汇总行:", summ.group(0) if summ else "(缺失)")
    names = re.findall(r'<div class="dim">(llama-[a-z0-9-]+)</div>', box)
    if not names:
        names = re.findall(r'class="dim">(llama-[a-z0-9-]+)', box)
    print("  卡片顺序:")
    for i, n in enumerate(names, 1):
        print("    %d. %s" % (i, n))
    print("  llama-custom 出现:", "是 ✗" if any("custom" in n for n in names) else "否 ✓")
    print("  错误条:", "显示 ✗" if re.search(r'id="error-bar" class="error-bar"', h) else "隐藏 ✓")

    print("\n=== 设置页 SSH 表单 ===")
    h2 = dom("#settings", "set")
    for i in ["ssh-host", "ssh-user", "ssh-port", "ssh-scripts", "ssh-keypath", "ssh-key"]:
        m = re.search(r'id="%s"[^>]*value="([^"]*)"' % i, h2)
        print("  %-12s 存在=%s value=%s" % (i, ('id="%s"' % i) in h2,
                                            m.group(1) if m else "(JS 填入)"))
    m = re.search(r'id="ssh-key-state"[^>]*>([^<]*)', h2)
    print("  密钥状态:", (m.group(1) if m else "—")[:80])

    print("\n=== OpenClaw 实时状态网格 ===")
    h3 = dom("#oc", "oc")
    box3 = grab(h3, 'id="oc-info"', "</section>")
    keys = re.findall(r'class="info-key">([^<]*)', box3)
    vals = re.findall(r'class="info-val">([^<]*)', box3)
    if not keys:
        print("  (网格为空)", box3[:120])
    for k, v in zip(keys, vals):
        print("  %-14s %s" % (k, v))


if __name__ == "__main__":
    main()
