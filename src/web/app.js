const $ = (id) => document.getElementById(String(id).replace(/^#/, ''));

const TABS = ['dash', 'models', 'load', 'hub', 'oc', 'settings'];
const KIND_LABEL = { moe: 'moe（混合专家）', dense: 'dense（稠密）', embed: 'embed（嵌入向量）' };

let SCHEMA = null;
let LAST = null;
let MODELS = [];
let MODELFILES = [];
let INFO = null;
let timer = null;
let hfTimer = null;
let CURTAB = 'dash';
let MODELKEY = 'llama-general';

const chats = {};

function api(path, opts = {}) {
  const headers = { 'Content-Type': 'application/json', ...opts.headers };
  const method = opts.method || 'GET';
  const body = opts.body ? JSON.stringify(opts.body) : undefined;

  return fetch(path, { method, headers, body })
    .then(res => {
      if (res.status === 401) {
        throw new Error('unauthorized');
      }
      return res.json().then(j => {
        if (!j.ok) {
          throw new Error(j.error || `HTTP ${res.status}`);
        }
        return j;
      });
    })
    .catch(err => {
      if (err.message === 'unauthorized') {
        showLogin();
      }
      throw err;
    });
}

function showError(msg) {
  const bar = $('#error-bar');
  bar.textContent = msg;
  bar.classList.remove('hidden');
  bar.onclick = () => {
    bar.classList.add('hidden');
    bar.onclick = null;
  };
}

function clearError() {
  $('#error-bar').classList.add('hidden');
}

function busy(btn, on) {
  if (btn) btn.disabled = on;
}

function pct(a, b) {
  return b > 0 ? (a / b * 100) : 0;
}

function fmtBytes(n) {
  if (!n || n <= 0) return '0 B';
  const u = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${u[i]}`;
}

function fmtDur(s) {
  s = Math.floor(s || 0);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d} 天 ${h} 小时`;
  if (h > 0) return `${h} 小时 ${m} 分`;
  return `${m} 分 ${s % 60} 秒`;
}

function setTab(name) {
  if (!TABS.includes(name)) name = 'dash';
  TABS.forEach(t => {
    const btn = $('#btn-tab-' + t);
    const panel = $('#panel-' + t);
    if (btn) btn.classList.toggle('active', t === name);
    if (panel) panel.classList.toggle('hidden', t !== name);
  });
  CURTAB = name;
  if (window.location.hash !== '#' + name) {
    history.replaceState(null, '', '#' + name);
  }
  if (name === 'hub') {
    renderHub();
    loadModelFiles().catch(() => {});
    if (LAST) renderModelList(LAST);
    if (!hfTimer) hfTimer = setInterval(renderHubProgress, 1500);
  } else if (hfTimer) {
    clearInterval(hfTimer);
    hfTimer = null;
  }
  if (LAST) {
    renderTab(LAST);
  }
  if (name === 'settings') {
    loadPanelInfo();
    loadSSH().catch(() => {});
    loadMode().catch(() => {});
  }
}

function renderTab(s) {
  if (CURTAB === 'models') {
    renderModels(s);
  } else if (CURTAB === 'load') {
    renderCustom(s);
  } else if (CURTAB === 'oc') {
    renderOpenClaw(s);
  } else if (CURTAB === 'settings') {
    renderSettings(s);
  } else if (CURTAB === 'hub') {
    renderModelList(s);
  }
}

function showLogin() {
  $('#login-layer').classList.remove('hidden');
  $('#login-token').value = '';
  $('#login-token').focus();
}

function refresh() {
  if (document.hidden) return;
  api('/api/state')
    .then(s => {
      LAST = s;
      $('#login-layer').classList.add('hidden');
      render(s);
      renderTab(s);
    })
    .catch(err => {
      if (err.message === 'unauthorized') {
        showLogin();
      } else {
        showError(err.message);
      }
    });
}

function loadSchema() {
  Promise.all([
    api('/api/schema').then(j => { SCHEMA = j; }),
    api('/api/models').then(j => { MODELS = j.models; })
  ]).catch(err => {
    showError(err.message);
  });
}

/* ==================== 仪表盘（只读） ==================== */

function render(s) {
  $('#last-update').textContent = new Date().toISOString().replace('T', ' ').slice(0, 19);
  const activeUnit = s.units.find(u => u.active && !u.coexist);
  $('#current-model').textContent = activeUnit ? `${activeUnit.label}（${activeUnit.name}）` : '未运行';
  const mode = s.openclaw.mode;
  $('#mode-badge').textContent = mode === 'local' ? '本地模式' : '云端模式';
  $('#mode-badge').className = `mode-${mode}`;

  const gpu = s.gpu;
  $('#dash-gpu-name').textContent = gpu.name;
  $('#dash-gpu-bar').style.width = pct(gpu.mem_used_mib, gpu.mem_total_mib) + '%';
  $('#dash-gpu-text').textContent = `${gpu.mem_used_mib} / ${gpu.mem_total_mib} MiB`;
  $('#dash-gpu-util').textContent = `利用率 ${gpu.util_pct}%`;
  $('#dash-gpu-temp').textContent = `温度 ${gpu.temp_c}℃`;
  const opg = gpu.other_procs || [];
  const opgEl = $('#dash-gpu-procs');
  if (opg.length) {
    opgEl.textContent = '⚠ 无归属进程：' + opg
      .map(p => `${p.name}(PID ${p.pid}) ${p.used_mib} MiB`).join(' · ');
    opgEl.className = 'msg-warn';
  } else {
    opgEl.textContent = '';
    opgEl.className = 'dim';
  }

  const cpu = s.cpu;
  $('#dash-cpu-bar').style.width = cpu.util_pct + '%';
  $('#dash-cpu-text').textContent = `CPU ${cpu.util_pct}% · ${cpu.count} 核`;
  $('#dash-load').textContent = `负载 ${cpu.load1} / ${cpu.load5} / ${cpu.load15}`;
  const cores = $('#dash-cpu-cores');
  cores.innerHTML = '';
  (cpu.cores || []).forEach(v => {
    const div = document.createElement('div');
    div.className = 'core-bar';
    const fill = document.createElement('div');
    fill.className = 'core-fill';
    fill.style.height = Math.max(0, Math.min(100, v)) + '%';
    div.appendChild(fill);
    cores.appendChild(div);
  });

  const mem = s.mem;
  $('#dash-mem-bar').style.width = pct(mem.used_mib, mem.total_mib) + '%';
  $('#dash-mem-text').textContent = `${mem.used_mib} / ${mem.total_mib} MiB (${pct(mem.used_mib, mem.total_mib).toFixed(1)}%)`;
  if (!mem.swap_total_mib) {
    $('#dash-swap-text').textContent = 'Swap 未启用';
  } else {
    $('#dash-swap-text').textContent = `Swap ${mem.swap_used_mib} / ${mem.swap_total_mib} MiB`;
  }

  const units = $('#dash-units');
  units.innerHTML = '';

  // 排序：自定义加载永远是模板，排最后；运行中的排最前；其余保持原顺序（sort 稳定）
  const ordered = s.units.slice().sort((a, b) => {
    const ac = a.name === 'llama-custom' ? 1 : 0;
    const bc = b.name === 'llama-custom' ? 1 : 0;
    if (ac !== bc) return ac - bc;
    return (a.active ? 0 : 1) - (b.active ? 0 : 1);
  });
  const running = ordered.filter(u => u.active).length;

  const summary = document.createElement('div');
  summary.className = 'dim';
  summary.style.marginBottom = '8px';
  summary.textContent = `运行中 ${running} / 共 ${ordered.length} 个 · 运行中的排在最前`;
  units.appendChild(summary);

  ordered.forEach(u => {
    // 自定义加载是空模板（模型在加载时才指定），没跑的时候不上仪表盘，免得像坏了一样
    if (u.name === 'llama-custom' && !u.active) return;
    const card = document.createElement('div');
    card.className = 'dash-card' + (u.installed ? '' : ' dash-missing');
    if (u.installed) {
      card.title = '点击进入该模型的调试页';
      card.addEventListener('click', () => {
        MODELKEY = u.name;
        setTab('models');
      });
    }

    const head = document.createElement('div');
    head.className = 'unit-head';
    const title = document.createElement('div');
    title.className = 'unit-title';
    title.textContent = u.label;
    const name = document.createElement('div');
    name.className = 'dim';
    name.textContent = u.name;
    head.appendChild(title);
    head.appendChild(name);
    const sub = document.createElement('div');
    sub.className = 'unit-sub';
    sub.textContent = `端口 ${u.port} · ${u.model || '（未指定模型）'}${u.user_added ? ' · 面板添加' : ''}${u.installed ? '' : ' · unit 缺失'}`;
    head.appendChild(sub);
    const status = document.createElement('span');
    status.className = `status-dot ${u.active && u.health === 'ok' ? 'dot-on' : u.active ? 'dot-busy' : 'dot-off'}`;
    const label = document.createElement('span');
    label.className = 'dim';
    label.textContent = u.state_label;
    head.appendChild(status);
    head.appendChild(label);
    card.appendChild(head);

    const bar = document.createElement('div');
    bar.className = 'bar';
    const fill = document.createElement('div');
    fill.className = 'bar-fill';
    fill.style.width = u.vram_pct + '%';
    bar.appendChild(fill);
    const text = document.createElement('div');
    text.className = 'bar-text';
    text.textContent = `${u.vram_mib} / ${s.gpu.mem_total_mib} MiB · ${u.vram_pct.toFixed(1)}%`;
    bar.appendChild(text);
    card.appendChild(bar);

    const health = document.createElement('div');
    health.className = 'dim';
    health.textContent = u.health === 'ok' ? '服务正常' : (u.active ? '启动中/异常' : '未运行');
    card.appendChild(health);

    units.appendChild(card);
  });
}

/* ==================== 模型调试 ==================== */

function renderModels(s) {
  const select = $('#model-select');
  const oldVal = select.value;
  select.innerHTML = '';
  s.units.forEach(u => {
    const opt = document.createElement('option');
    opt.value = u.name;
    opt.textContent = `${u.label}（${u.name}）${u.installed ? '' : ' · unit 缺失'}`;
    select.appendChild(opt);
  });
  const want = MODELKEY || oldVal;
  const found = select.querySelector(`option[value="${want}"]`);
  select.value = found ? want : (s.units[0] ? s.units[0].name : '');
  MODELKEY = select.value;

  const unit = s.units.find(u => u.name === MODELKEY);
  if (unit) {
    $('#model-state').textContent = `${unit.state_label} · 端口 ${unit.port} · ${unit.health}`;
    renderParamForm($('#model-params'), unit, 'models');
  }
  renderChatPorts(s);
}

function renderParamForm(container, unit, mode) {
  container.innerHTML = '';
  if (!SCHEMA) {
    container.textContent = '参数模式载入中…';
    return;
  }
  const groups = SCHEMA.groups;
  const order = SCHEMA.order;
  const kinds = SCHEMA.kinds;
  const scopes = kinds[unit.kind] || ['all'];

  groups.forEach(group => {
    const title = document.createElement('div');
    title.className = 'param-group-title';
    title.textContent = group;
    container.appendChild(title);

    const grid = document.createElement('div');
    grid.className = 'param-grid';
    container.appendChild(grid);

    order.forEach(k => {
      const spec = SCHEMA.params[k];
      if (!spec || spec.group !== group) return;

      const ok = scopes.includes(spec.scope);
      const row = document.createElement('div');
      row.className = 'param-row' + (ok ? '' : ' param-disabled');

      const label = document.createElement('div');
      label.className = 'param-label';
      label.textContent = ok ? spec.label : '【该模型不适用】';
      if (!ok) label.title = `${spec.label}：该类型模型没有这个参数`;
      row.appendChild(label);

      const control = document.createElement('div');
      control.className = 'param-control';
      let customInput = null;
      let primary = null;
      const current = (unit.resolved && unit.resolved[k]) || '';

      if (spec.type === 'model') {
        primary = document.createElement('select');
        primary.dataset.key = k;
        primary.disabled = !ok;
        const found = MODELS.find(m => m.file === current);
        if (!found && current) {
          const opt = document.createElement('option');
          opt.value = current;
          opt.textContent = current;
          primary.appendChild(opt);
        }
        MODELS.forEach(m => {
          const opt = document.createElement('option');
          opt.value = m.file;
          opt.textContent = m.file;
          primary.appendChild(opt);
        });
        primary.value = current;
        control.appendChild(primary);
      } else if (spec.type === 'str') {
        primary = document.createElement('input');
        primary.type = 'text';
        primary.dataset.key = k;
        primary.value = current;
        primary.disabled = !ok;
        control.appendChild(primary);
      } else if (spec.type === 'enum' || spec.type === 'bool') {
        primary = document.createElement('select');
        primary.dataset.key = k;
        primary.disabled = !ok;
        (spec.options || []).forEach(opt => {
          const option = document.createElement('option');
          option.value = opt;
          option.textContent = opt === '' ? '默认' : opt;
          primary.appendChild(option);
        });
        primary.value = current;
        control.appendChild(primary);
      } else {
        // int / float：下拉框给常用档位，外加「自定义…」
        primary = document.createElement('select');
        primary.dataset.key = k;
        primary.disabled = !ok;
        (spec.options || []).forEach(opt => {
          const option = document.createElement('option');
          option.value = opt;
          option.textContent = opt === '' ? '默认' : opt;
          primary.appendChild(option);
        });
        const customOpt = document.createElement('option');
        customOpt.value = '__custom__';
        customOpt.textContent = '自定义…';
        primary.appendChild(customOpt);

        const hasPreset = primary.querySelector(`option[value="${current}"]`);
        primary.value = hasPreset ? current : '__custom__';
        control.appendChild(primary);

        customInput = document.createElement('input');
        customInput.type = 'number';
        customInput.step = 'any';
        customInput.dataset.custom = k;
        customInput.disabled = !ok;
        customInput.value = primary.value === '__custom__' ? current : '';
        customInput.style.display = primary.value === '__custom__' ? 'inline-block' : 'none';
        control.appendChild(customInput);

        primary.addEventListener('change', () => {
          const isCustom = primary.value === '__custom__';
          customInput.style.display = isCustom ? 'inline-block' : 'none';
          if (isCustom) customInput.focus();
          validate();
        });
        customInput.addEventListener('input', validate);
      }

      row.appendChild(control);

      const note = document.createElement('div');
      note.className = 'param-note';
      const noteText = () => `${spec.note || ''}${spec.safe ? '（安全值 ' + spec.safe + '）' : ''}`;
      note.textContent = noteText();
      row.appendChild(note);

      function validate() {
        if (spec.type !== 'int' && spec.type !== 'float') return;
        if (!ok) return;
        let val = null;
        if (primary.value === '__custom__') {
          val = parseFloat(customInput.value);
          customInput.classList.remove('param-bad');
          if (customInput.value !== '' && (isNaN(val) || val < spec.min || val > spec.max)) {
            customInput.classList.add('param-bad');
            note.textContent = `超出范围（${spec.min} - ${spec.max}）`;
            return;
          }
        } else {
          val = parseFloat(primary.value);
          primary.classList.remove('param-bad');
          if (primary.value !== '' && !isNaN(val) && (val < spec.min || val > spec.max)) {
            primary.classList.add('param-bad');
            note.textContent = `超出范围（${spec.min} - ${spec.max}）`;
            return;
          }
        }
        note.textContent = noteText();
      }

      grid.appendChild(row);
    });
  });
}

function collectValues(container, unit) {
  const result = {};
  const nodes = container.querySelectorAll('[data-key], [data-custom]');
  nodes.forEach(el => {
    const key = el.dataset.key;
    const custom = el.dataset.custom;
    if (custom) {
      const input = container.querySelector(`[data-custom="${custom}"]`);
      if (input && input.value !== '') {
        result[custom] = input.value;
      }
      return;
    }
    if (el.tagName === 'SELECT' && el.value === '__custom__') return;
    if (el.value === '') return;
    result[key] = el.value;
  });
  return result;
}

/* ==================== 加载与对话 ==================== */

function renderCustom(s) {
  const modelSelect = $('#custom-model');
  const current = modelSelect.value;
  modelSelect.innerHTML = '';
  MODELS.forEach(m => {
    const opt = document.createElement('option');
    opt.value = m.file;
    opt.textContent = m.file;
    modelSelect.appendChild(opt);
  });
  if (current) {
    const found = modelSelect.querySelector(`option[value="${current}"]`);
    if (found) modelSelect.value = current;
  }

  const customUnit = s.units.find(u => u.name === 'llama-custom');
  const resolved = customUnit ? customUnit.resolved : {};
  renderParamForm($('#custom-params'), { name: 'llama-custom', kind: 'custom', resolved }, 'custom');
  renderChatPorts(s);
}

/* ==================== 对话组件（可复用，流式输出） ==================== */

function createChat(prefix) {
  const log = $('#chat-' + prefix + '-log');
  const input = $('#chat-' + prefix + '-input');
  const stat = $('#chat-' + prefix + '-stat');
  const sendBtn = $('#btn-chat-' + prefix + '-send');
  let sending = false;

  function push(role, text) {
    const wrap = document.createElement('div');
    wrap.className = 'chat-msg chat-' + role;
    const who = document.createElement('div');
    who.className = 'chat-who';
    who.textContent = role === 'user' ? '你' : '模型';
    const body = document.createElement('div');
    body.className = 'chat-body';
    body.textContent = text;
    wrap.appendChild(who);
    wrap.appendChild(body);
    log.appendChild(wrap);
    log.scrollTop = log.scrollHeight;
    return body;
  }

  async function send() {
    if (sending) return;
    const port = Number($('#chat-' + prefix + '-port').value);
    const message = input.value.trim();
    if (!message) return;
    if (!port) {
      showError('没有可用的对话目标：请先启动一个模型，或到「加载与对话」页加载');
      return;
    }
    sending = true;
    sendBtn.disabled = true;
    input.value = '';
    push('user', message);

    const body = push('assistant', '');
    body.classList.add('chat-cursor');
    const t0 = performance.now();
    let chars = 0;
    let firstAt = 0;

    try {
      const res = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ port, message, max_tokens: 1024 })
      });
      if (res.status === 401) { showLogin(); throw new Error('unauthorized'); }
      if (!res.ok) throw new Error(`HTTP ${res.status}`);

      const reader = res.body.getReader();
      const decoder = new TextDecoder('utf-8');
      let buffer = '';
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const chunks = buffer.split('\n\n');
        buffer = chunks.pop();
        for (const chunk of chunks) {
          if (!chunk.startsWith('data: ')) continue;
          let data;
          try { data = JSON.parse(chunk.slice(5)); } catch (e) { continue; }
          if (data.error) throw new Error(data.error);
          if (data.delta) {
            if (!firstAt) firstAt = performance.now();
            chars += data.delta.length;
            body.textContent += data.delta;
            log.scrollTop = log.scrollHeight;
          }
        }
      }

      const ms = performance.now() - t0;
      const secs = ms / 1000;
      const ttft = firstAt ? (firstAt - t0) : 0;
      stat.textContent = `${chars} 字 · 首字 ${ttft.toFixed(0)} ms · 总耗时 ${secs.toFixed(1)} s · ${(chars / (secs || 1)).toFixed(1)} 字/秒`;
      stat.className = 'msg-ok';
    } catch (err) {
      if (err.message === 'unauthorized') {
        body.textContent = body.textContent || '（未授权）';
      } else {
        body.classList.add('chat-error');
        body.textContent = (body.textContent || '') + `\n[出错] ${err.message}`;
        showError(err.message);
      }
      stat.textContent = '失败';
      stat.className = 'msg-bad';
    } finally {
      body.classList.remove('chat-cursor');
      sending = false;
      sendBtn.disabled = false;
      input.focus();
    }
  }

  return {
    send,
    clear() {
      log.innerHTML = '';
      stat.textContent = '—';
      stat.className = 'dim';
    }
  };
}

