const statusEl = document.getElementById('socks-status');
const socksListEl = document.getElementById('socks-list');
const addSocksButton = document.getElementById('add-socks');
const addHttpSocksButton = document.getElementById('add-http-socks');
const clearSocksButton = document.getElementById('clear-socks');

let formPorts = [];
let fullConfig = {};
let latestAvailableOutbounds = [];
let savedPortsSnapshot = [];
const expandedPorts = new Set();

async function load() {
  const response = await fetch('/api/config');
  const data = await response.json();
  if (!response.ok) {
    throw new Error(data?.error?.message || '加载配置失败');
  }
  fullConfig = data.config || {};
  latestAvailableOutbounds = data.availableOutbounds || [];
  formPorts = normalizePorts(fullConfig.ports || []);
  for (const [index, p] of formPorts.entries()) {
    if (p.username || p.password) {
      expandedPorts.add(index);
    }
  }
  await assignMissingSuggestedPorts();
  savedPortsSnapshot = JSON.parse(JSON.stringify(formPorts));
  const appVersionEl = document.getElementById('app-version');
  if (appVersionEl && (data.displayVersion || data.appVersion)) {
    appVersionEl.textContent = data.displayVersion || `v${data.appVersion}`;
  }
  render();
}

function normalizePorts(ports) {
  if (!Array.isArray(ports) || !ports.length) {
    return [createDefaultPort('socks')];
  }
  return ports.map((item, index) => {
    const isHttp = item.type === 'http';
    return {
      tag: item.tag || `${isHttp ? 'http' : 'socks'}-${index + 1}`,
      type: isHttp ? 'http' : 'socks',
      listen: item.listen || '127.0.0.1',
      port: item.port || '',
      target: item.target || 'proxy',
      sniff: true,
      username: item.username || '',
      password: item.password || ''
    };
  });
}

function createDefaultPort(type = 'socks') {
  const isHttp = type === 'http';
  const normType = isHttp ? 'http' : 'socks';
  return {
    tag: `${normType}-${formPorts.length + 1 || 1}`,
    type: normType,
    listen: '127.0.0.1',
    port: '',
    target: fullConfig?.routing?.routeFinal || 'proxy',
    sniff: true,
    username: '',
    password: ''
  };
}

function generateRandomString(length = 10) {
  const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  const array = new Uint8Array(length);
  crypto.getRandomValues(array);
  let str = '';
  for (let i = 0; i < length; i += 1) {
    str += chars[array[i] % chars.length];
  }
  return str;
}

function formatProxyUrl(p) {
  const isHttp = p.type === 'http';
  const scheme = isHttp ? 'http' : 'socks5';
  const host = p.listen || '127.0.0.1';
  const port = p.port;
  const username = (p.username || '').trim();
  const password = (p.password || '').trim();
  if (username && password) {
    return `${scheme}://${encodeURIComponent(username)}:${encodeURIComponent(password)}@${host}:${port}`;
  }
  if (username) {
    return `${scheme}://${encodeURIComponent(username)}@${host}:${port}`;
  }
  if (password) {
    return `${scheme}://:${encodeURIComponent(password)}@${host}:${port}`;
  }
  return `${scheme}://${host}:${port}`;
}
const formatSocksUrl = formatProxyUrl;

function updateUnsavedBadge() {
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
}

