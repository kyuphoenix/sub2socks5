const editor = document.getElementById('config-editor');
const nodesEl = document.getElementById('nodes');

const generatedEl = document.getElementById('generated');
const kernelEl = document.getElementById('kernel');
const architectureEl = document.getElementById('architecture');
const logsEl = document.getElementById('logs');
const statusBar = document.getElementById('status-bar');
const editorStatus = document.getElementById('editor-status');
const actionButtons = [
  ...document.querySelectorAll('.actions button'),
  ...document.querySelectorAll('.section-heading-actions button')
];
const formView = document.getElementById('form-view');
const jsonView = document.getElementById('json-view');
const switchFormButton = document.getElementById('switch-form');
const switchJsonButton = document.getElementById('switch-json');
const tabButtons = [...document.querySelectorAll('.tab-button[data-tab]')];
const manageNodesButton = document.getElementById('manage-nodes');
const socksServicesEl = document.getElementById('socks-services');
const socksCountEl = document.getElementById('socks-count');
const exportSocksButton = document.getElementById('export-socks');
const copySocksButton = document.getElementById('copy-socks');
const autoConfigureSocksButton = document.getElementById('auto-configure-socks');
const editSocksServiceButton = document.getElementById('edit-socks-service');
const subscriptionUrlsEl = document.getElementById('subscription-urls');
const addSubscriptionUrlButton = document.getElementById('add-subscription-url');
const openSubAutoUpdateButton = document.getElementById('open-sub-auto-update');
const kernelVersionSelect = document.getElementById('kernel-version-select');
const kernelSelectVersionButton = document.getElementById('kernel-select-version');
const kernelArchSelect = document.getElementById('kernel-arch-select');
const kernelCheckUpdatesButton = document.getElementById('kernel-check-updates');
const dnsRemoteUrlWrap = document.getElementById('field-dns-remote-url-wrap');
const dnsBootstrapWrap = document.getElementById('field-dns-bootstrap-wrap');
const socksConfigOverlay = document.getElementById('socks-config-overlay');
const socksConfigStep = document.getElementById('socks-config-step');
const socksConfigProgress = document.getElementById('socks-config-progress');
const socksConfigDetail = document.getElementById('socks-config-detail');
const cancelSocksConfigButton = document.getElementById('cancel-socks-config');
const subAutoUpdateOverlay = document.getElementById('sub-auto-update-overlay');
const subAutoScopeWrap = document.getElementById('sub-auto-scope-wrap');
const subAutoModeWrap = document.getElementById('sub-auto-mode-wrap');
const subUrlZoomOverlay = document.getElementById('sub-url-zoom-overlay');
const subUrlZoomInput = document.getElementById('sub-url-zoom-input');
const saveSubUrlZoomButton = document.getElementById('save-sub-url-zoom');
const cancelSubUrlZoomButton = document.getElementById('cancel-sub-url-zoom');
const copySocksOverlay = document.getElementById('copy-socks-overlay');
const copySocksTabLines = document.getElementById('copy-socks-tab-lines');
const copySocksTabCustom = document.getElementById('copy-socks-tab-custom');
const copySocksLinesPanel = document.getElementById('copy-socks-lines-panel');
const copySocksCustomPanel = document.getElementById('copy-socks-custom-panel');
const copySocksLinesPreview = document.getElementById('copy-socks-lines-preview');
const copySocksCustomSeparator = document.getElementById('copy-socks-custom-separator');
const copySocksCustomPreview = document.getElementById('copy-socks-custom-preview');
const copySocksConfirmButton = document.getElementById('copy-socks-confirm');
const copySocksCancelButton = document.getElementById('copy-socks-cancel');
const subFilterOverlay = document.getElementById('sub-filter-overlay');
const subFilterTabOff = document.getElementById('sub-filter-tab-off');
const subFilterTabBlack = document.getElementById('sub-filter-tab-black');
const subFilterTabWhite = document.getElementById('sub-filter-tab-white');
const subFilterInputsEl = document.getElementById('sub-filter-inputs');
const subFilterAddButton = document.getElementById('sub-filter-add');
const subFilterSaveButton = document.getElementById('sub-filter-save');
const subFilterCancelButton = document.getElementById('sub-filter-cancel');
const subAutoScopeEl = document.getElementById('sub-auto-scope');
const subAutoModeEl = document.getElementById('sub-auto-mode');
const subAutoIntervalWrap = document.getElementById('sub-auto-interval-wrap');
const subAutoIntervalEl = document.getElementById('sub-auto-interval');
const subAutoTimeWrap = document.getElementById('sub-auto-time-wrap');
const subAutoTimeEl = document.getElementById('sub-auto-time');
const subAutoDayWrap = document.getElementById('sub-auto-day-wrap');
const subAutoDayModeEl = document.getElementById('sub-auto-day-mode');
const subAutoTargetsWrap = document.getElementById('sub-auto-targets-wrap');
const subAutoTargetsEl = document.getElementById('sub-auto-targets');
const saveSubAutoUpdateButton = document.getElementById('save-sub-auto-update');
const cancelSubAutoUpdateButton = document.getElementById('cancel-sub-auto-update');

const tabPanels = {
  overview: document.getElementById('tab-overview'),
  logs: document.getElementById('tab-logs')
};

const NODES_UPDATED_KEY = 'sub2socks5:nodes-updated-at';
const DNS_PRESET_URLS = {
  google: 'https://dns.google/dns-query',
  cloudflare: 'https://cloudflare-dns.com/dns-query'
};
const DNS_BOOTSTRAP_PRESETS = ['1.1.1.1', '8.8.8.8', '223.5.5.5'];

const forms = {
  architecture: document.getElementById('architecture-form'),
  kernel: document.getElementById('kernel-form'),
  subscription: document.getElementById('subscription-form'),
  logs: document.getElementById('logs-form')
};

const fields = {
  appHost: document.getElementById('field-app-host'),
  appPort: document.getElementById('field-app-port'),
  appBinary: document.getElementById('field-app-binary'),
  appLogLevel: document.getElementById('field-app-log-level'),
  appAutoStart: document.getElementById('field-app-auto-start'),
  appAutoConfigureSubscription: document.getElementById('field-app-auto-configure-subscription'),
  dnsStrategy: document.getElementById('field-dns-strategy'),
  dnsRemotePreset: document.getElementById('field-dns-remote-preset'),
  dnsRemoteUrl: document.getElementById('field-dns-remote-url'),
  dnsBootstrapPreset: document.getElementById('field-dns-bootstrap-preset'),
  dnsBootstrap: document.getElementById('field-dns-bootstrap'),
  routeFinal: document.getElementById('field-route-final')
};

const infoViews = {
  architecture: setupInfoView('architecture'),
  kernel: setupInfoView('kernel'),
  subscription: setupInfoView('subscription'),
  logs: setupInfoView('logs')
};

let lastSavedConfigText = '';
let currentView = 'form';
let formTouched = false;
let lastKnownNodesUpdatedAt = localStorage.getItem(NODES_UPDATED_KEY) || '';
let latestData = {
  config: null,
  subscription: null,
  availableOutbounds: [],
  runtime: null,
  kernel: null,
  architecture: null,
  plannedKernel: null,
  releaseList: [],
  generated: null,
  logs: null,
  download: null
};
let downloadInFlight = false;
let formPorts = [];
let formSubscriptionUrls = [];
let isFormInteracting = false;
let selectedKernelArch = 'windows-amd64';
let selectedKernelVersion = '';
let kernelArchManuallySelected = false;
let socksConfigAbortController = null;
const expandedSubscriptionInputs = new Set();
let activeSubscriptionAutoConfigIndex = -1;
let activeZoomSubscriptionIndex = -1;
let copySocksMode = 'lines';
let activeSubFilterIndex = -1;
let activeSubFilterMode = 'off';
let activeSubFilterKeywords = [''];

async function load() {
  const [configData, generatedData, logsData, downloadData] = await Promise.all([
    api('/api/config'),
    api('/api/runtime/generated'),
    api('/api/runtime/logs'),
    api('/api/kernel/download')
  ]);

  latestData = {
    config: configData.config,
    subscription: configData.subscription,
    availableOutbounds: configData.availableOutbounds || [],
    runtime: configData.runtime,
    kernel: configData.kernel,
    architecture: configData.architecture || { stored: false, message: '尚未检测架构' },
    plannedKernel: configData.plannedKernel || null,
    releaseList: configData.releaseList || [],
    generated: generatedData,
    logs: logsData,
    download: downloadData
  };

  const formattedConfig = JSON.stringify(configData.config, null, 2);
  const shouldReplaceEditor = !isEditorDirty() || editor.value.trim() === '' || editor.value === lastSavedConfigText;
  if (shouldReplaceEditor) {
    editor.value = formattedConfig;
  }

  if (!isFormInteracting && !formTouched && (!isFormDirty() || shouldReplaceEditor)) {
    fillForm(configData.config);
  }

  lastSavedConfigText = formattedConfig;
  updateEditorState();
  renderOverview();
}