function renderChatPorts(s) {
  ['models', 'load'].forEach(prefix => {
    const select = $('#chat-' + prefix + '-port');
    if (!select) return;
    const current = select.value;
    select.innerHTML = '';
    const alive = s.units.filter(u => u.active && !u.coexist);
    alive.forEach(u => {
      const opt = document.createElement('option');
      opt.value = u.port;
      opt.textContent = `${u.label}（${u.port}）`;
      select.appendChild(opt);
    });
    const custom = s.units.find(u => u.name === 'llama-custom');
    if (custom && custom.active && !alive.some(u => u.port === custom.port)) {
      const opt = document.createElement('option');
      opt.value = custom.port;
      opt.textContent = `自定义加载（${custom.port}）`;
      select.appendChild(opt);
    }
    if (current) {
      const found = select.querySelector(`option[value="${current}"]`);
      if (found) select.value = current;
    }
  });
}

/* ==================== OpenClaw ==================== */

function renderOpenClaw(s) {
  const oc = s.openclaw || {};
  const box = $('#oc-info');
  if (!box) return;
  $('#oc-checked').textContent = oc.checked_at ? `· ${oc.checked_at} 读取` : '';

  const rows = [
    ['模式', oc.mode === 'local' ? '本地模式' : '云端模式'],
    ['主模型', oc.primary || '—'],
    ['回退链', (oc.fallbacks || []).join('  →  ') || '—'],
    ['网关', `${oc.gateway || 'unknown'}${oc.gateway_sub ? ' / ' + oc.gateway_sub : ''}`],
    ['网关自', oc.gateway_since || '—'],
    ['重启次数', oc.restarts || '0'],
    ['网关 PID', oc.gateway_pid || '—'],
    ['网关内存', oc.gateway_mem_mib ? `${oc.gateway_mem_mib} MiB` : '—'],
    ['OpenClaw', oc.version || '—'],
    ['连接目标', oc.target || '—'],
    ['读取延迟', oc.latency_ms != null ? `${oc.latency_ms} ms` : '—'],
    ['可达', oc.reachable ? '是' : '否'],
  ];
  if (oc.detail) rows.push(['备注', oc.detail]);

  box.innerHTML = '';
  rows.forEach(([k, v]) => {
    const kk = document.createElement('div');
    kk.className = 'info-key';
    kk.textContent = k;
    const vv = document.createElement('div');
    vv.className = 'info-val';
    vv.textContent = String(v);
    if (k === '可达' && !oc.reachable) vv.style.color = '#ef4d5a';
    box.appendChild(kk);
    box.appendChild(vv);
  });
}

