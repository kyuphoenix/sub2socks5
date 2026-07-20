const statusEl = document.getElementById('socks-status');
const socksListEl = document.getElementById('socks-list');
const addSocksButton = document.getElementById('add-socks');
const clearSocksButton = document.getElementById('clear-socks');

let formPorts = [];
let fullConfig = {};
let latestAvailableOutbounds = [];
let savedPortsSnapshot = [];

async function load() {
  const response = await fetch('/api/config');
  const data = await response.json();
  if (!response.ok) {
    throw new Error(data?.error?.message || '加载配置失败');
  }
  fullConfig = data.config || {};
  latestAvailableOutbounds = data.availableOutbounds || [];
  formPorts = normalizePorts(fullConfig.ports || []);
  await assignMissingSuggestedPorts();
  savedPortsSnapshot = JSON.parse(JSON.stringify(formPorts));
  render();
}

function normalizePorts(ports) {
  if (!Array.isArray(ports) || !ports.length) {
    return [createDefaultPort()];
  }
  return ports.map((item, index) => ({
    tag: item.tag || `socks-${index + 1}`,
    listen: item.listen || '127.0.0.1',
    port: item.port || '',
    target: item.target || 'proxy',
    sniff: true
  }));
}

function createDefaultPort() {
  return {
    tag: `socks-${formPorts.length + 1 || 1}`,
    listen: '127.0.0.1',
    port: '',
    target: fullConfig?.routing?.routeFinal || 'proxy',
    sniff: true
  };
}

function render() {
  const countBadge = document.getElementById('service-count-badge');
  const unsavedBadge = document.getElementById('unsaved-badge');
  const count = formPorts.length;
  if (countBadge) countBadge.textContent = `${count} 个服务`;
  const unsaved = JSON.stringify(formPorts) !== JSON.stringify(savedPortsSnapshot);
  if (unsavedBadge) {
    if (unsaved && count > 0) {
      unsavedBadge.textContent = '有未保存的修改';
      unsavedBadge.style.display = 'inline';
    } else {
      unsavedBadge.style.display = 'none';
    }
  }
  socksListEl.innerHTML = '';
  if (!formPorts.length) {
    socksListEl.innerHTML = '<div class="timeline-item"><div class="title">暂无 SOCKS5 服务</div></div>';
    return;
  }
  for (const [index, portItem] of formPorts.entries()) {
    const item = document.createElement('div');
    item.className = 'timeline-item socks-service-card';
    item.innerHTML = `
      <div class="socks-service-content">
        <div class="socks-service-card-heading">
          <div class="title">SOCKS5 服务 ${index + 1}</div>
          <button type="button" class="copy-service-button" data-copy-port="${index}">复制</button>
        </div>
        <div class="form-grid socks-service-grid">
          <label>
            <span>监听地址</span>
            <input data-port-index="${index}" data-port-field="listen" value="${escapeHtmlAttr(portItem.listen || '127.0.0.1')}" />
          </label>
          <label>
            <span>端口</span>
            <input data-port-index="${index}" data-port-field="port" type="number" min="1" step="1" value="${escapeHtmlAttr(String(portItem.port || ''))}" />
          </label>
          <label class="socks-target-field">
            <span>目标出口</span>
            <select data-port-index="${index}" data-port-field="target">
              ${buildOutboundOptionsHtml(portItem.target)}
            </select>
          </label>
          <div class="socks-remove-action">
            ${formPorts.length > 1 ? `<button type="button" class="danger-button" data-remove-port="${index}">删除</button>` : ''}
          </div>
        </div>
      </div>
    `;
    socksListEl.appendChild(item);
  }
}

function buildOutboundOptionsHtml(selectedTag) {
  const outbounds = latestAvailableOutbounds?.length
    ? latestAvailableOutbounds
    : [{ tag: 'direct', label: 'direct', type: 'direct', source: 'builtin' }];
  return outbounds
    .map((optionInfo) => (
      `<option value="${escapeHtmlAttr(optionInfo.tag)}" ${optionInfo.tag === selectedTag ? 'selected' : ''}>${escapeHtml(optionInfo.label || optionInfo.tag)}</option>`
    ))
    .join('');
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function escapeHtmlAttr(str) {
  return escapeHtml(str).replace(/"/g, '&quot;');
}

function setStatus(message, kind = 'idle') {
  statusEl.textContent = message;
  statusEl.className = `status-bar is-${kind}`;
}

async function resolveNextPort(host, start, exclude = []) {
  const response = await fetch('/api/ports/next', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ host, start, exclude })
  });
  const data = await response.json();
  return Number(data.port || start);
}