function render() {
  updateUnsavedBadge();
  socksListEl.innerHTML = '';
  if (!formPorts.length) {
    socksListEl.innerHTML = '<div class="timeline-item"><div class="title">暂无代理服务</div></div>';
    return;
  }
  for (const [index, portItem] of formPorts.entries()) {
    const isExpanded = expandedPorts.has(index);
    const isHttp = portItem.type === 'http';
    const serviceLabel = isHttp ? 'HTTP' : 'SOCKS5';
    const item = document.createElement('div');
    item.className = 'timeline-item socks-service-card';
    item.innerHTML = `
      <div class="socks-service-content">
        <div class="socks-service-card-heading">
          <div class="title">${serviceLabel} 服务 ${index + 1}</div>
          <div class="socks-card-actions">
            <button type="button" class="copy-service-button" data-test-port="${index}">测试</button>
            <button type="button" class="copy-service-button" data-copy-port="${index}">复制</button>
            <button type="button" class="copy-service-button socks-expand-btn" data-toggle-expand="${index}">${isExpanded ? '收起' : '展开'}</button>
          </div>
        </div>
        <div class="form-grid socks-service-grid">
          <label>
            <span>协议类型</span>
            <select data-port-index="${index}" data-port-field="type">
              <option value="socks" ${!isHttp ? 'selected' : ''}>SOCKS5</option>
              <option value="http" ${isHttp ? 'selected' : ''}>HTTP</option>
            </select>
          </label>
          <label class="socks-target-field">
            <span>目标出口</span>
            <select data-port-index="${index}" data-port-field="target">
              ${buildOutboundOptionsHtml(portItem.target)}
            </select>
          </label>
          <label>
            <span>监听地址</span>
            <input data-port-index="${index}" data-port-field="listen" value="${escapeHtmlAttr(portItem.listen || '127.0.0.1')}" />
          </label>
          <label>
            <span>端口</span>
            <input data-port-index="${index}" data-port-field="port" type="number" min="1" step="1" value="${escapeHtmlAttr(String(portItem.port || ''))}" />
          </label>
          <div class="socks-remove-action">
            ${formPorts.length > 1 ? `<button type="button" class="danger-button" data-remove-port="${index}">删除</button>` : ''}
          </div>
        </div>
        <div class="socks-auth-panel ${isExpanded ? '' : 'is-hidden'}" data-auth-panel="${index}">
          <div class="form-grid socks-auth-grid">
            <div class="socks-auth-field">
              <label for="socks-username-${index}">用户名（可选）</label>
              <div class="input-action-group">
                <input id="socks-username-${index}" data-port-index="${index}" data-port-field="username" placeholder="留空无认证" value="${escapeHtmlAttr(portItem.username || '')}" />
                <button type="button" class="input-suffix-button" data-refresh-field="username" data-port-index="${index}" title="生成随机用户名" aria-label="生成随机用户名" draggable="false">
                  <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                    <path d="M21.5 2v6h-6M2.5 22v-6h6M2 11.5a10 10 0 0 1 18.8-4.3M22 12.5a10 10 0 0 1-18.8 4.2"/>
                  </svg>
                </button>
              </div>
            </div>
            <div class="socks-auth-field">
              <label for="socks-password-${index}">密码（可选）</label>
              <div class="input-action-group">
                <input id="socks-password-${index}" data-port-index="${index}" data-port-field="password" placeholder="留空无认证" value="${escapeHtmlAttr(portItem.password || '')}" />
                <button type="button" class="input-suffix-button" data-refresh-field="password" data-port-index="${index}" title="生成随机密码" aria-label="生成随机密码" draggable="false">
                  <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                    <path d="M21.5 2v6h-6M2.5 22v-6h6M2 11.5a10 10 0 0 1 18.8-4.3M22 12.5a10 10 0 0 1-18.8 4.2"/>
                  </svg>
                </button>
              </div>
            </div>
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
      const typeLabel = portItem.type === 'http' ? 'HTTP' : 'SOCKS5';
      if (!portItem.tag?.trim()) throw new Error(`${typeLabel} 服务 tag 不能为空`);
      if (seenTags.has(portItem.tag)) throw new Error(`${typeLabel} 服务 tag 重复：${portItem.tag}`);
      seenTags.add(portItem.tag);
      if (!portItem.listen?.trim()) throw new Error(`${typeLabel} 服务 ${portItem.tag} 监听地址不能为空`);
      const portNum = Number(portItem.port);
      if (!Number.isInteger(portNum) || portNum <= 0) throw new Error(`${typeLabel} 服务 ${portItem.tag} 端口无效`);
      if (seenPorts.has(`${portItem.listen}:${portNum}`)) {
        throw new Error(`代理服务监听重复：${portItem.listen}:${portNum}`);
      }
      seenPorts.add(`${portItem.listen}:${portNum}`);
      if (!portItem.target) throw new Error(`${typeLabel} 服务 ${portItem.tag} 目标出口不能为空`);
    }

    fullConfig.ports = formPorts.map((item, index) => {
      const isHttp = item.type === 'http';
      const portObj = {
        tag: item.tag?.trim() || `${isHttp ? 'http' : 'socks'}-${index + 1}`,
        listen: item.listen?.trim() || '127.0.0.1',
        port: Number(item.port || 0),
        target: item.target || fullConfig?.routing?.routeFinal || 'proxy',
        sniff: true
      };
      if (isHttp) {
        portObj.type = 'http';
      }
      const username = item.username?.trim();
      const password = item.password?.trim();
      if (username) portObj.username = username;
      if (password) portObj.password = password;
      return portObj;
    });

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
    setStatus('代理服务已保存', 'success');
  } catch (error) {
    setStatus(`保存失败：${error.message}`, 'error');
  }
});

addSocksButton?.addEventListener('click', async () => {
  formPorts.push(createDefaultPort('socks'));
  await assignMissingSuggestedPorts();
  render();
});

addHttpSocksButton?.addEventListener('click', async () => {
  formPorts.push(createDefaultPort('http'));
  await assignMissingSuggestedPorts();
  render();
});

clearSocksButton?.addEventListener('click', () => {
  const confirmed = window.confirm('确认删除当前配置的所有代理服务吗？将重置为 1 个默认服务，保存后生效。');
  if (!confirmed) {
    return;
  }
  formPorts = [createDefaultPort('socks')];
  assignMissingSuggestedPorts()
    .then(() => {
      render();
      setStatus('已重置为默认代理服务，请点击“保存配置”以应用', 'idle');
    })
    .catch(() => {
      render();
      setStatus('已重置为默认代理服务，请点击“保存配置”以应用', 'idle');
    });
});

document.addEventListener('input', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLInputElement || target instanceof HTMLSelectElement)) return;
  if (target.dataset.portIndex !== undefined) {
    const index = Number(target.dataset.portIndex);
    const field = target.dataset.portField;
    formPorts[index][field] = target.value;
    updateUnsavedBadge();
  }
});

document.addEventListener('change', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLInputElement || target instanceof HTMLSelectElement)) return;
  if (target.dataset.portIndex !== undefined) {
    const index = Number(target.dataset.portIndex);
    const field = target.dataset.portField;
    formPorts[index][field] = target.value;
    if (field === 'type') {
      render();
      return;
    }
    updateUnsavedBadge();
  }
});

document.addEventListener('dragstart', (event) => {
  if (event.target instanceof Element && event.target.closest('.input-suffix-button')) {
    event.preventDefault();
  }
});

document.addEventListener('click', (event) => {
  const target = event.target;
  if (!(target instanceof Element)) return;

  const toggleExpandBtn = target.closest('[data-toggle-expand]');
  if (toggleExpandBtn) {
    const index = Number(toggleExpandBtn.dataset.toggleExpand);
    if (expandedPorts.has(index)) {
      expandedPorts.delete(index);
    } else {
      expandedPorts.add(index);
    }
    const isExpanded = expandedPorts.has(index);
    const panel = socksListEl.querySelector(`[data-auth-panel="${index}"]`);
    if (panel) {
      panel.classList.toggle('is-hidden', !isExpanded);
    }
    toggleExpandBtn.textContent = isExpanded ? '收起' : '展开';
    return;
  }

  const refreshBtn = target.closest('[data-refresh-field]');
  if (refreshBtn) {
    const index = Number(refreshBtn.dataset.portIndex);
    const field = refreshBtn.dataset.refreshField;
    const randomStr = generateRandomString(10);
    formPorts[index][field] = randomStr;
    const input = socksListEl.querySelector(`input[data-port-index="${index}"][data-port-field="${field}"]`);
    if (input) {
      input.value = randomStr;
    }
    updateUnsavedBadge();
    showToast(`已生成随机${field === 'username' ? '用户名' : '密码'}`, true);
    return;
  }

  if (target.dataset.removePort) {
    const index = Number(target.dataset.removePort);
    if (formPorts.length > 1) {
      formPorts.splice(index, 1);
      expandedPorts.delete(index);
      render();
    }
    return;
  }
  if (target.dataset.testPort) {
    const index = Number(target.dataset.testPort);
    const p = formPorts[index];
    if (p && p.listen && p.port) {
      currentTest = {
        type: p.type === 'http' ? 'http' : 'socks',
        host: p.listen,
        port: Number(p.port),
        username: (p.username || '').trim(),
        password: (p.password || '').trim()
      };
      showSocksTestOverlay();
    } else {
      showToast('请先填写端口再测试', false);
    }
    return;
  }
  if (target.dataset.copyPort) {
    const index = Number(target.dataset.copyPort);
    const p = formPorts[index];
    if (p && p.listen && p.port) {
      navigator.clipboard.writeText(formatProxyUrl(p)).then(
        () => showToast('复制成功', true),
        () => showToast('复制失败', false)
      );
    }
    return;
  }
});

const socksTestOverlay = document.getElementById('socks-test-overlay');
const socksTestStep = document.getElementById('socks-test-step');
const socksTestDetail = document.getElementById('socks-test-detail');
const socksTestSource = document.getElementById('socks-test-source');
const socksTestStart = document.getElementById('socks-test-start');

let currentTest = null; // { type, host, port, username, password }
let testing = false;

function showSocksTestOverlay() {
  if (!socksTestOverlay) return;
  const isHttp = currentTest?.type === 'http';
  const protoLabel = isHttp ? 'HTTP' : 'SOCKS5';
  const titleEl = document.getElementById('socks-test-title');
  if (titleEl) titleEl.textContent = `测试 ${protoLabel} 代理服务`;
  if (socksTestStep) socksTestStep.textContent = '选择测试 API';
  if (socksTestDetail) socksTestDetail.textContent = '';
  if (socksTestStart) socksTestStart.textContent = '开始测试';
  if (socksTestStart) socksTestStart.disabled = false;
  testing = false;
  socksTestOverlay.classList.remove('is-hidden');
  socksTestOverlay.setAttribute('aria-hidden', 'false');
  socksTestSource?.focus();
}

function hideSocksTestOverlay() {
  if (!socksTestOverlay) return;
  if (testing) return;
  socksTestOverlay.classList.add('is-hidden');
  socksTestOverlay.setAttribute('aria-hidden', 'true');
  currentTest = null;
}

async function runSocksTest() {
  if (!currentTest) return;
  testing = true;
  if (socksTestStart) socksTestStart.disabled = true;
  if (socksTestStart) socksTestStart.textContent = '测试中...';
  const { type, host, port, username, password } = currentTest;
  const isHttp = type === 'http';
  const protoName = isHttp ? 'HTTP' : 'SOCKS5';
  const source = socksTestSource?.value || 'api.ipify.org';
  if (socksTestStep) socksTestStep.textContent = '正在查询出口 IP...';
  if (socksTestDetail) socksTestDetail.textContent = `正在通过 ${protoName} 代理 ${host}:${port} 访问 ${source}...`;
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 12000);
    const response = await fetch('/api/socks5/test', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ type: type || 'socks', listen: host, port, username, password, timeoutMs: 8000, source }),
      signal: controller.signal
    });
    clearTimeout(timer);
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data?.error?.message || '测试失败');
    }
    if (socksTestStep) socksTestStep.textContent = `出口 IP：${data.ip}`;
    if (socksTestDetail) socksTestDetail.textContent = `通过 ${protoName} 代理 ${host}:${port} 从 ${source} 查询到 ${data.ip}。`;
  } catch (error) {
    if (socksTestStep) socksTestStep.textContent = '测试失败';
    if (socksTestDetail) socksTestDetail.textContent = `无法通过 ${protoName} 代理 ${host}:${port} 获取出口 IP：${error.message}`;
  } finally {
    if (socksTestStart) socksTestStart.textContent = '重新测试';
    if (socksTestStart) socksTestStart.disabled = false;
    testing = false;
  }
}

socksTestStart?.addEventListener('click', runSocksTest);
document.getElementById('socks-test-close')?.addEventListener('click', hideSocksTestOverlay);
socksTestOverlay?.addEventListener('click', (event) => {
  if (event.target === socksTestOverlay) hideSocksTestOverlay();
});

document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && socksTestOverlay && !socksTestOverlay.classList.contains('is-hidden')) {
    hideSocksTestOverlay();
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