/* ==================== 设置：跨机连接与密钥 ==================== */

function loadMode() {
  return api('/api/panel/mode').then(j => {
    const st = $('#mode-state');
    const isRoot = j.mode === 'root';
    st.textContent = isRoot
      ? '当前：以 root 运行 —— 面板拥有完整 root 权限'
      : `当前：以非特权用户 ${j.user} 运行 —— 只有写操作经 helper 提权`;
    st.className = isRoot ? 'msg-bad' : 'msg-ok';
    $('#btn-mode-root').disabled = isRoot;
    $('#btn-mode-user').disabled = !isRoot || !j.can_switch;
    if (j.selinux === 'Enforcing') {
      $('#mode-msg').textContent = '本机 SELinux 为 Enforcing —— 已规避已知的标签冲突（不使用 StateDirectory）';
      $('#mode-msg').className = 'dim';
    } else {
      $('#mode-msg').textContent = j.state_dir ? `状态目录：${j.state_dir}` : '—';
      $('#mode-msg').className = 'dim';
    }
  }).catch(err => showError(err.message));
}

function switchMode(mode) {
  const label = mode === 'root' ? 'root' : '非特权用户';
  if (mode === 'user' && !confirm(
      '切换到非特权用户运行？\n\n' +
      '会创建 llama-panel 用户、迁移状态到 /var/lib/llama-panel、\n' +
      '把 /opt/llama-panel 收紧为 root 所有，然后重启面板。\n\n' +
      '若起不来会自动回滚到 root。')) return;
  $('#mode-msg').textContent = `正在切到${label}…`;
  $('#mode-msg').className = 'dim';
  api('/api/panel/mode', { method: 'POST', body: { mode } })
    .then(j => {
      $('#mode-msg').textContent = j.msg;
      $('#mode-msg').className = 'msg-ok';
      // 面板大约 3 秒后重启，等一会儿再回读
      setTimeout(() => loadMode().catch(() => {}), 12000);
    })
    .catch(err => {
      $('#mode-msg').textContent = err.message;
      $('#mode-msg').className = 'msg-bad';
    });
}