function renderOverview() {
  nodesEl.textContent = JSON.stringify(latestData.subscription, null, 2);
  kernelEl.textContent = JSON.stringify({
    ...latestData.kernel,
    plannedKernel: latestData.plannedKernel,
    releaseListCount: latestData.releaseList.length
  }, null, 2);
  architectureEl.textContent = JSON.stringify(latestData.architecture, null, 2);
  if (generatedEl) generatedEl.textContent = JSON.stringify(latestData.generated, null, 2);
  logsEl.textContent = (latestData.logs?.logs || []).join('\n') || '暂无运行日志';

  const arch = latestData.architecture || {};
  renderKeyValue(forms.architecture, chineseLabels(flattenObject({
    stored: Boolean(arch.detectedAt),
    platform: arch.platform || '',
    arch: arch.arch || '',
    executableName: arch.executableName || '',
    assetSuffix: arch.assetSuffix || '',
    plannedVersion: latestData.plannedKernel?.version || ''
  })));
  const kernelBadge = document.getElementById('kernel-status-badge');
  if (kernelBadge) {
    const isRunning = latestData.runtime?.running;
    const isInstalled = latestData.kernel?.installed;
    if (isRunning) {
      kernelBadge.textContent = '运行中';
      kernelBadge.className = 'editor-status is-saved';
    } else if (isInstalled) {
      kernelBadge.textContent = '未运行';
      kernelBadge.className = 'editor-status is-invalid';
    } else {
      kernelBadge.textContent = '未下载';
      kernelBadge.className = 'editor-status is-idle';
    }
  }
  renderKeyValue(forms.kernel, chineseLabels({
    installed: latestData.kernel?.installed,
    binaryPath: latestData.kernel?.binaryPath,
    installedVersion: latestData.kernel?.releaseInfo?.version || latestData.kernel?.releaseInfo?.tag_name || '',
    plannedVersion: latestData.plannedKernel?.version || '',
    plannedAsset: latestData.plannedKernel?.assetName || ''
  }));
  renderKeyValue(forms.subscription, chineseLabels(buildSubscriptionSummary(latestData.subscription)));
  if (forms.generated) renderKeyValue(forms.generated, buildGeneratedSummary(latestData.generated));
  renderLogTimeline(forms.logs, latestData.logs?.logs || []);
  renderArchitectureSelector();
  renderKernelVersionOptions();
  renderRouteFinalOptions();
  renderDnsPresetUi();
  if ((!formTouched && !isFormInteracting) || currentView !== 'form') {
    renderSubscriptionUrls();
    renderSocksServices();
  }
}

function renderArchitectureSelector() {
  const detected = latestData.architecture?.assetSuffix;
  if (detected && !kernelArchManuallySelected) {
    selectedKernelArch = detected;
  }
  kernelArchSelect.value = selectedKernelArch || detected || 'windows-amd64';
}

function renderKernelVersionOptions() {
  const releases = latestData.releaseList || [];
  const selectedVersion = selectedKernelVersion || latestData.plannedKernel?.version || '';
  kernelVersionSelect.innerHTML = '';

  if (!releases.length) {
    const option = document.createElement('option');
    option.value = '';
    option.textContent = '请先检测架构';
    kernelVersionSelect.appendChild(option);
    kernelVersionSelect.disabled = true;
    kernelSelectVersionButton.disabled = true;
    return;
  }

  for (const release of releases) {
    const option = document.createElement('option');
    option.value = release.version;
    option.textContent = release.version;
    option.selected = release.version === selectedVersion;
    kernelVersionSelect.appendChild(option);
  }

  if (!kernelVersionSelect.value && releases[0]) {
    kernelVersionSelect.value = releases[0].version;
  }
  selectedKernelVersion = kernelVersionSelect.value;

  kernelVersionSelect.disabled = false;
  kernelSelectVersionButton.disabled = false;
}

function renderRouteFinalOptions() {
  const selectedTag = latestData.config?.routing?.routeFinal || fields.routeFinal.value || 'proxy';
  fields.routeFinal.innerHTML = buildOutboundOptionsHtml(selectedTag);
  fields.routeFinal.value = selectedTag;
}

function renderDnsPresetUi() {
  const preset = fields.dnsRemotePreset.value || 'cloudflare';
  const custom = preset === 'custom';
  dnsRemoteUrlWrap.classList.toggle('is-hidden', !custom);
  fields.dnsRemoteUrl.disabled = !custom;

  const bootstrapPreset = fields.dnsBootstrapPreset.value || '1.1.1.1';
  const bootstrapCustom = bootstrapPreset === 'custom';
  dnsBootstrapWrap.classList.toggle('is-hidden', !bootstrapCustom);
  fields.dnsBootstrap.disabled = !bootstrapCustom;
}

function renderSubscriptionUrls() {
  subscriptionUrlsEl.innerHTML = '';
  if (!formSubscriptionUrls.length) {
    subscriptionUrlsEl.innerHTML = '<div class="timeline-item"><div class="title">暂无订阅地址</div></div>';
    return;
  }

  for (const [index, item] of formSubscriptionUrls.entries()) {
    const expanded = expandedSubscriptionInputs.has(index);
    const isIndependent = (subAutoScopeEl?.value || 'off') === 'independent';
    const rowClass = isIndependent ? 'sub-url-row has-side-action' : 'sub-url-row';
    const inputClass = expanded ? 'sub-url-input is-expanded' : 'sub-url-input';
    const block = document.createElement('div');
    block.className = 'timeline-item';
    block.innerHTML = `
      <div class="title">订阅地址 ${index + 1}</div>
      <div class="form-grid">
        <label class="sub-url-label">
          <span>URL</span>
          <div class="${rowClass}">
            <div class="sub-url-input-wrap ${expanded ? 'is-expanded' : ''}">
              <input class="${inputClass}" data-subscription-index="${index}" data-subscription-field="url" value="${escapeHtmlAttr(item.url || '')}" />
              <button type="button" class="sub-url-zoom" data-zoom-subscription="${index}" title="放大编辑">⤢</button>
            </div>
            <button type="button" data-open-sub-filter="${index}">过滤</button>
            ${isIndependent ? `<button type="button" data-open-sub-auto-update-item="${index}">自动更新</button>` : ''}
          </div>
        </label>
      </div>
      <div class="section-heading-actions">
        ${formSubscriptionUrls.length > 1 ? `<button type="button" data-remove-subscription="${index}">删除</button>` : ''}
      </div>
    `;
    subscriptionUrlsEl.appendChild(block);
  }
}

function renderSocksServices() {
  const count = formPorts.length;
  if (socksCountEl) socksCountEl.textContent = `${count} 个服务`;
  if (!socksServicesEl) return;
  socksServicesEl.innerHTML = '';

  for (const [index, item] of formPorts.entries()) {
    const block = document.createElement('div');
    block.className = 'timeline-item';
    block.innerHTML = `
      <div class="title">SOCKS5 服务 ${index + 1}</div>
      <div class="form-grid">
        <label><span>tag</span><input data-port-index="${index}" data-port-field="tag" value="${escapeHtmlAttr(item.tag || '')}" /></label>
        <label><span>监听地址</span><input data-port-index="${index}" data-port-field="listen" value="${escapeHtmlAttr(item.listen || '127.0.0.1')}" /></label>
        <label><span>端口</span><input data-port-index="${index}" data-port-field="port" type="number" min="1" step="1" value="${escapeHtmlAttr(item.port || '')}" /></label>
        <label><span>目标出口</span><select data-port-index="${index}" data-port-field="target">${buildOutboundOptionsHtml(item.target || 'proxy')}</select></label>
      </div>
      <div class="section-heading-actions">
        <button type="button" data-suggest-port="${index}">推荐端口</button>
        ${formPorts.length > 1 ? `<button type="button" data-remove-port="${index}">删除</button>` : ''}
      </div>
    `;
    socksServicesEl.appendChild(block);
  }

  const actions = document.createElement('div');
  actions.className = 'section-heading-actions';
  actions.innerHTML = '<button type="button" id="add-socks-service">+ 添加 SOCKS5 服务</button>';
  socksServicesEl.appendChild(actions);
}

function buildOutboundOptionsHtml(selectedTag) {
  const outbounds = latestData.availableOutbounds?.length
    ? latestData.availableOutbounds
    : [{ tag: 'direct', label: 'direct', type: 'direct', source: 'builtin' }];

  return outbounds
    .map((optionInfo) => (
      `<option value="${escapeHtmlAttr(optionInfo.tag)}" ${optionInfo.tag === selectedTag ? 'selected' : ''}>${escapeHtml(optionInfo.label || optionInfo.tag)}</option>`
    ))
    .join('');
}

async function post(path, body) {
  const response = await fetch(path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: body ? JSON.stringify(body) : '{}'
  });
  const data = await readResponseJson(response);
  if (!response.ok) {
    throw new Error(extractErrorMessage(data, response));
  }
  return data;
}

async function api(path) {
  const response = await fetch(path);
  const data = await readResponseJson(response);
  if (!response.ok) {
    throw new Error(extractErrorMessage(data, response));
  }
  return data;
}

async function readResponseJson(response) {
  const text = await response.text();
  if (!text) return {};
  try {
    return JSON.parse(text);
  } catch {
    return { raw: text };
  }
}

function extractErrorMessage(data, response) {
  if (data?.error?.message) return data.error.message;
  if (typeof data?.error === 'string') return data.error;
  if (typeof data?.raw === 'string' && data.raw.trim()) return data.raw;
  return `Request failed: ${response.status}`;
}

function setStatus(message, kind = 'idle') {
  statusBar.textContent = message;
  statusBar.className = `status-bar is-${kind}`;
}

function syncStatusBarWithDownload(downloadState = {}) {
  if (downloadState?.active !== true) return false;
  const progress = downloadState.progress || {};
  const stage = progress.stage || 'download';
  const message = progress.message || '正在下载';
  const percent = typeof progress.percent === 'number' ? `${progress.percent.toFixed(0)}%` : '处理中';
  const threads = progress.threads ? `，${progress.threads} 线程` : '';
  setStatus(`下载内核中 [${stage}] ${message}，${percent}${threads}`, 'loading');
  return true;
}

