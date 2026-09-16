#!/bin/bash
# run-tasks.sh — 顺序跑生成任务，遇错即停。用法：run-tasks.sh g02_gpu g03_units ...
set -uo pipefail
cd /home/user/.openclaw/workspace/projects/llama-panel
FAILED=""
for t in "$@"; do
  echo "##### $t 开始 $(date +%H:%M:%S)"
  python3 -u harness/gen.py "$t" --max-repair 2
  rc=$?
  echo "##### $t 结束 rc=$rc $(date +%H:%M:%S)"
  if [ $rc -ne 0 ]; then FAILED="$t"; echo "STOPPED-AFTER=$t"; break; fi
done
if [ -z "$FAILED" ]; then echo "ALL-DONE"; else echo "BLOCKED-BY=$FAILED"; fi