function loadSSH() {
  return api('/api/panel/ssh').then(j => {
    const st = j.settings || {};
    $('#ssh-host').value = st.ssh_host || '';
    $('#ssh-user').value = st.ssh_user || '';
    $('#ssh-port').value = st.ssh_port || '';
    $('#ssh-scripts').value = st.scripts_dir || '';
    $('#ssh-keypath').value = st.ssh_key_path || '';
    if ($('#panel-port') && st.listen_port) $('#panel-port').value = st.listen_port;
    const state = $('#ssh-key-state');
    if (j.key_set) {
      state.textContent = '已加密保存私钥（优先于「密钥路径」）';
      state.className = 'msg-ok';
    } else {
      state.textContent = `未保存私钥，当前使用「密钥路径」：${j.key_path || '—'}`;
      state.className = 'dim';
    }
    if (j.key_error) showError('密钥读取异常：' + j.key_error);
  }).catch(err => showError(err.message));
}

/* ==================== 设置 ==================== */

function renderSettings(s) {
  $('#autostart-toggle').checked = s.panel.autostart;
  $('#auth-toggle').checked = s.panel.auth_required;
}

// 模型增删现在归「模型仓库」页
function renderModelList(s) {
  const box = $('#model-list');
  if (!box) return;
  box.innerHTML = '';
  s.units.forEach(u => {
    const row = document.createElement('div');
    row.className = 'model-row';

    const info = document.createElement('div');
    info.className = 'model-info';
    const title = document.createElement('div');
    title.className = 'unit-title';
    title.textContent = `${u.label}（${u.name}）`;
    const sub = document.createElement('div');
    sub.className = 'dim';
    sub.textContent = `类型 ${u.kind} · 端口 ${u.port} · ${u.installed ? 'unit 已安装' : 'unit 缺失'} · ${u.user_added ? '面板添加' : '内置'}`;
    info.appendChild(title);
    info.appendChild(sub);
    row.appendChild(info);

    const actions = document.createElement('div');
    actions.className = 'model-actions';

    if (!u.installed) {
      const rb = document.createElement('button');
      rb.className = 'btn btn-sm btn-success';
      rb.textContent = '恢复';
      rb.addEventListener('click', () => {
        api('/api/models/restore', { method: 'POST', body: { name: u.name } })
          .then(j => {
            $('#add-msg').textContent = j.msg;
            $('#add-msg').className = 'msg-ok';
            refresh();
          })
          .catch(err => showError(err.message));
      });
      actions.appendChild(rb);
    }

    if (u.name !== 'llama-custom' && u.name !== 'llama-embed') {
      const db = document.createElement('button');
      db.className = 'btn btn-sm btn-danger';
      db.textContent = '删除';
      db.addEventListener('click', () => {
        const extra = u.user_added ? '，并移除登记记录' : '（内置模型之后可用「恢复」重建）';
        if (!confirm(`删除 ${u.name}？会先停止服务，再移除 unit 文件与参数文件${extra}`)) return;
        api('/api/models/delete', { method: 'POST', body: { name: u.name } })
          .then(j => {
            $('#add-msg').textContent = j.msg;
            $('#add-msg').className = 'msg-ok';
            refresh();
          })
          .catch(err => showError(err.message));
      });
      actions.appendChild(db);
    }

    row.appendChild(actions);
    box.appendChild(row);
  });
}

