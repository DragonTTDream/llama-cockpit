#!/bin/bash
# gate-web.sh — 前端验收：① 无任何外部资源 ② index.html 必需 id 齐全 ③ app.js 用的 id 必须在 html 里存在
set -uo pipefail
W=/home/user/.openclaw/workspace/projects/llama-panel/src/web
REQ="login-layer login-token login-msg btn-login error-bar current-model mode-badge last-update tabs btn-tab-dash btn-tab-models btn-tab-load btn-tab-hub btn-tab-oc btn-tab-settings panel-dash panel-models panel-load panel-hub panel-oc panel-settings dash-cpu-bar dash-cpu-text dash-cpu-cores dash-load dash-mem-bar dash-mem-text dash-swap-text dash-gpu-bar dash-gpu-text dash-gpu-util dash-gpu-temp dash-gpu-name dash-units model-select model-state model-params model-msg btn-model-start btn-model-stop btn-model-restart btn-params-save btn-params-reset custom-model custom-port custom-params custom-status btn-custom-load btn-custom-stop chat-models-port chat-models-stat chat-models-log chat-models-input btn-chat-models-send btn-chat-models-clear chat-load-port chat-load-stat chat-load-log chat-load-input btn-chat-load-send btn-chat-load-clear hf-query btn-hf-search hf-msg hf-results hf-files hf-autoreg hf-kind hf-port hf-progress autostart-toggle autostart-label auth-toggle auth-label auth-msg add-name add-label add-file add-kind add-port btn-add-model add-msg model-list panel-info oc-mode oc-primary oc-gateway oc-msg oc-info oc-checked panel-port btn-port-save btn-panel-restart port-msg mode-state mode-msg btn-mode-root btn-mode-user dash-gpu-procs ssh-host ssh-user ssh-port ssh-scripts ssh-keypath ssh-key ssh-msg ssh-key-state btn-ssh-save btn-ssh-test btn-ssh-secret btn-ssh-secret-clear btn-cloud btn-local btn-gw-restart btn-gw-force"

ext=0
for f in "$W"/index.html "$W"/style.css "$W"/app.js; do
  [ -f "$f" ] || continue
  n=$(grep -oE 'https?://|//cdn|url\(http' "$f" | wc -l)
  ext=$((ext + n))
done

miss=""
for i in $REQ; do
  grep -q "id=\"$i\"" "$W/index.html" 2>/dev/null || miss="$miss $i"
done

used=$( { grep -oE "getElementById\('[^']+'\)" "$W/app.js" 2>/dev/null | sed -E "s/.*'(.*)'.*/\1/"; \
          grep -oE "\$\('#[^']+'\)" "$W/app.js" 2>/dev/null | sed -E "s/.*#'(.*)'.*/\1/"; } | sort -u )
bad=""
for i in $used; do
  grep -q "id=\"$i\"" "$W/index.html" 2>/dev/null || bad="$bad $i"
done

# 辅助函数契约：$ 收裸 id 时，调用点绝不能写 $('#x')（否则每个都返回 null）
helper_ok=YES
if grep -qE "getElementById\(id\)" "$W/app.js" && grep -q "\$('#" "$W/app.js"; then
  helper_ok=NO
fi
echo "HELPER_CONTRACT=$helper_ok"

echo "EXTERNAL=$ext"
echo "MISSING_HTML=[${miss# }]"
echo "JS_IDS_NOT_IN_HTML=[${bad# }]"
echo "SIZES:"
wc -c "$W"/index.html "$W"/style.css "$W"/app.js 2>/dev/null | head -4