async function assignMissingSuggestedPorts() {
  const host = '127.0.0.1';
  const used = new Set(
    formPorts
      .map((item) => Number(item.port))
      .filter((value) => Number.isInteger(value) && value > 0)
  );
  let changed = false;
  for (let index = 0; index < formPorts.length; index += 1) {
    if (Number(formPorts[index].port) > 0) continue;
    const start = index === 0
      ? Number(fullConfig?.app?.port || 18080) + 1
      : Number(formPorts[index - 1].port || 0) + 1;
    const nextPort = await resolveNextPort(host, start, [...used]);
    formPorts[index].port = nextPort;
    used.add(nextPort);
    changed = true;
  }
  return changed;
}

document.getElementById('back-home').addEventListener('click', () => {
  window.location.href = '/';
});

document.getElementById('save-socks').addEventListener('click', async () => {
  try {
    const seenTags = new Set();
    const seenPorts = new Set();
    for (const portItem of formPorts) {
      if (!portItem.tag?.trim()) throw new Error('SOCKS5 服务 tag 不能为空');
      if (seenTags.has(portItem.tag)) throw new Error(`SOCKS5 服务 tag 重复：${portItem.tag}`);
      seenTags.add(portItem.tag);
      if (!portItem.listen?.trim()) throw new Error(`SOCKS5 服务 ${portItem.tag} 监听地址不能为空`);
      const portNum = Number(portItem.port);
      if (!Number.isInteger(portNum) || portNum <= 0) throw new Error(`SOCKS5 服务 ${portItem.tag} 端口无效`);
      if (seenPorts.has(`${portItem.listen}:${portNum}`)) {
        throw new Error(`SOCKS5 服务监听重复：${portItem.listen}:${portNum}`);
      }
      seenPorts.add(`${portItem.listen}:${portNum}`);
      if (!portItem.target) throw new Error(`SOCKS5 服务 ${portItem.tag} 目标出口不能为空`);
    }

    fullConfig.ports = formPorts.map((item, index) => ({
      tag: item.tag?.trim() || `socks-${index + 1}`,
      listen: item.listen?.trim() || '127.0.0.1',
      port: Number(item.port || 0),
      target: item.target || fullConfig?.routing?.routeFinal || 'proxy',
      sniff: true
    }));

    const response = await fetch('/api/config', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(fullConfig)
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data?.error?.message || '保存失败');
    }
    savedPortsSnapshot = JSON.parse(JSON.stringify(formPorts));
    render();
    setStatus('SOCKS5 服务已保存', 'success');
  } catch (error) {
    setStatus(`保存失败：${error.message}`, 'error');
  }
});

addSocksButton.addEventListener('click', async () => {
  formPorts.push(createDefaultPort());
  await assignMissingSuggestedPorts();
  render();
});

clearSocksButton?.addEventListener('click', () => {
  const confirmed = window.confirm('确认删除当前配置的所有 SOCKS5 服务吗？将重置为 1 个默认服务，保存后生效。');
  if (!confirmed) {
    return;
  }
  formPorts = [createDefaultPort()];
  assignMissingSuggestedPorts()
    .then(() => {
      render();
      setStatus('已重置为默认 SOCKS5 服务，请点击“保存配置”以应用', 'idle');
    })
    .catch(() => {
      render();
      setStatus('已重置为默认 SOCKS5 服务，请点击“保存配置”以应用', 'idle');
    });
});

document.addEventListener('input', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLInputElement || target instanceof HTMLSelectElement)) return;
  if (target.dataset.portIndex) {
    const index = Number(target.dataset.portIndex);
    const field = target.dataset.portField;
    formPorts[index][field] = target.value;
  }
});

document.addEventListener('change', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLInputElement || target instanceof HTMLSelectElement)) return;
  if (target.dataset.portIndex) {
    const index = Number(target.dataset.portIndex);
    const field = target.dataset.portField;
    formPorts[index][field] = target.value;
  }
});

document.addEventListener('click', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLElement)) return;
  if (target.dataset.removePort) {
    const index = Number(target.dataset.removePort);
    if (formPorts.length > 1) {
      formPorts.splice(index, 1);
      render();
    }
  }
  if (target.dataset.copyPort) {
    const index = Number(target.dataset.copyPort);
    const p = formPorts[index];
    if (p && p.listen && p.port) {
      navigator.clipboard.writeText(`socks5://${p.listen}:${p.port}`).then(
        () => showToast('复制成功', true),
        () => showToast('复制失败', false)
      );
    }
  }
});

const toastEl = document.getElementById('toast-container');
let toastTimer = null;

function showToast(message, success) {
  if (!toastEl) return;
  clearTimeout(toastTimer);
  toastEl.innerHTML = `${success
    ? '<span style="color:#10b981;font-size:18px">&#10003;</span>'
    : '<span style="color:#ef4444;font-size:18px">&#10007;</span>'
  } ${message}`;
  toastEl.style.display = 'flex';
  toastTimer = setTimeout(() => { toastEl.style.display = 'none'; }, 2500);
}

load()
  .then(() => setStatus('准备就绪', 'idle'))
  .catch((error) => setStatus(`加载失败：${error.message}`, 'error'));