function loadModelFiles() {
  return api('/api/model-files').then(j => {
    MODELFILES = j.files || [];
    const sel = $('#add-file');
    sel.innerHTML = '';
    MODELFILES.forEach(f => {
      const o = document.createElement('option');
      o.value = f.name;
      o.textContent = `${f.name}（${f.size_mib} MiB）${f.used ? ' · 已登记' : ''}`;
      sel.appendChild(o);
    });
    if (!$('#add-port').value) {
      $('#add-port').value = j.free_port;
    }
    const kinds = j.kinds || ['moe', 'dense', 'embed'];
    ['add-kind', 'hf-kind'].forEach(id => {
      const s = $('#' + id);
      if (!s || s.options.length) return;
      kinds.forEach(k => {
        const o = document.createElement('option');
        o.value = k;
        o.textContent = KIND_LABEL[k] || k;
        s.appendChild(o);
      });
    });
  });
}

function loadPanelInfo() {
  api('/api/panel/info').then(j => {
    INFO = j;
    const box = $('#panel-info');
    box.innerHTML = '';
    const rows = [
      ['主机', j.host],
      ['服务状态', j.service],
      ['监听端口', j.listen_port],
      ['运行时长', fmtDur(j.uptime_s)],
      ['二进制', `${j.binary}（${j.binary_mib} MiB · ${j.binary_time}）`],
      ['Go 版本', j.go_version],
      ['模型目录', j.models_dir],
      ['磁盘可用', `${j.disk_free_gib} / ${j.disk_total_gib} GiB`],
      ['模型登记表', `${j.units_file}（${j.user_units} 个面板添加）`]
    ];
    rows.forEach(([k, v]) => {
      const kk = document.createElement('div');
      kk.className = 'info-key';
      kk.textContent = k;
      const vv = document.createElement('div');
      vv.className = 'info-val';
      vv.textContent = String(v);
      box.appendChild(kk);
      box.appendChild(vv);
    });
  }).catch(err => {
    const box = $('#panel-info');
    box.textContent = '读取失败：' + err.message;
  });
}

/* ==================== 模型仓库（抱脸虫） ==================== */

function renderHub() {
  if (!$('#hf-results').dataset.loaded) {
    $('#hf-msg').textContent = '输入关键词后点「搜索」。也可以直接粘贴仓库名，如 Qwen/Qwen2.5-7B-Instruct-GGUF';
  }
  renderHubProgress();
}