function setBusy(isBusy) {
  if (downloadInFlight && isBusy) return;
  for (const button of actionButtons) {
    if (button.id === 'kernel-download' && downloadInFlight) {
      button.disabled = true;
      continue;
    }
    button.disabled = isBusy;
  }
}

function updateEditorState() {
  const validation = parseConfigFromCurrentView();
  if (!validation.ok) {
    editor.classList.add('is-invalid');
    editorStatus.textContent = `配置格式无效：${validation.error}`;
    editorStatus.className = 'editor-status is-invalid';
    return;
  }

  editor.classList.remove('is-invalid');
  const saved = latestData.config || safeParseJson(lastSavedConfigText) || {};
  if (deepEqual(validation.value, saved)) {
    editorStatus.textContent = '配置已保存';
    editorStatus.className = 'editor-status is-saved';
    return;
  }

  editorStatus.textContent = '有未保存修改';
  editorStatus.className = 'editor-status is-dirty';
}

function parseConfigFromCurrentView() {
  return currentView === 'json' ? parseJsonEditor() : parseFormConfig(false);
}

async function postConfigWithoutRuntimeRestart(config) {
  const response = await fetch('/api/config', {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      'x-skip-runtime-restart': '1'
    },
    body: JSON.stringify(config || {})
  });
  const data = await readResponseJson(response);
  if (!response.ok) {
    throw new Error(extractErrorMessage(data, response));
  }
  return data;
}

function safeParseJson(text) {
  try {
    return JSON.parse(text || '{}');
  } catch {
    return null;
  }
}

function deepEqual(a, b) {
  if (a === b) return true;
  if (typeof a !== typeof b) return false;
  if (a == null || b == null) return a === b;
  if (Array.isArray(a)) {
    if (!Array.isArray(b) || a.length !== b.length) return false;
    for (let i = 0; i < a.length; i += 1) {
      if (!deepEqual(a[i], b[i])) return false;
    }
    return true;
  }
  if (typeof a === 'object') {
    const aKeys = Object.keys(a).sort();
    const bKeys = Object.keys(b).sort();
    if (aKeys.length !== bKeys.length) return false;
    for (let i = 0; i < aKeys.length; i += 1) {
      if (aKeys[i] !== bKeys[i]) return false;
      if (!deepEqual(a[aKeys[i]], b[bKeys[i]])) return false;
    }
    return true;
  }
  return a === b;
}

function parseJsonEditor() {
  try {
    const value = JSON.parse(editor.value || '{}');
    return { ok: true, value, text: JSON.stringify(value, null, 2) };
  } catch (error) {
    return { ok: false, error: error.message };
  }
}

function parseFormConfig(validateRequired = false) {
  try {
    const parsedJson = parseJsonEditor();
    const base = parsedJson.ok ? parsedJson.value : JSON.parse(lastSavedConfigText || '{}');
    const next = structuredClone(base);

    next.subscription ||= {};
    next.app ||= {};
    next.dns ||= {};
    next.routing ||= {};
    next.nodeRegistry ||= { manualNodes: [], groups: [] };

    next.subscription.urls = formSubscriptionUrls.map((item) => item.url.trim()).filter(Boolean);
    next.subscription.url = next.subscription.urls[0] || '';
    next.subscription.format = next.subscription.format || 'raw';
    next.subscription.autoUpdate = next.subscription.autoUpdate || {};
    const scope = subAutoScopeEl?.value || 'off';
    next.subscription.autoUpdate.scope = scope;
    if (scope === 'simultaneous') {
      next.subscription.autoUpdate.mode = subAutoModeEl?.value || 'interval';
      next.subscription.autoUpdate.intervalMinutes = Number(subAutoIntervalEl?.value || 60);
      next.subscription.autoUpdate.time = subAutoTimeEl?.value || '03:00';
      next.subscription.autoUpdate.dayMode = subAutoDayModeEl?.value || 'daily';
      next.subscription.autoUpdate.targets = collectSubAutoTargets();
    } else if (scope === 'independent') {
      delete next.subscription.autoUpdate.mode;
      delete next.subscription.autoUpdate.intervalMinutes;
      delete next.subscription.autoUpdate.time;
      delete next.subscription.autoUpdate.dayMode;
      delete next.subscription.autoUpdate.targets;
    } else {
      delete next.subscription.autoUpdate.mode;
      delete next.subscription.autoUpdate.intervalMinutes;
      delete next.subscription.autoUpdate.time;
      delete next.subscription.autoUpdate.dayMode;
      delete next.subscription.autoUpdate.targets;
      next.subscription.autoUpdate.items = [];
    }
    next.subscription.autoUpdate.items = next.subscription.autoUpdate.items || [];
    next.subscription.autoUpdate = next.subscription.autoUpdate || {};
    next.subscription.autoUpdate.mode = subAutoModeEl?.value || 'interval';
    next.subscription.autoUpdate.intervalMinutes = Number(subAutoIntervalEl?.value || 60);
    next.subscription.autoUpdate.time = subAutoTimeEl?.value || '03:00';
    next.app.host = fields.appHost.value.trim();
    next.app.port = Number(fields.appPort.value || 0);
    next.app.singBoxBinary = fields.appBinary.value.trim();
    next.app.logLevel = fields.appLogLevel.value;
    next.app.autoStart = fields.appAutoStart.value === 'true';
    next.app.autoConfigureOnSubscription = fields.appAutoConfigureSubscription.value === 'true';
    next.dns.strategy = fields.dnsStrategy.value;
    next.dns.remotePreset = fields.dnsRemotePreset.value;
    next.dns.remoteUrl = fields.dnsRemotePreset.value === 'custom'
      ? fields.dnsRemoteUrl.value.trim()
      : DNS_PRESET_URLS[fields.dnsRemotePreset.value] || DNS_PRESET_URLS.cloudflare;
    next.dns.bootstrapServer = fields.dnsBootstrapPreset.value === 'custom'
      ? fields.dnsBootstrap.value.trim()
      : fields.dnsBootstrapPreset.value;
    next.routing.routeFinal = fields.routeFinal.value || next.routing.routeFinal || 'proxy';

    next.ports = formPorts.map((item, index) => ({
      tag: item.tag?.trim() || `socks-${index + 1}`,
      listen: item.listen?.trim() || '127.0.0.1',
      port: Number(item.port || 0),
      target: item.target || next.routing.routeFinal || 'proxy',
      sniff: true
    }));

    if (validateRequired) {
      if (!next.app.host) throw new Error('Web UI 监听地址不能为空');
      if (!Number.isInteger(next.app.port) || next.app.port <= 0) throw new Error('Web UI 端口无效');
      if (!next.app.singBoxBinary) throw new Error('sing-box 二进制路径不能为空');
      if (!next.routing.routeFinal) throw new Error('默认路由出口不能为空');
      if (!next.ports.length) throw new Error('至少需要一个 SOCKS5 服务');
      if (!next.dns.remoteUrl) throw new Error('DoH 地址不能为空');

      const seenSubUrls = new Set();
      for (const url of next.subscription.urls) {
        if (seenSubUrls.has(url)) throw new Error(`订阅地址重复：${url}`);
        seenSubUrls.add(url);
      }

      const seenTags = new Set();
      const seenPorts = new Set();
      for (const portItem of next.ports) {
        if (!portItem.tag) throw new Error('SOCKS5 服务 tag 不能为空');
        if (seenTags.has(portItem.tag)) throw new Error(`SOCKS5 服务 tag 重复：${portItem.tag}`);
        seenTags.add(portItem.tag);

        if (!portItem.listen) throw new Error(`SOCKS5 服务 ${portItem.tag} 监听地址不能为空`);
        if (!Number.isInteger(portItem.port) || portItem.port <= 0) throw new Error(`SOCKS5 服务 ${portItem.tag} 端口无效`);
        if (seenPorts.has(`${portItem.listen}:${portItem.port}`)) {
          throw new Error(`SOCKS5 服务监听重复：${portItem.listen}:${portItem.port}`);
        }
        seenPorts.add(`${portItem.listen}:${portItem.port}`);

        if (!portItem.target) throw new Error(`SOCKS5 服务 ${portItem.tag} 目标出口不能为空`);
      }
    }

    return { ok: true, value: next, text: JSON.stringify(next, null, 2) };
  } catch (error) {
    return { ok: false, error: error.message };
  }
}

function fillForm(config) {
  const urls = Array.isArray(config.subscription?.urls) && config.subscription.urls.length
    ? config.subscription.urls
    : (config.subscription?.url ? [config.subscription.url] : ['']);

  formSubscriptionUrls = urls.map((url) => ({ url }));
  expandedSubscriptionInputs.clear();
  fields.appHost.value = config.app?.host || '0.0.0.0';
  fields.appPort.value = config.app?.port || 18080;
  fields.appBinary.value = config.app?.singBoxBinary || '';
  fields.appLogLevel.value = config.app?.logLevel || 'info';
  fields.appAutoStart.value = config.app?.autoStart ? 'true' : 'false';
  fields.appAutoConfigureSubscription.value = config.app?.autoConfigureOnSubscription ? 'true' : 'false';
  fields.dnsStrategy.value = config.dns?.strategy || 'prefer_ipv4';
  fields.dnsRemotePreset.value = config.dns?.remotePreset || inferDnsPreset(config.dns?.remoteUrl);
  fields.dnsRemoteUrl.value = config.dns?.remoteUrl || DNS_PRESET_URLS.cloudflare;
  fields.dnsBootstrapPreset.value = inferBootstrapPreset(config.dns?.bootstrapServer);
  fields.dnsBootstrap.value = config.dns?.bootstrapServer || '1.1.1.1';
  formPorts = normalizePorts(config.ports || []);
  renderRouteFinalOptions();
  fields.routeFinal.value = config.routing?.routeFinal || 'proxy';
  subAutoScopeEl.value = config.subscription?.autoUpdate?.scope || 'off';
  subAutoModeEl.value = config.subscription?.autoUpdate?.mode || 'interval';
  subAutoIntervalEl.value = config.subscription?.autoUpdate?.intervalMinutes || 60;
  subAutoTimeEl.value = config.subscription?.autoUpdate?.time || '03:00';
  subAutoDayModeEl.value = config.subscription?.autoUpdate?.dayMode || 'daily';
  renderSubAutoUpdateMode();
  renderDnsPresetUi();
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
    target: fields.routeFinal?.value || latestData.config?.routing?.routeFinal || 'proxy',
    sniff: true
  };
}

