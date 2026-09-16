#!/bin/bash
# gate.sh — 同步 src/ 到 GPU 机的干净构建目录，跑机器验收门。只输出结论。
# 构建目录放在 /tmp（不再用 /opt/llama-panel/.build：那个目录已归 root，
# 而且把可写目录放在 /opt 下本身就会给「替换二进制」留下后门）。
set -uo pipefail
SRC=/home/user/.openclaw/workspace/projects/llama-panel/src
BUILD_DIR=/tmp/llama-panel-build
SSH=(ssh -o BatchMode=yes -o IdentitiesOnly=yes -i /home/user/.ssh/id_ed25519 user@192.0.2.10)

"${SSH[@]}" "rm -rf $BUILD_DIR && mkdir -p $BUILD_DIR" || { echo "GATE=DIR-FAIL"; exit 1; }
tar czf - -C "$SRC" . | "${SSH[@]}" "tar xzf - -C $BUILD_DIR" || { echo "GATE=SYNC-FAIL"; exit 1; }

"${SSH[@]}" "cd $BUILD_DIR && export GOTOOLCHAIN=local GOFLAGS=-mod=mod GOCACHE=\$HOME/.cache/go-build && {
  gofmt -l -e . > /tmp/lp-fmt.txt 2>&1
  go vet ./... > /tmp/lp-vet.txt 2>&1
  if [ -f main.go ]; then
    if CGO_ENABLED=0 go build -o /tmp/lp-bin . 2>/tmp/lp-build.txt; then b=OK; else b=FAIL; fi
  else
    b=SKIP
  fi
  echo \"FMT-BEGIN\"; head -20 /tmp/lp-fmt.txt; echo \"FMT-END\"
  echo \"VET-BEGIN\"; head -25 /tmp/lp-vet.txt; echo \"VET-END\"
  echo \"BUILD=\$b\"
  if [ \"\$b\" = FAIL ]; then echo \"ERR-BEGIN\"; head -40 /tmp/lp-build.txt; echo \"ERR-END\"; fi
  if [ \"\$b\" = OK ]; then file /tmp/lp-bin | sed 's/^/BIN: /'; fi
}"