function hfSearch() {
  const q = $('#hf-query').value.trim();
  if (!q) return;
  $('#hf-msg').textContent = '搜索中…';
  $('#hf-msg').className = 'dim';
  $('#hf-files').innerHTML = '';
  api('/api/hf/search?q=' + encodeURIComponent(q))
    .then(j => {
      $('#hf-msg').textContent = `关键词「${j.query}」· 命中 ${j.results.length} 个仓库（按下载量排序）`;
      const box = $('#hf-results');
      box.innerHTML = '';
      box.dataset.loaded = '1';
      if (!j.results.length) {
        box.textContent = '没有命中，换个关键词试试';
        return;
      }
      j.results.forEach(r => {
        const row = document.createElement('div');
        row.className = 'hf-row';
        const info = document.createElement('div');
        info.className = 'model-info';
        const t = document.createElement('div');
        t.className = 'unit-title';
        t.textContent = r.id;
        const s = document.createElement('div');
        s.className = 'dim';
        s.textContent = `下载 ${r.downloads} · 收藏 ${r.likes}${r.task ? ' · ' + r.task : ''}`;
        info.appendChild(t);
        info.appendChild(s);
        row.appendChild(info);
        const b = document.createElement('button');
        b.className = 'btn btn-sm btn-primary';
        b.textContent = '查看文件';
        b.addEventListener('click', () => hfFiles(r.id));
        row.appendChild(b);
        box.appendChild(row);
      });
    })
    .catch(err => {
      $('#hf-msg').textContent = '搜索失败：' + err.message;
      $('#hf-msg').className = 'msg-bad';
    });
}

function hfFiles(repo) {
  const box = $('#hf-files');
  box.innerHTML = '';
  $('#hf-msg').textContent = `读取 ${repo} 的文件…`;
  api('/api/hf/files?repo=' + encodeURIComponent(repo))
    .then(j => {
      $('#hf-msg').textContent = `${j.repo} · ${j.files.length} 个 GGUF 文件（按体积排序）`;
      if (!j.files.length) {
        box.textContent = '该仓库没有 GGUF 文件（可能是 safetensors 原始权重，llama.cpp 不能直接跑）';
        return;
      }
      j.files.forEach(f => {
        const row = document.createElement('div');
        row.className = 'hf-row';
        const info = document.createElement('div');
        info.className = 'model-info';
        const t = document.createElement('div');
        t.className = 'unit-title';
        t.textContent = f.name;
        const s = document.createElement('div');
        s.className = 'dim';
        s.textContent = `${f.size_mib} MiB${f.quant ? ' · ' + f.quant : ''}${f.local ? ' · 已下载' : ''}`;
        info.appendChild(t);
        info.appendChild(s);
        row.appendChild(info);

        if (!f.local) {
          const b = document.createElement('button');
          b.className = 'btn btn-sm btn-primary';
          b.textContent = '下载';
          b.addEventListener('click', () => {
            if (!confirm(`下载 ${f.name}（${f.size_mib} MiB）到 /home/user/models？`)) return;
            api('/api/hf/download', {
              method: 'POST',
              body: {
                repo: j.repo,
                file: f.name,
                auto_register: $('#hf-autoreg').checked,
                kind: $('#hf-kind').value,
                port: Number($('#hf-port').value) || 0
              }
            }).then(r => {
              $('#hf-msg').textContent = r.msg;
              $('#hf-msg').className = 'msg-ok';
              renderHubProgress();
            }).catch(err => showError(err.message));
          });
          row.appendChild(b);
        }
        box.appendChild(row);
      });
    })
    .catch(err => {
      $('#hf-msg').textContent = '读取失败：' + err.message;
      $('#hf-msg').className = 'msg-bad';
    });
}

function renderHubProgress() {
  if (CURTAB !== 'hub') return;
  api('/api/hf/progress').then(j => {
    const box = $('#hf-progress');
    box.innerHTML = '';
    const jobs = j.jobs || [];
    if (!jobs.length) {
      box.textContent = '没有下载任务';
      box.className = 'dim';
      return;
    }
    box.className = '';
    jobs.forEach(job => {
      const row = document.createElement('div');
      row.className = 'hf-row';
      const info = document.createElement('div');
      info.className = 'model-info';
      const t = document.createElement('div');
      t.className = 'unit-title';
      t.textContent = job.file;
      const p = job.total > 0 ? (job.done / job.total * 100) : 0;
      const s = document.createElement('div');
      s.className = 'dim';
      const statusText = job.status === 'running' ? '下载中'
        : job.status === 'done' ? '完成'
        : job.status === 'cancelled' ? '已取消' : '失败';
      s.textContent = `${statusText} · ${fmtBytes(job.done)} / ${job.total > 0 ? fmtBytes(job.total) : '未知'}${job.total > 0 ? ' · ' + p.toFixed(1) + '%' : ''}${job.error ? ' · ' + job.error : ''}`;
      const bar = document.createElement('div');
      bar.className = 'bar';
      const fill = document.createElement('div');
      fill.className = 'bar-fill';
      fill.style.width = p.toFixed(1) + '%';
      bar.appendChild(fill);
      info.appendChild(t);
      info.appendChild(s);
      info.appendChild(bar);
      row.appendChild(info);

      if (job.status === 'running') {
        const b = document.createElement('button');
        b.className = 'btn btn-sm btn-danger';
        b.textContent = '取消';
        b.addEventListener('click', () => {
          api('/api/hf/cancel', { method: 'POST', body: { id: job.id } })
            .then(() => renderHubProgress())
            .catch(err => showError(err.message));
        });
        row.appendChild(b);
      }
      box.appendChild(row);
    });
  }).catch(() => {});
}

/* ==================== 初始化 ==================== */