async function resolveNextPort(host, start, exclude = []) {
  const data = await post('/api/ports/next', { host, start, exclude });
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
      ? Number(fields.appPort.value || 18080) + 1
      : Number(formPorts[index - 1].port || 0) + 1;
    const nextPort = await resolveNextPort(host, start, [...used]);
    formPorts[index].port = nextPort;
    used.add(nextPort);
    changed = true;
  }

  return changed;
}

async function autoConfigureSocksServicesFromOutbounds() {
  socksConfigAbortController = new AbortController();
  showSocksConfigOverlay('准备阶段', 0, 0, '正在收集可用节点...');
  const outbounds = (latestData.availableOutbounds || [])
    .filter((item) => item && item.tag && !['proxy', 'auto', 'block', 'direct'].includes(item.tag));
  if (!outbounds.length) {
    hideSocksConfigOverlay();
    socksConfigAbortController = null;
    throw new Error('当前没有可用节点可用于配置 SOCKS5 服务');
  }

  const runtimeWasRunning = Boolean(latestData.runtime?.running);
  let connectivity = {};
  try {
    if (!runtimeWasRunning) {
      updateSocksConfigOverlay('启动内核', 0, outbounds.length, '正在启动 sing-box...');
      await post('/api/runtime/start');
      await waitForRuntimeReady();
    }
    updateSocksConfigOverlay('节点延迟测试', 0, outbounds.length, '正在批量测试节点延迟...');
    connectivity = await checkOutboundsConnectivity(outbounds.map((item) => item.tag));
  } catch (error) {
    hideSocksConfigOverlay();
    socksConfigAbortController = null;
    throw new Error(`延迟测试前准备失败：${error.message}`);
  } finally {
    if (!runtimeWasRunning) {
      await post('/api/runtime/stop');
    }
  }
  const passedOutbounds = outbounds.filter((item) => connectivity[item.tag]?.ok);
  if (!passedOutbounds.length) {
    hideSocksConfigOverlay();
    socksConfigAbortController = null;
    throw new Error('没有通过延迟测试的节点，未创建 SOCKS5 服务');
  }

  updateSocksConfigOverlay('出口识别', 0, passedOutbounds.length, '正在识别节点出口 IP...');
  const egressResults = await checkOutboundsEgressIP(passedOutbounds.map((item) => item.tag));
  const ipBuckets = new Map();
  for (const item of passedOutbounds) {
    const ip = egressResults[item.tag]?.egressIP;
    if (!ip) continue;
    if (!ipBuckets.has(ip)) ipBuckets.set(ip, []);
    ipBuckets.get(ip).push(item.tag);
  }

  const existingGroups = Array.isArray(latestData?.config?.nodeRegistry?.groups)
    ? latestData.config.nodeRegistry.groups.slice()
    : [];
  const generatedGroups = [];
  const urltestGroupTag = 'ALL-PASSED-URLTEST';
  generatedGroups.push({
    tag: urltestGroupTag,
    strategy: 'urltest',
    url: 'https://www.gstatic.com/generate_204',
    interval: '10m',
    timeoutMs: 5000,
    members: passedOutbounds.map((item) => item.tag)
  });
  const groupedNodeTags = new Set();
  for (const [ip, members] of ipBuckets.entries()) {
    if (members.length < 2) continue;
    const countryCode = extractCountryCodeFromTag(members[0]);
    const tag = `${countryCode}-${ip}`;
    generatedGroups.push({
      tag,
      strategy: 'urltest',
      url: 'https://www.gstatic.com/generate_204',
      interval: '10m',
      timeoutMs: 5000,
      members: [...new Set(members)]
    });
    members.forEach((nodeTag) => groupedNodeTags.add(nodeTag));
  }
  const mergedGroups = mergeGeneratedGroups(existingGroups, generatedGroups);
  latestData.config.nodeRegistry ||= {};
  latestData.config.nodeRegistry.groups = mergedGroups;

  updateSocksConfigOverlay('生成服务', passedOutbounds.length, outbounds.length, '正在为延迟测试通过的节点分配端口...');
  const host = '127.0.0.1';
  const used = new Set();
  const appPort = Number(fields.appPort.value || latestData.config?.app?.port || 18080);
  let nextStart = appPort + 1;
  const generated = [];

  const targetsForServices = [
    { tag: urltestGroupTag, isGroup: true },
    ...generatedGroups.map((group) => ({ tag: group.tag, isGroup: true })),
    ...passedOutbounds.filter((item) => !groupedNodeTags.has(item.tag)).map((item) => ({ tag: item.tag, isGroup: false }))
  ];
  const dedupTargets = [];
  const seenTargetTags = new Set();
  for (const target of targetsForServices) {
    if (seenTargetTags.has(target.tag)) continue;
    seenTargetTags.add(target.tag);
    dedupTargets.push(target);
  }

  for (const [idx, item] of dedupTargets.entries()) {
    ensureSocksConfigNotCancelled();
    const safeTag = String(item.tag).replace(/[^a-zA-Z0-9_.-]/g, '-');
    let port;
    if (idx === 0) {
      port = await resolveNextPort(host, 18081, [...used]);
    } else {
      port = await resolveNextPort(host, nextStart, [...used]);
    }
    used.add(port);
    nextStart = port + 1;
    generated.push({
      tag: `socks-${safeTag}`,
      listen: host,
      port,
      target: item.tag,
      sniff: true
    });
  }

  formPorts = generated;
  latestData.config.ports = generated.map((item) => ({ ...item }));
  if (currentView === 'json') {
    const parsed = parseJsonEditor();
    if (parsed.ok) {
      const next = parsed.value;
      next.nodeRegistry ||= {};
      next.nodeRegistry.groups = mergedGroups;
      editor.value = JSON.stringify(next, null, 2);
    }
  }
  latestData.config.nodeRegistry.groups = mergedGroups;
  editor.value = JSON.stringify(latestData.config, null, 2);
  fillForm(latestData.config);
  renderSocksServices();
  formTouched = true;
  updateEditorState();
  updateSocksConfigOverlay('完成', dedupTargets.length, outbounds.length, `已生成 ${dedupTargets.length} 个 SOCKS5 服务，自动创建 ${generatedGroups.length} 个节点组`);
  await sleep(350);
  hideSocksConfigOverlay();
  socksConfigAbortController = null;
}

async function checkOutboundsConnectivity(tags) {
  const results = {};
  let completed = 0;
  let cursor = 0;
  const workerCount = Math.min(5, tags.length);
  const runOne = async () => {
    while (true) {
      ensureSocksConfigNotCancelled();
      const current = cursor;
      cursor += 1;
      if (current >= tags.length) return;
      const tag = tags[current];
      updateSocksConfigOverlay('节点延迟测试', completed, tags.length, `正在测试节点：${tag}`);
      try {
        const response = await fetch('/api/nodes/check', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ tags: [tag] }),
          signal: socksConfigAbortController?.signal
        });
        const data = await readResponseJson(response);
        if (!response.ok) {
          throw new Error(extractErrorMessage(data, response));
        }
        results[tag] = data.results?.[tag] || { ok: false, error: '延迟测试失败' };
      } catch (error) {
        if (error?.name === 'AbortError') {
          throw new Error('用户取消了操作');
        }
        results[tag] = { ok: false, error: error.message };
      } finally {
        completed += 1;
        updateSocksConfigOverlay('节点延迟测试', completed, tags.length, `已完成 ${completed}/${tags.length}`);
      }
    }
  };
  await Promise.all(Array.from({ length: workerCount }, () => runOne()));
  return results;
}

async function checkOutboundsEgressIP(tags) {
  const response = await fetch('/api/nodes/egress', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ tags, timeoutMs: 6000 }),
    signal: socksConfigAbortController?.signal
  });
  const data = await readResponseJson(response);
  if (!response.ok) {
    throw new Error(extractErrorMessage(data, response));
  }
  return data.results || {};
}

function extractCountryCodeFromTag(tag) {
  const text = String(tag || '').toUpperCase();
  const match = text.match(/([A-Z]{2})/);
  return match?.[1] || 'UN';
}

function mergeGeneratedGroups(existingGroups, generatedGroups) {
  const output = [];
  const existingByTag = new Map();
  for (const group of existingGroups || []) {
    if (group?.tag) existingByTag.set(group.tag, group);
    output.push(group);
  }
  for (const group of generatedGroups || []) {
    const previous = existingByTag.get(group.tag);
    if (previous) {
      previous.strategy = 'urltest';
      previous.url = group.url;
      previous.interval = group.interval;
      previous.timeoutMs = group.timeoutMs;
      previous.members = group.members;
    } else {
      output.push(group);
    }
  }
  return output;
}

function showSocksConfigOverlay(step, current, total, detail) {
  if (!socksConfigOverlay) return;
  updateSocksConfigOverlay(step, current, total, detail);
  socksConfigOverlay.classList.remove('is-hidden');
  socksConfigOverlay.setAttribute('aria-hidden', 'false');
}

function updateSocksConfigOverlay(step, current, total, detail) {
  if (socksConfigStep) socksConfigStep.textContent = `步骤：${step}`;
  if (socksConfigProgress) socksConfigProgress.textContent = `${current} / ${total}`;
  if (socksConfigDetail) socksConfigDetail.textContent = detail || '';
}

function hideSocksConfigOverlay() {
  if (!socksConfigOverlay) return;
  socksConfigOverlay.classList.add('is-hidden');
  socksConfigOverlay.setAttribute('aria-hidden', 'true');
}

function showSubAutoUpdateOverlay() {
  if (!subAutoUpdateOverlay) return;
  renderSubAutoUpdateMode();
  subAutoUpdateOverlay.classList.remove('is-hidden');
  subAutoUpdateOverlay.setAttribute('aria-hidden', 'false');
}

function showSubUrlZoomOverlay(index) {
  activeZoomSubscriptionIndex = index;
  subUrlZoomInput.value = formSubscriptionUrls[index]?.url || '';
  subUrlZoomOverlay.classList.remove('is-hidden');
  subUrlZoomOverlay.setAttribute('aria-hidden', 'false');
}

function hideSubUrlZoomOverlay() {
  subUrlZoomOverlay.classList.add('is-hidden');
  subUrlZoomOverlay.setAttribute('aria-hidden', 'true');
  activeZoomSubscriptionIndex = -1;
}

function showCopySocksOverlay() {
  if (!copySocksOverlay) return;
  copySocksMode = 'lines';
  updateCopySocksPreviews();
  renderCopySocksTabs();
  copySocksOverlay.classList.remove('is-hidden');
  copySocksOverlay.setAttribute('aria-hidden', 'false');
}

function hideCopySocksOverlay() {
  if (!copySocksOverlay) return;
  copySocksOverlay.classList.add('is-hidden');
  copySocksOverlay.setAttribute('aria-hidden', 'true');
}

function renderCopySocksTabs() {
  const isLines = copySocksMode === 'lines';
  copySocksTabLines?.classList.toggle('is-active', isLines);
  copySocksTabCustom?.classList.toggle('is-active', !isLines);
  copySocksLinesPanel?.classList.toggle('is-hidden', !isLines);
  copySocksLinesPanel?.classList.toggle('is-active', isLines);
  copySocksCustomPanel?.classList.toggle('is-hidden', isLines);
  copySocksCustomPanel?.classList.toggle('is-active', !isLines);
}

function buildSocksAddressList() {
  return formPorts
    .filter((p) => p.listen && p.port)
    .map((p) => `socks5://${p.listen}:${p.port}`);
}

function updateCopySocksPreviews() {
  const list = buildSocksAddressList();
  const sep = copySocksCustomSeparator?.value ?? ',';
  if (copySocksLinesPreview) {
    copySocksLinesPreview.value = list.join('\n');
  }
  if (copySocksCustomPreview) {
    copySocksCustomPreview.value = list.join(sep);
  }
}

function showSubFilterOverlay(index) {
  activeSubFilterIndex = index;
  const parsed = parseFormConfig(false);
  const cfg = parsed.ok ? parsed.value : (latestData.config || {});
  const filters = Array.isArray(cfg.subscription?.filters) ? cfg.subscription.filters : [];
  const current = filters[index] || {};
  activeSubFilterMode = String(current.mode || 'off');
  activeSubFilterKeywords = Array.isArray(current.keywords) && current.keywords.length ? current.keywords.map((v) => String(v)) : [''];
  renderSubFilterTabs();
  renderSubFilterInputs();
  subFilterOverlay?.classList.remove('is-hidden');
  subFilterOverlay?.setAttribute('aria-hidden', 'false');
}

function hideSubFilterOverlay() {
  subFilterOverlay?.classList.add('is-hidden');
  subFilterOverlay?.setAttribute('aria-hidden', 'true');
  activeSubFilterIndex = -1;
}

function renderSubFilterTabs() {
  subFilterTabOff?.classList.toggle('is-active', activeSubFilterMode === 'off');
  subFilterTabBlack?.classList.toggle('is-active', activeSubFilterMode === 'blacklist');
  subFilterTabWhite?.classList.toggle('is-active', activeSubFilterMode === 'whitelist');
  if (subFilterAddButton) {
    subFilterAddButton.disabled = activeSubFilterMode === 'off';
    subFilterAddButton.classList.toggle('is-hidden', activeSubFilterMode === 'off');
  }
}

function renderSubFilterInputs() {
  if (!subFilterInputsEl) return;
  if (activeSubFilterMode === 'off') {
    subFilterInputsEl.innerHTML = '<div class="timeline-item"><div class="title">过滤已关闭</div></div>';
    return;
  }
  subFilterInputsEl.innerHTML = activeSubFilterKeywords.map((keyword, index) => `
    <div class="timeline-item">
      <div class="sub-url-row">
        <input data-sub-filter-index="${index}" value="${escapeHtmlAttr(keyword || '')}" placeholder="输入关键词" />
        ${activeSubFilterKeywords.length > 1 ? `<button type="button" data-remove-sub-filter="${index}">删除</button>` : ''}
      </div>
    </div>
  `).join('');
}

function saveSubFilterConfigLocally() {
  const parsed = parseFormConfig(false);
  if (!parsed.ok) {
    throw new Error(parsed.error);
  }
  const cfg = parsed.value;
  cfg.subscription = cfg.subscription || {};
  cfg.subscription.filters = Array.isArray(cfg.subscription.filters) ? cfg.subscription.filters : [];
  while (cfg.subscription.filters.length < formSubscriptionUrls.length) {
    cfg.subscription.filters.push({ mode: 'off', keywords: [] });
  }
  const keywords = activeSubFilterKeywords.map((v) => String(v || '').trim()).filter(Boolean);
  cfg.subscription.filters[activeSubFilterIndex] = {
    mode: activeSubFilterMode,
    keywords: activeSubFilterMode === 'off' ? [] : keywords
  };
  editor.value = JSON.stringify(cfg, null, 2);
  fillForm(cfg);
  renderSubscriptionUrls();
  formTouched = true;
  updateEditorState();
}

function hideSubAutoUpdateOverlay() {
  if (!subAutoUpdateOverlay) return;
  subAutoUpdateOverlay.classList.add('is-hidden');
  subAutoUpdateOverlay.setAttribute('aria-hidden', 'true');
}

function renderSubAutoUpdateMode() {
  const scope = subAutoScopeEl?.value || 'off';
  const isSimultaneous = scope === 'simultaneous';
  const isOff = scope === 'off';
  const isIndependentItem = activeSubscriptionAutoConfigIndex >= 0;
  const isInterval = (subAutoModeEl?.value || 'interval') === 'interval';
  const showScope = !isIndependentItem;
  const showCommon = isIndependentItem || isSimultaneous;
  subAutoScopeWrap?.classList.toggle('is-hidden', !showScope);
  subAutoModeWrap?.classList.toggle('is-hidden', !showCommon);
  subAutoTargetsWrap?.classList.toggle('is-hidden', !(isSimultaneous && !isIndependentItem));
  subAutoIntervalWrap?.classList.toggle('is-hidden', !(showCommon && isInterval));
  subAutoTimeWrap?.classList.toggle('is-hidden', !(showCommon && !isInterval));
  subAutoDayWrap?.classList.toggle('is-hidden', !(showCommon && !isInterval));
  if (isOff && !isIndependentItem) {
    subAutoModeWrap?.classList.add('is-hidden');
    subAutoIntervalWrap?.classList.add('is-hidden');
    subAutoTimeWrap?.classList.add('is-hidden');
    subAutoDayWrap?.classList.add('is-hidden');
    subAutoTargetsWrap?.classList.add('is-hidden');
  }
  renderSubAutoTargets();
}

function renderSubAutoTargets() {
  if (!subAutoTargetsEl) return;
  const parsed = parseFormConfig(false);
  const cfg = parsed.ok ? parsed.value : (latestData.config || {});
  const urls = (cfg.subscription?.urls || []).filter(Boolean);
  const selected = new Set(cfg.subscription?.autoUpdate?.targets || urls);
  subAutoTargetsEl.innerHTML = urls.map((url, index) => `
    <label class="member-option">
      <input type="checkbox" data-sub-auto-target="${index}" ${selected.has(url) ? 'checked' : ''} />
      <span>${escapeHtml(url)}</span>
    </label>
  `).join('') || '<div class="timeline-item"><div class="title">暂无订阅地址</div></div>';
}

function collectSubAutoTargets() {
  if ((subAutoScopeEl?.value || 'off') !== 'simultaneous') {
    return [];
  }
  const selected = [];
  const urlMap = formSubscriptionUrls.map((item) => item.url.trim()).filter(Boolean);
  for (const input of subAutoTargetsEl?.querySelectorAll('[data-sub-auto-target]') || []) {
    if (!(input instanceof HTMLInputElement) || !input.checked) continue;
    const index = Number(input.dataset.subAutoTarget);
    if (urlMap[index]) selected.push(urlMap[index]);
  }
  return selected;
}

function ensureSocksConfigNotCancelled() {
  if (socksConfigAbortController?.signal?.aborted) {
    throw new Error('用户取消了操作');
  }
}