function init() {
  document.addEventListener('DOMContentLoaded', () => {
    loadSchema();
    refresh();
    if (!LAST) {
      showLogin();
    }
    timer = setInterval(refresh, 2000);
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) {
        refresh();
      }
    });
  });

  TABS.forEach(t => {
    const btn = $('#btn-tab-' + t);
    if (btn) btn.addEventListener('click', () => setTab(t));
  });

  chats.models = createChat('models');
  chats.load = createChat('load');

  $('#model-select').addEventListener('change', () => {
    MODELKEY = $('#model-select').value;
    if (LAST) renderModels(LAST);
  });

  $('#add-file').addEventListener('change', () => {
    const nameInput = $('#add-name');
    if (nameInput.value.trim()) return;
    const file = $('#add-file').value || '';
    const guess = file.replace(/\.gguf$/i, '').toLowerCase()
      .replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 28);
    nameInput.placeholder = guess ? `如 ${guess}` : '如 my-qwen3';
  });

  ['models', 'load'].forEach(prefix => {
    $('#btn-chat-' + prefix + '-send').addEventListener('click', () => chats[prefix].send());
    $('#btn-chat-' + prefix + '-clear').addEventListener('click', () => chats[prefix].clear());
    $('#chat-' + prefix + '-input').addEventListener('keypress', e => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        chats[prefix].send();
      }
    });
  });

  $('#btn-model-start').addEventListener('click', () => {
    const name = MODELKEY;
    busy($('#btn-model-start'), true);
    api('/api/unit/action', { method: 'POST', body: { name, action: 'start' } })
      .then(() => refresh())
      .catch(err => showError(err.message))
      .finally(() => busy($('#btn-model-start'), false));
  });

  $('#btn-model-stop').addEventListener('click', () => {
    const name = MODELKEY;
    busy($('#btn-model-stop'), true);
    api('/api/unit/action', { method: 'POST', body: { name, action: 'stop' } })
      .then(() => refresh())
      .catch(err => showError(err.message))
      .finally(() => busy($('#btn-model-stop'), false));
  });

  $('#btn-model-restart').addEventListener('click', () => {
    const name = MODELKEY;
    busy($('#btn-model-restart'), true);
    api('/api/unit/action', { method: 'POST', body: { name, action: 'restart' } })
      .then(() => refresh())
      .catch(err => showError(err.message))
      .finally(() => busy($('#btn-model-restart'), false));
  });

  $('#btn-params-save').addEventListener('click', () => {
    const name = MODELKEY;
    const params = collectValues($('#model-params'), { name });
    busy($('#btn-params-save'), true);
    api('/api/unit/params', { method: 'POST', body: { name, params } })
      .then(() => {
        $('#model-msg').textContent = '已保存并重启';
        $('#model-msg').className = 'msg-ok';
        refresh();
      })
      .catch(err => showError(err.message))
      .finally(() => busy($('#btn-params-save'), false));
  });

  $('#btn-params-reset').addEventListener('click', () => {
    api(`/api/unit/defaults?name=${MODELKEY}`)
      .then(j => {
        const unit = LAST.units.find(u => u.name === MODELKEY);
        if (unit) {
          unit.resolved = j.params;
          renderParamForm($('#model-params'), unit, 'models');
          $('#model-msg').textContent = '已填入默认值，点『保存并重启』生效';
          $('#model-msg').className = 'msg-ok';
        }
      })
      .catch(err => showError(err.message));
  });

  $('#btn-custom-load').addEventListener('click', () => {
    const modelFile = $('#custom-model').value;
    const port = Number($('#custom-port').value);
    const params = collectValues($('#custom-params'), { name: 'llama-custom', kind: 'custom' });
    busy($('#btn-custom-load'), true);
    api('/api/custom/load', { method: 'POST', body: { model_file: modelFile, port, params } })
      .then(j => {
        $('#custom-status').textContent = j.msg || '加载成功';
        $('#custom-status').className = 'msg-ok';
        refresh();
      })
      .catch(err => showError(err.message))
      .finally(() => busy($('#btn-custom-load'), false));
  });

  $('#btn-custom-stop').addEventListener('click', () => {
    busy($('#btn-custom-stop'), true);
    api('/api/custom/stop', { method: 'POST' })
      .then(() => {
        $('#custom-status').textContent = '已停止';
        $('#custom-status').className = 'msg-ok';
        refresh();
      })
      .catch(err => showError(err.message))
      .finally(() => busy($('#btn-custom-stop'), false));
  });

  $('#btn-add-model').addEventListener('click', () => {
    const body = {
      name: $('#add-name').value.trim(),
      label: $('#add-label').value.trim(),
      kind: $('#add-kind').value,
      model_file: $('#add-file').value,
      port: Number($('#add-port').value) || 0
    };
    if (!body.name) {
      $('#add-msg').textContent = '请填写模型标识';
      $('#add-msg').className = 'msg-bad';
      return;
    }
    busy($('#btn-add-model'), true);
    api('/api/models/add', { method: 'POST', body })
      .then(j => {
        $('#add-msg').textContent = j.msg;
        $('#add-msg').className = 'msg-ok';
        $('#add-name').value = '';
        $('#add-label').value = '';
        refresh();
        loadModelFiles().catch(() => {});
      })
      .catch(err => {
        $('#add-msg').textContent = err.message;
        $('#add-msg').className = 'msg-bad';
      })
      .finally(() => busy($('#btn-add-model'), false));
  });

  $('#btn-mode-root').addEventListener('click', () => switchMode('root'));
  $('#btn-mode-user').addEventListener('click', () => switchMode('user'));

  $('#btn-ssh-save').addEventListener('click', () => {
    const body = {
      ssh_host: $('#ssh-host').value.trim(),
      ssh_user: $('#ssh-user').value.trim(),
      ssh_port: Number($('#ssh-port').value) || 22,
      ssh_key_path: $('#ssh-keypath').value.trim(),
      scripts_dir: $('#ssh-scripts').value.trim()
    };
    api('/api/panel/ssh', { method: 'POST', body })
      .then(j => {
        $('#ssh-msg').textContent = j.msg + '（' + j.target + '）';
        $('#ssh-msg').className = 'msg-ok';
      })
      .catch(err => { $('#ssh-msg').textContent = err.message; $('#ssh-msg').className = 'msg-bad'; });
  });

  $('#btn-ssh-test').addEventListener('click', () => {
    busy($('#btn-ssh-test'), true);
    $('#ssh-msg').textContent = '测试中…';
    $('#ssh-msg').className = 'dim';
    api('/api/panel/ssh/test', { method: 'POST', body: {} })
      .then(j => {
        $('#ssh-msg').textContent = `${j.msg}${j.raw ? ' · ' + j.raw.replace(/\n/g, ' | ') : ''}`;
        $('#ssh-msg').className = j.ok ? 'msg-ok' : 'msg-bad';
      })
      .catch(err => { $('#ssh-msg').textContent = err.message; $('#ssh-msg').className = 'msg-bad'; })
      .finally(() => busy($('#btn-ssh-test'), false));
  });

  $('#btn-ssh-secret').addEventListener('click', () => {
    const key = $('#ssh-key').value;
    if (!key.trim()) {
      $('#ssh-msg').textContent = '请先粘贴私钥内容';
      $('#ssh-msg').className = 'msg-bad';
      return;
    }
    api('/api/panel/ssh/secret', { method: 'POST', body: { private_key: key } })
      .then(j => {
        $('#ssh-key').value = '';
        $('#ssh-msg').textContent = j.msg;
        $('#ssh-msg').className = 'msg-ok';
        loadSSH().catch(() => {});
      })
      .catch(err => { $('#ssh-msg').textContent = err.message; $('#ssh-msg').className = 'msg-bad'; });
  });

  $('#btn-ssh-secret-clear').addEventListener('click', () => {
    if (!confirm('清除已保存的私钥？之后会回退使用「密钥路径」。')) return;
    api('/api/panel/ssh/secret', { method: 'POST', body: { private_key: '' } })
      .then(j => {
        $('#ssh-msg').textContent = j.msg;
        $('#ssh-msg').className = 'msg-ok';
        loadSSH().catch(() => {});
      })
      .catch(err => { $('#ssh-msg').textContent = err.message; $('#ssh-msg').className = 'msg-bad'; });
  });

  $('#btn-port-save').addEventListener('click', () => {
    const port = Number($('#panel-port').value);
    api('/api/panel/port', { method: 'POST', body: { port } })
      .then(j => {
        $('#port-msg').textContent = `${j.msg} · ${j.note}（原 ${j.old_port} → 新 ${j.new_port}）`;
        $('#port-msg').className = j.old_port === j.new_port ? 'dim' : 'msg-ok';
      })
      .catch(err => { $('#port-msg').textContent = err.message; $('#port-msg').className = 'msg-bad'; });
  });

  $('#btn-panel-restart').addEventListener('click', () => {
    if (!confirm('重启面板服务？页面会短暂失联，约 2~3 秒后自动恢复。')) return;
    $('#port-msg').textContent = '已触发重启，正在等待恢复…';
    $('#port-msg').className = 'dim';
    api('/api/panel/restart', { method: 'POST', body: {} })
      .then(j => { $('#port-msg').textContent = j.msg; $('#port-msg').className = 'msg-ok'; })
      .catch(err => { $('#port-msg').textContent = err.message; $('#port-msg').className = 'msg-bad'; });
  });

  $('#btn-hf-search').addEventListener('click', hfSearch);
  $('#hf-query').addEventListener('keypress', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      hfSearch();
    }
  });

  $('#autostart-toggle').addEventListener('change', () => {
    api('/api/panel/autostart', {
      method: 'POST',
      body: { enabled: $('#autostart-toggle').checked }
    }).catch(() => {
      $('#autostart-toggle').checked = !$('#autostart-toggle').checked;
    });
  });

  $('#auth-toggle').addEventListener('change', () => {
    const enabled = $('#auth-toggle').checked;
    api('/api/panel/auth', { method: 'POST', body: { enabled } })
      .then(j => {
        if (enabled && j.token) {
          $('#auth-msg').textContent = `已启用令牌保护，请保存令牌：${j.token}`;
        } else {
          $('#auth-msg').textContent = '已关闭令牌保护：局域网内任何人打开本页即可操作';
        }
      })
      .catch(() => {
        $('#auth-toggle').checked = !enabled;
      });
  });

  $('#btn-login').addEventListener('click', () => {
    const token = $('#login-token').value.trim();
    if (!token) return;
    doLogin(token);
  });

  $('#login-token').addEventListener('keypress', e => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      $('#btn-login').click();
    }
  });

  $('#btn-cloud').addEventListener('click', () => {
    api('/api/openclaw/mode', { method: 'POST', body: { mode: 'cloud' } })
      .then(j => {
        $('#oc-msg').textContent = `切换到云端模式成功：${j.mode}`;
        $('#oc-msg').className = 'msg-ok';
      })
      .catch(err => showError(err.message));
  });

  $('#btn-local').addEventListener('click', () => {
    api('/api/openclaw/mode', { method: 'POST', body: { mode: 'local' } })
      .then(j => {
        $('#oc-msg').textContent = `切换到本地模式成功：${j.mode}`;
        $('#oc-msg').className = 'msg-ok';
      })
      .catch(err => showError(err.message));
  });

  $('#btn-gw-restart').addEventListener('click', () => {
    if (confirm('重启网关会断开当前会话，确定继续？')) {
      api('/api/openclaw/gateway', { method: 'POST', body: { action: 'restart' } })
        .then(j => {
          $('#oc-msg').textContent = `网关重启成功：${j.msg}`;
          $('#oc-msg').className = 'msg-ok';
        })
        .catch(err => showError(err.message));
    }
  });

  $('#btn-gw-force').addEventListener('click', () => {
    if (confirm('强制重启会立即杀掉网关进程，确定继续？')) {
      api('/api/openclaw/gateway', { method: 'POST', body: { action: 'force' } })
        .then(j => {
          $('#oc-msg').textContent = `网关强制重启成功：${j.msg}`;
          $('#oc-msg').className = 'msg-ok';
        })
        .catch(err => showError(err.message));
    }
  });
}

function doLogin(token) {
  api('/api/login', { method: 'POST', body: { token } })
    .then(() => {
      $('#login-layer').classList.add('hidden');
      $('#login-msg').textContent = '';
      refresh();
    })
    .catch(() => {
      $('#login-msg').textContent = '令牌不正确';
    });
}

init();
const MODELQ = new URLSearchParams(window.location.search).get('model');
if (MODELQ) {
  MODELKEY = MODELQ;
}
setTab((window.location.hash || '#dash').slice(1));