async function waitForRuntimeReady(timeoutMs = 8000) {
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    const cfg = await api('/api/config');
    if (cfg?.runtime?.running) {
      await sleep(700);
      return;
    }
    await sleep(250);
  }
  throw new Error('sing-box 启动超时');
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function createDefaultSubscriptionUrl() {
  return { url: '' };
}

function inferDnsPreset(remoteUrl = '') {
  if (remoteUrl === DNS_PRESET_URLS.google) return 'google';
  if (remoteUrl === DNS_PRESET_URLS.cloudflare) return 'cloudflare';
  return 'custom';
}

function inferBootstrapPreset(value = '') {
  return DNS_BOOTSTRAP_PRESETS.includes(value) ? value : 'custom';
}

function markFormInteraction(active) {
  isFormInteracting = active;
}

function isInteractiveFormElement(target) {
  return target instanceof HTMLInputElement
    || target instanceof HTMLSelectElement
    || target instanceof HTMLTextAreaElement;
}

function isEditorDirty() {
  return editor.value !== lastSavedConfigText;
}

function isFormDirty() {
  const parsed = parseFormConfig(false);
  return parsed.ok ? parsed.text !== lastSavedConfigText : true;
}

function syncFormToJson() {
  const parsed = parseFormConfig(false);
  if (parsed.ok) editor.value = parsed.text;
  updateEditorState();
  return parsed;
}

function syncJsonToForm() {
  const parsed = parseJsonEditor();
  if (parsed.ok) {
    fillForm(parsed.value);
    formTouched = false;
    renderSubscriptionUrls();
    renderSocksServices();
  }
  updateEditorState();
  return parsed;
}

function switchView(view) {
  if (view === currentView) return;
  if (view === 'json') {
    const parsed = syncFormToJson();
    if (!parsed.ok) return setStatus(`切换失败：${parsed.error}`, 'error');
  } else {
    const parsed = syncJsonToForm();
    if (!parsed.ok) return setStatus(`切换失败：${parsed.error}`, 'error');
  }
  currentView = view;
  formView.classList.toggle('is-hidden', view !== 'form');
  jsonView.classList.toggle('is-hidden', view !== 'json');
  switchFormButton.classList.toggle('is-active', view === 'form');
  switchJsonButton.classList.toggle('is-active', view === 'json');
  updateEditorState();
}

function switchTab(tabName) {
  for (const button of tabButtons) {
    button.classList.toggle('is-active', button.dataset.tab === tabName);
  }
  for (const [name, panel] of Object.entries(tabPanels)) {
    panel.classList.toggle('is-active', name === tabName);
    panel.classList.toggle('is-hidden', name !== tabName);
  }
}

function setupInfoView(name) {
  const formViewEl = document.getElementById(`${name}-form-view`);
  const jsonViewEl = document.getElementById(`${name}-json-view`);
  const formButton = document.getElementById(`switch-${name}-form`);
  const jsonButton = document.getElementById(`switch-${name}-json`);
  const state = { formViewEl, jsonViewEl, formButton, jsonButton };
  formButton.addEventListener('click', () => setInfoView(name, 'form'));
  jsonButton.addEventListener('click', () => setInfoView(name, 'json'));
  return state;
}

function setInfoView(name, mode) {
  const state = infoViews[name];
  state.formViewEl.classList.toggle('is-hidden', mode !== 'form');
  state.jsonViewEl.classList.toggle('is-hidden', mode !== 'json');
  state.formButton.classList.toggle('is-active', mode === 'form');
  state.jsonButton.classList.toggle('is-active', mode === 'json');
}

function renderKeyValue(container, entries) {
  container.innerHTML = '';
  for (const [key, value] of Object.entries(entries)) {
    const item = document.createElement('div');
    item.className = 'kv-item';
    item.innerHTML = `<div class="key">${escapeHtml(key)}</div><div class="value">${escapeHtml(String(value))}</div>`;
    container.appendChild(item);
  }
}

function renderTimeline(container, items) {
  container.innerHTML = '';
  if (!items.length) {
    const empty = document.createElement('div');
    empty.className = 'timeline-item';
    empty.innerHTML = '<div class="title">暂无内容</div>';
    container.appendChild(empty);
    return;
  }

  for (const item of items.slice().reverse()) {
    const node = document.createElement('div');
    node.className = 'timeline-item';
    node.innerHTML = `
      <div class="time">${escapeHtml(item.time || '')}</div>
      <div class="title">${escapeHtml(item.title || '')}</div>
      <div class="details">${escapeHtml(item.details || '')}</div>
    `;
    container.appendChild(node);
  }
}

function renderLogTimeline(container, logs) {
  renderTimeline(container, logs.map((line, index) => ({
    time: `#${index + 1}`,
    title: '运行日志',
    details: line
  })));
}

function flattenObject(input, prefix = '', output = {}) {
  for (const [key, value] of Object.entries(input || {})) {
    const nextKey = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      flattenObject(value, nextKey, output);
    } else {
      output[nextKey] = Array.isArray(value) ? JSON.stringify(value) : (value ?? '');
    }
  }
  return output;
}

function buildSubscriptionSummary(subscription) {
  const nodes = subscription?.nodes || [];
  return {
    updatedAt: subscription?.updatedAt || '',
    nodeCount: nodes.length,
    warningCount: (subscription?.warnings || []).length,
    firstNode: nodes[0]?.tag || '',
    warnings: (subscription?.warnings || []).join(' | ')
  };
}

const LABEL_MAP = {
  stored: '已检测',
  platform: '系统平台',
  arch: '架构',
  executableName: '可执行文件名',
  assetSuffix: '资产后缀',
  plannedVersion: '计划版本',
  installed: '已安装',
  binaryPath: '二进制路径',
  installedVersion: '已安装版本',
  plannedAsset: '计划资产',
  updatedAt: '更新时间',
  nodeCount: '节点数量',
  warningCount: '警告数量',
  firstNode: '首选节点',
  warnings: '警告信息',
  rawLength: '原始长度'
};

function chineseLabels(obj) {
  const result = {};
  for (const [key, value] of Object.entries(obj)) {
    result[LABEL_MAP[key] || key] = value;
  }
  return result;
}

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

function buildGeneratedSummary(generated) {
  return {
    inboundCount: generated?.inbounds?.length || 0,
    outboundCount: generated?.outbounds?.length || 0,
    routeRuleCount: generated?.route?.rules?.length || 0,
    dnsServerCount: generated?.dns?.servers?.length || 0,
    finalOutbound: generated?.route?.final || '',
    logLevel: generated?.log?.level || ''
  };
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function escapeHtmlAttr(value) {
  return escapeHtml(value);
}

async function action(label, fn) {
  try {
    setBusy(true);
    setStatus(`${label}进行中...`, 'loading');
    await fn();
    await load();
    if (!syncStatusBarWithDownload(latestData.download)) {
      setStatus(`${label}完成`, 'success');
    }
  } catch (error) {
    setStatus(`${label}失败：${error.message}`, 'error');
  } finally {
    if (!downloadInFlight) setBusy(false);
  }
}

async function detectArchitectureAndLoadReleases() {
  // Real auto-detection from current runtime platform.
  kernelArchManuallySelected = false;
  await post('/api/kernel/architecture', {});
  await api('/api/kernel/releases');
  selectedKernelVersion = '';
}

async function saveCurrentConfigIfNeeded(force = false) {
  const validation = currentView === 'json' ? parseJsonEditor() : parseFormConfig(true);
  if (!validation.ok) {
    updateEditorState();
    throw new Error(validation.error);
  }

  const currentText = validation.text;
  const needsSave = force || currentText !== lastSavedConfigText;
  if (!needsSave) {
    return { saved: false, config: validation.value, text: currentText };
  }

  editor.value = currentText;
  await post('/api/config', validation.value);
  lastSavedConfigText = currentText;
  formTouched = false;
  fillForm(validation.value);
  renderSubscriptionUrls();
  renderSocksServices();
  updateEditorState();
  return { saved: true, config: validation.value, text: currentText };
}

function buildSubscriptionNodeFingerprint(subscription) {
  const nodes = Array.isArray(subscription?.nodes) ? subscription.nodes : [];
  const compact = nodes
    .map((node) => ({
      tag: String(node?.tag || ''),
      type: String(node?.type || ''),
      server: String(node?.server || ''),
      server_port: Number(node?.server_port || 0)
    }))
    .sort((a, b) => a.tag.localeCompare(b.tag) || a.type.localeCompare(b.type) || a.server.localeCompare(b.server) || a.server_port - b.server_port);
  return JSON.stringify(compact);
}

async function runSubscriptionRefreshFlow() {
  const beforeFingerprint = buildSubscriptionNodeFingerprint(latestData.subscription);
  await post('/api/subscription/refresh');
  await load();

  if (fields.appAutoConfigureSubscription.value !== 'true') {
    return;
  }

  const afterFingerprint = buildSubscriptionNodeFingerprint(latestData.subscription);
  if (beforeFingerprint === afterFingerprint) {
    setStatus('订阅节点未变化，已跳过自动配置 SOCKS5 服务', 'idle');
    return;
  }

  await autoConfigureSocksServicesFromOutbounds();
  const parsed = parseFormConfig(true);
  if (!parsed.ok) throw new Error(parsed.error);
  editor.value = parsed.text;
  await post('/api/config', parsed.value);
  lastSavedConfigText = parsed.text;
  formTouched = false;
  fillForm(parsed.value);
  renderSubscriptionUrls();
  renderSocksServices();
  updateEditorState();
}

async function applySelectedArchitectureAndLoadReleases() {
  await post('/api/kernel/architecture', { assetSuffix: selectedKernelArch || kernelArchSelect.value });
  await api('/api/kernel/releases');
}

async function startDownloadFlow() {
  downloadInFlight = true;
  setBusy(false);
  try {
    await applySelectedArchitectureAndLoadReleases();
    if (kernelVersionSelect.value) {
      await post('/api/kernel/plan', {
        version: kernelVersionSelect.value,
        assetSuffix: selectedKernelArch || kernelArchSelect.value
      });
    }
    await post('/api/kernel/download');
    await load();
    syncStatusBarWithDownload(latestData.download);
  } finally {
    downloadInFlight = false;
    setBusy(false);
  }
}

async function refreshAfterNodesUpdate() {
  const current = localStorage.getItem(NODES_UPDATED_KEY) || '';
  if (current && current !== lastKnownNodesUpdatedAt) {
    lastKnownNodesUpdatedAt = current;
    await load();
    setStatus('节点列表已同步到主页', 'success');
  }
}

document.getElementById('save-config').onclick = () => action('保存配置', async () => {
  await saveCurrentConfigIfNeeded(true);
  setStatus('配置已保存并自动更新 sing-box 配置', 'success');
});

document.getElementById('refresh-sub').onclick = () => action('更新订阅', async () => {
  const saveResult = await saveCurrentConfigIfNeeded(false);
  if (saveResult.saved) {
    setStatus('检测到未保存配置，已先保存配置', 'success');
  }
  await runSubscriptionRefreshFlow();
});

document.getElementById('start').onclick = () => action('启动 sing-box', async () => {
  await post('/api/runtime/start');
});

document.getElementById('stop').onclick = () => action('停止 sing-box', async () => {
  await post('/api/runtime/stop');
});

document.getElementById('kernel-architecture-detect').onclick = () => action('检测当前架构', async () => {
  await detectArchitectureAndLoadReleases();
});

document.getElementById('kernel-check').onclick = () => action('检查内核版本', async () => {
  await Promise.all([api('/api/kernel/status'), api('/api/kernel/releases')]);
});

kernelCheckUpdatesButton.onclick = () => action('检查版本更新', async () => {
  await post('/api/kernel/releases/update');
});

kernelSelectVersionButton.onclick = () => action('设为计划版本', async () => {
  await applySelectedArchitectureAndLoadReleases();
  if (!kernelVersionSelect.value) throw new Error('请先选择一个内核版本');
  selectedKernelVersion = kernelVersionSelect.value;
  kernelArchManuallySelected = false;
  await post('/api/kernel/plan', {
    version: kernelVersionSelect.value,
    assetSuffix: selectedKernelArch || kernelArchSelect.value
  });
});

document.getElementById('kernel-download').onclick = async () => {
  try {
    setStatus('开始拉取 sing-box 内核...', 'loading');
    await startDownloadFlow();
  } catch (error) {
    setStatus(`拉取 sing-box 内核失败：${error.message}`, 'error');
  }
};

manageNodesButton?.addEventListener('click', () => {
  window.location.href = '/nodes.html';
});

addSubscriptionUrlButton?.addEventListener('click', () => {
  formSubscriptionUrls.push(createDefaultSubscriptionUrl());
  renderSubscriptionUrls();
  formTouched = true;
  updateEditorState();
});

editSocksServiceButton?.addEventListener('click', () => {
  window.location.href = '/socks5.html';
});

exportSocksButton?.addEventListener('click', () => {
  const lines = formPorts
    .filter(p => p.listen && p.port)
    .map(p => `socks5://${p.listen}:${p.port}`);
  const json = JSON.stringify(lines, null, 2);
  const blob = new Blob([json], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'socks5.json';
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
});

copySocksButton?.addEventListener('click', () => {
  const list = buildSocksAddressList();
  if (!list.length) {
    showToast('没有可复制的服务', false);
    return;
  }
  showCopySocksOverlay();
});

copySocksTabLines?.addEventListener('click', () => {
  copySocksMode = 'lines';
  renderCopySocksTabs();
});

copySocksTabCustom?.addEventListener('click', () => {
  copySocksMode = 'custom';
  renderCopySocksTabs();
});

copySocksCustomSeparator?.addEventListener('input', () => {
  updateCopySocksPreviews();
});

copySocksConfirmButton?.addEventListener('click', () => {
  updateCopySocksPreviews();
  const text = copySocksMode === 'custom'
    ? (copySocksCustomPreview?.value || '')
    : (copySocksLinesPreview?.value || '');
  if (!text.trim()) {
    showToast('没有可复制的内容', false);
    return;
  }
  navigator.clipboard.writeText(text).then(
    () => {
      showToast('复制成功', true);
      hideCopySocksOverlay();
    },
    () => showToast('复制失败', false)
  );
});

copySocksCancelButton?.addEventListener('click', () => {
  hideCopySocksOverlay();
});

subFilterTabOff?.addEventListener('click', () => {
  activeSubFilterMode = 'off';
  renderSubFilterTabs();
  renderSubFilterInputs();
});

subFilterTabBlack?.addEventListener('click', () => {
  activeSubFilterMode = 'blacklist';
  if (!activeSubFilterKeywords.length) activeSubFilterKeywords = [''];
  renderSubFilterTabs();
  renderSubFilterInputs();
});

subFilterTabWhite?.addEventListener('click', () => {
  activeSubFilterMode = 'whitelist';
  if (!activeSubFilterKeywords.length) activeSubFilterKeywords = [''];
  renderSubFilterTabs();
  renderSubFilterInputs();
});

subFilterAddButton?.addEventListener('click', () => {
  if (activeSubFilterMode === 'off') return;
  activeSubFilterKeywords.push('');
  renderSubFilterInputs();
});

subFilterSaveButton?.addEventListener('click', async () => {
  try {
    if (activeSubFilterIndex < 0) return;
    saveSubFilterConfigLocally();
    const parsed = parseFormConfig(true);
    if (!parsed.ok) throw new Error(parsed.error);
    await postConfigWithoutRuntimeRestart(parsed.value);
    lastSavedConfigText = parsed.text;
    formTouched = false;
    hideSubFilterOverlay();
    setStatus('订阅过滤设置已保存', 'success');
  } catch (error) {
    setStatus(`保存失败：${error.message}`, 'error');
  }
});

subFilterCancelButton?.addEventListener('click', () => hideSubFilterOverlay());

autoConfigureSocksButton?.addEventListener('click', () => action('一键配置 SOCKS5 服务', async () => {
  try {
    await load();
    await autoConfigureSocksServicesFromOutbounds();
    const configToSave = structuredClone(latestData.config || {});
    const parsed = parseFormConfig(true);
    if (!parsed.ok) throw new Error(parsed.error);
    configToSave.subscription = parsed.value.subscription;
    configToSave.app = parsed.value.app;
    configToSave.dns = parsed.value.dns;
    configToSave.routing = parsed.value.routing;
    configToSave.ports = parsed.value.ports;
    editor.value = JSON.stringify(configToSave, null, 2);
    await post('/api/config', configToSave);
    lastSavedConfigText = editor.value;
    formTouched = false;
    latestData.config = configToSave;
    fillForm(configToSave);
    renderSubscriptionUrls();
    renderSocksServices();
    updateEditorState();
  } finally {
    hideSocksConfigOverlay();
    socksConfigAbortController = null;
  }
}));

openSubAutoUpdateButton?.addEventListener('click', () => {
  activeSubscriptionAutoConfigIndex = -1;
  showSubAutoUpdateOverlay();
});

saveSubUrlZoomButton?.addEventListener('click', () => {
  if (activeZoomSubscriptionIndex >= 0 && formSubscriptionUrls[activeZoomSubscriptionIndex]) {
    formSubscriptionUrls[activeZoomSubscriptionIndex].url = subUrlZoomInput.value;
    renderSubscriptionUrls();
    formTouched = true;
    updateEditorState();
  }
  hideSubUrlZoomOverlay();
});

cancelSubUrlZoomButton?.addEventListener('click', () => hideSubUrlZoomOverlay());

saveSubAutoUpdateButton?.addEventListener('click', () => {
  const persistAutoUpdateNow = async () => {
    const parsed = parseFormConfig(true);
    if (!parsed.ok) {
      throw new Error(parsed.error);
    }
    editor.value = parsed.text;
    await postConfigWithoutRuntimeRestart(parsed.value);
    lastSavedConfigText = parsed.text;
    formTouched = false;
    fillForm(parsed.value);
    renderSubscriptionUrls();
    renderSocksServices();
    updateEditorState();
  };

  if (activeSubscriptionAutoConfigIndex >= 0) {
    const parsed = parseFormConfig(false);
    if (!parsed.ok) {
      setStatus(parsed.error, 'error');
      return;
    }
    const cfg = parsed.value;
    cfg.subscription = cfg.subscription || {};
    cfg.subscription.autoUpdate = cfg.subscription.autoUpdate || {};
    cfg.subscription.autoUpdate.items = cfg.subscription.autoUpdate.items || [];
    cfg.subscription.autoUpdate.items[activeSubscriptionAutoConfigIndex] = {
      mode: subAutoModeEl.value,
      intervalMinutes: Number(subAutoIntervalEl.value || 60),
      time: subAutoTimeEl.value || '03:00',
      dayMode: subAutoDayModeEl.value || 'daily'
    };
    editor.value = JSON.stringify(cfg, null, 2);
    fillForm(cfg);
    renderSubscriptionUrls();
    formTouched = true;
    updateEditorState();
    persistAutoUpdateNow()
      .then(() => {
        hideSubAutoUpdateOverlay();
        setStatus('已保存当前订阅的独立自动更新设置', 'success');
      })
      .catch((error) => setStatus(`保存失败：${error.message}`, 'error'));
    activeSubscriptionAutoConfigIndex = -1;
    return;
  }
  renderSubAutoUpdateMode();
  formTouched = true;
  updateEditorState();
  persistAutoUpdateNow()
    .then(() => {
      hideSubAutoUpdateOverlay();
      setStatus('已保存自动更新设置', 'success');
    })
    .catch((error) => setStatus(`保存失败：${error.message}`, 'error'));
  activeSubscriptionAutoConfigIndex = -1;
});

cancelSubAutoUpdateButton?.addEventListener('click', () => hideSubAutoUpdateOverlay());

subAutoModeEl?.addEventListener('change', () => {
  renderSubAutoUpdateMode();
  formTouched = true;
  updateEditorState();
});

subAutoScopeEl?.addEventListener('change', () => {
  renderSubAutoUpdateMode();
  renderSubscriptionUrls();
  formTouched = true;
  updateEditorState();
});

cancelSocksConfigButton?.addEventListener('click', () => {
  if (!socksConfigAbortController) return;
  const confirmed = window.confirm('确认取消当前一键配置 SOCKS5 服务任务吗？');
  if (!confirmed) return;
  socksConfigAbortController.abort();
  updateSocksConfigOverlay('取消中', 0, 0, '正在取消任务，请稍候...');
});
document.addEventListener('input', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLInputElement || target instanceof HTMLSelectElement)) return;

  if (target.dataset.portIndex) {
    const index = Number(target.dataset.portIndex);
    const field = target.dataset.portField;
    formPorts[index][field] = target.value;
    formTouched = true;
    markFormInteraction(true);
    updateEditorState();
  }

  if (target.dataset.subscriptionIndex) {
    const index = Number(target.dataset.subscriptionIndex);
    formSubscriptionUrls[index].url = target.value;
    formTouched = true;
    markFormInteraction(true);
    updateEditorState();
  }
});

document.addEventListener('change', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLInputElement || target instanceof HTMLSelectElement)) return;

  if (target.dataset.portIndex) {
    const index = Number(target.dataset.portIndex);
    const field = target.dataset.portField;
    formPorts[index][field] = target.value;
    formTouched = true;
    markFormInteraction(true);
    updateEditorState();
  }
});

document.addEventListener('focusin', (event) => {
  if (isInteractiveFormElement(event.target) && formView.contains(event.target)) {
    markFormInteraction(true);
  }
});

document.addEventListener('focusout', () => {
  setTimeout(() => {
    const active = document.activeElement;
    if (!(active instanceof Element) || !formView.contains(active) || !isInteractiveFormElement(active)) {
      markFormInteraction(false);
    }
  }, 0);
});

document.addEventListener('click', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLElement)) return;

  if (target.dataset.removePort) {
    const index = Number(target.dataset.removePort);
    if (formPorts.length > 1) {
      formPorts.splice(index, 1);
      renderSocksServices();
      formTouched = true;
      updateEditorState();
    }
  }

  if (target.dataset.toggleSubscriptionExpand) {
    const index = Number(target.dataset.toggleSubscriptionExpand);
    if (expandedSubscriptionInputs.has(index)) {
      expandedSubscriptionInputs.delete(index);
    } else {
      expandedSubscriptionInputs.add(index);
    }
    renderSubscriptionUrls();
  }

  if (target.dataset.zoomSubscription) {
    showSubUrlZoomOverlay(Number(target.dataset.zoomSubscription));
  }

  if (target.dataset.openSubAutoUpdate) {
    activeSubscriptionAutoConfigIndex = -1;
    showSubAutoUpdateOverlay();
  }

  if (target.dataset.openSubFilter) {
    showSubFilterOverlay(Number(target.dataset.openSubFilter));
  }

  if (target.dataset.openSubAutoUpdateItem) {
    activeSubscriptionAutoConfigIndex = Number(target.dataset.openSubAutoUpdateItem);
    const parsed = parseFormConfig(false);
    const cfg = parsed.ok ? parsed.value : (latestData.config || {});
    const itemCfg = cfg.subscription?.autoUpdate?.items?.[activeSubscriptionAutoConfigIndex] || {};
    subAutoModeEl.value = itemCfg.mode || cfg.subscription?.autoUpdate?.mode || 'interval';
    subAutoIntervalEl.value = itemCfg.intervalMinutes || cfg.subscription?.autoUpdate?.intervalMinutes || 60;
    subAutoTimeEl.value = itemCfg.time || cfg.subscription?.autoUpdate?.time || '03:00';
    subAutoDayModeEl.value = itemCfg.dayMode || cfg.subscription?.autoUpdate?.dayMode || 'daily';
    showSubAutoUpdateOverlay();
  }

  if (target.id === 'add-socks-service') {
    formPorts.push(createDefaultPort());
    renderSocksServices();
    formTouched = true;
    updateEditorState();
  }

  if (target.dataset.suggestPort) {
    const index = Number(target.dataset.suggestPort);
    const host = formPorts[index]?.listen || '127.0.0.1';
    const used = formPorts
      .map((item, i) => (i === index ? 0 : Number(item.port)))
      .filter((p) => Number.isInteger(p) && p > 0);
    const start = Number(formPorts[index]?.port || 0) > 0
      ? Number(formPorts[index].port) + 1
      : Number(fields.appPort.value || 18080) + 1;
    resolveNextPort(host, start, used).then((port) => {
      formPorts[index].port = port;
      renderSocksServices();
      formTouched = true;
      updateEditorState();
    }).catch(() => {});
  }

  if (target.dataset.removeSubscription) {
    const index = Number(target.dataset.removeSubscription);
    if (formSubscriptionUrls.length > 1) {
      formSubscriptionUrls.splice(index, 1);
      renderSubscriptionUrls();
      formTouched = true;
      updateEditorState();
    }
  }

  if (target.dataset.removeSubFilter) {
    const index = Number(target.dataset.removeSubFilter);
    if (activeSubFilterKeywords.length > 1) {
      activeSubFilterKeywords.splice(index, 1);
      renderSubFilterInputs();
    }
  }
});

document.addEventListener('input', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLInputElement)) return;
  if (target.dataset.subFilterIndex) {
    const index = Number(target.dataset.subFilterIndex);
    activeSubFilterKeywords[index] = target.value;
  }
});

kernelArchSelect.addEventListener('change', () => {
  kernelArchManuallySelected = true;
  selectedKernelArch = kernelArchSelect.value;
  const [os, archName] = kernelArchSelect.value.split('-');
  latestData.architecture = {
    ...(latestData.architecture || {}),
    os,
    archName,
    assetSuffix: kernelArchSelect.value
  };
});

kernelVersionSelect.addEventListener('change', () => {
  selectedKernelVersion = kernelVersionSelect.value;
});

fields.dnsRemotePreset.addEventListener('change', () => {
  if (fields.dnsRemotePreset.value !== 'custom') {
    fields.dnsRemoteUrl.value = DNS_PRESET_URLS[fields.dnsRemotePreset.value] || DNS_PRESET_URLS.cloudflare;
  }
  renderDnsPresetUi();
  formTouched = true;
  updateEditorState();
});

fields.dnsBootstrapPreset.addEventListener('change', () => {
  if (fields.dnsBootstrapPreset.value !== 'custom') {
    fields.dnsBootstrap.value = fields.dnsBootstrapPreset.value;
  }
  renderDnsPresetUi();
  formTouched = true;
  updateEditorState();
});

fields.appPort?.addEventListener('change', async () => {
  if (!formPorts.length) return;
  const currentAppPort = Number(fields.appPort.value || 18080);
  const firstPort = Number(formPorts[0]?.port || 0);
  if (!firstPort || firstPort <= currentAppPort) {
    formPorts[0].port = '';
    const changed = await assignMissingSuggestedPorts();
    if (changed) {
      renderSocksServices();
    }
    formTouched = true;
    updateEditorState();
  }
});

switchFormButton.addEventListener('click', () => switchView('form'));
switchJsonButton.addEventListener('click', () => switchView('json'));
editor.addEventListener('input', updateEditorState);

for (const button of tabButtons) {
  button.addEventListener('click', () => switchTab(button.dataset.tab));
}

for (const element of Object.values(fields)) {
  element?.addEventListener('input', () => {
    formTouched = true;
    markFormInteraction(true);
    updateEditorState();
  });
  element?.addEventListener('change', () => {
    formTouched = true;
    markFormInteraction(true);
    updateEditorState();
  });
}

window.addEventListener('storage', (event) => {
  if (event.key === NODES_UPDATED_KEY) {
    refreshAfterNodesUpdate().catch(() => {});
  }
});

load()
  .then(() => {
    switchView('form');
    switchTab('overview');
    if (!syncStatusBarWithDownload(latestData.download)) {
      setStatus('准备就绪', 'idle');
    }
  })
  .catch((error) => setStatus(`初始化失败：${error.message}`, 'error'));

setInterval(() => {
  if (currentView === 'form' && (formTouched || isFormInteracting)) {
    refreshAfterNodesUpdate().catch(() => {});
    return;
  }

  Promise.all([load(), refreshAfterNodesUpdate()])
    .then(() => {
      const downloading = syncStatusBarWithDownload(latestData.download);
      if (downloading) {
        setBusy(false);
      } else if (!statusBar.classList.contains('is-error')) {
        setStatus('准备就绪', 'idle');
      }
    })
    .catch(() => {});
}, 2000);

