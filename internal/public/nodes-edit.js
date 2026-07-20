const statusEl = document.getElementById('edit-node-status');
const inputEl = document.getElementById('edit-manual-node-input');
const importResultEl = document.getElementById('edit-manual-import-result');
const nodeListEl = document.getElementById('edit-node-list');
const formTypeEl = document.getElementById('manual-form-type');
const formTagEl = document.getElementById('manual-form-tag');
const formServerEl = document.getElementById('manual-form-server');
const formPortEl = document.getElementById('manual-form-port');
const formFieldsEl = document.getElementById('manual-form-fields');
const NODES_UPDATED_KEY = 'sub2socks5:nodes-updated-at';

const PROTOCOL_LABELS = {
  vless: 'VLESS',
  vmess: 'VMess',
  trojan: 'Trojan',
  shadowsocks: 'Shadowsocks',
  socks: 'SOCKS5',
  hysteria2: 'Hysteria2',
  tuic: 'TUIC'
};

const PROTOCOL_DEFAULT_PORTS = {
  shadowsocks: 8388,
  socks: 1080
};

const TLS_FINGERPRINT_OPTIONS = [
  { value: '', label: '不启用 uTLS' },
  { value: 'chrome', label: 'Chrome' },
  { value: 'firefox', label: 'Firefox' },
  { value: 'edge', label: 'Edge' },
  { value: 'safari', label: 'Safari' },
  { value: 'ios', label: 'iOS' },
  { value: 'android', label: 'Android' },
  { value: 'random', label: '随机指纹' }
];

const TRANSPORT_FIELDS = [
  {
    key: 'transportType',
    label: '传输方式',
    type: 'select',
    group: '传输设置',
    defaultValue: 'tcp',
    options: [
      { value: 'tcp', label: 'TCP（无额外传输层）' },
      { value: 'ws', label: 'WebSocket' },
      { value: 'grpc', label: 'gRPC' },
      { value: 'http', label: 'HTTP' }
    ]
  },
  {
    key: 'transportHost',
    label: 'Host',
    group: '传输设置',
    placeholder: 'cdn.example.com',
    help: 'WebSocket 请求头或 HTTP Host。',
    showWhen: { field: 'transportType', values: ['ws', 'http'] }
  },
  {
    key: 'transportPath',
    label: 'Path',
    group: '传输设置',
    placeholder: '/',
    help: '留空时使用 /。',
    showWhen: { field: 'transportType', values: ['ws', 'http'] }
  },
  {
    key: 'grpcServiceName',
    label: 'gRPC Service Name',
    group: '传输设置',
    placeholder: 'grpc-service',
    showWhen: { field: 'transportType', values: ['grpc'] }
  }
];

const FORM_PROTOCOLS = {
  vless: [
    { key: 'uuid', label: 'UUID', group: '认证信息', required: true, placeholder: 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx' },
    {
      key: 'flow',
      label: 'Flow',
      type: 'select',
      group: '认证信息',
      options: [
        { value: '', label: '无' },
        { value: 'xtls-rprx-vision', label: 'xtls-rprx-vision' }
      ]
    },
    {
      key: 'tlsMode',
      label: 'TLS 模式',
      type: 'select',
      group: 'TLS 设置',
      defaultValue: 'tls',
      options: [
        { value: 'none', label: '不启用 TLS' },
        { value: 'tls', label: 'TLS' },
        { value: 'reality', label: 'Reality' }
      ]
    },
    { key: 'tlsServerName', label: 'SNI', group: 'TLS 设置', placeholder: 'front.example.com', showWhen: { field: 'tlsMode', values: ['tls', 'reality'] } },
    { key: 'tlsInsecure', label: '跳过证书验证', type: 'bool', group: 'TLS 设置', showWhen: { field: 'tlsMode', values: ['tls', 'reality'] } },
    { key: 'tlsFingerprint', label: 'TLS 指纹', type: 'select', group: 'TLS 设置', defaultValue: 'chrome', options: TLS_FINGERPRINT_OPTIONS, help: 'Reality 客户端必须启用 uTLS，推荐使用 Chrome。', showWhen: { field: 'tlsMode', values: ['tls', 'reality'] } },
    { key: 'realityPublicKey', label: 'Reality Public Key', group: 'TLS 设置', required: true, placeholder: '服务端 Reality 公钥', showWhen: { field: 'tlsMode', values: ['reality'] } },
    { key: 'realityShortID', label: 'Reality Short ID', group: 'TLS 设置', placeholder: '例如 6ba85179e30d4fc2', showWhen: { field: 'tlsMode', values: ['reality'] } },
    ...TRANSPORT_FIELDS
  ],
  vmess: [
    { key: 'uuid', label: 'UUID', group: '认证信息', required: true, placeholder: 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx' },
    {
      key: 'security',
      label: '加密方式',
      type: 'select',
      group: '认证信息',
      defaultValue: 'auto',
      options: ['auto', 'aes-128-gcm', 'chacha20-poly1305', 'none']
    },
    { key: 'alter_id', label: 'Alter ID', type: 'number', group: '认证信息', defaultValue: 0, min: 0, step: 1 },
    {
      key: 'tlsMode',
      label: 'TLS 模式',
      type: 'select',
      group: 'TLS 设置',
      defaultValue: 'tls',
      options: [
        { value: 'none', label: '不启用 TLS' },
        { value: 'tls', label: 'TLS' }
      ]
    },
    { key: 'tlsServerName', label: 'SNI', group: 'TLS 设置', placeholder: 'front.example.com', showWhen: { field: 'tlsMode', values: ['tls'] } },
    { key: 'tlsInsecure', label: '跳过证书验证', type: 'bool', group: 'TLS 设置', showWhen: { field: 'tlsMode', values: ['tls'] } },
    { key: 'tlsFingerprint', label: 'TLS 指纹', type: 'select', group: 'TLS 设置', options: TLS_FINGERPRINT_OPTIONS, showWhen: { field: 'tlsMode', values: ['tls'] } },
    ...TRANSPORT_FIELDS
  ],
  trojan: [
    { key: 'password', label: '密码', group: '认证信息', required: true, placeholder: 'Trojan 密码' },
    { key: 'tlsServerName', label: 'SNI', group: 'TLS 设置', placeholder: 'front.example.com' },
    { key: 'tlsInsecure', label: '跳过证书验证', type: 'bool', group: 'TLS 设置' },
    { key: 'tlsFingerprint', label: 'TLS 指纹', type: 'select', group: 'TLS 设置', options: TLS_FINGERPRINT_OPTIONS },
    ...TRANSPORT_FIELDS
  ],
  shadowsocks: [
    {
      key: 'method',
      label: '加密方式',
      type: 'datalist',
      group: '认证信息',
      required: true,
      defaultValue: 'aes-256-gcm',
      options: [
        'aes-128-gcm',
        'aes-256-gcm',
        'chacha20-ietf-poly1305',
        '2022-blake3-aes-128-gcm',
        '2022-blake3-aes-256-gcm',
        '2022-blake3-chacha20-poly1305'
      ],
      help: '可从常用算法中选择，也可以直接输入其他 sing-box 支持的算法。'
    },
    { key: 'password', label: '密码', group: '认证信息', required: true, placeholder: 'Shadowsocks 密码' }
  ],
  socks: [
    { key: 'username', label: '用户名', group: '认证信息', placeholder: '可选' },
    { key: 'password', label: '密码', group: '认证信息', placeholder: '可选' }
  ],
  hysteria2: [
    { key: 'password', label: '认证密码', group: '认证信息', required: true, placeholder: 'Hysteria2 auth/password' },
    { key: 'serverPorts', label: '端口跳跃', group: '连接设置', placeholder: '20000-30000,40000', help: '多个端口用逗号分隔，范围可写成 20000-30000。' },
    { key: 'tlsServerName', label: 'SNI', group: 'TLS 设置', placeholder: 'front.example.com' },
    { key: 'tlsInsecure', label: '跳过证书验证', type: 'bool', group: 'TLS 设置' },
    { key: 'tlsAlpn', label: 'ALPN', group: 'TLS 设置', defaultValue: 'h3', placeholder: 'h3', help: '多个值用逗号分隔。' },
    { key: 'up_mbps', label: '上行带宽（Mbps）', type: 'number', group: '性能与混淆', min: 1, step: 1, placeholder: '100' },
    { key: 'down_mbps', label: '下行带宽（Mbps）', type: 'number', group: '性能与混淆', min: 1, step: 1, placeholder: '100' },
    {
      key: 'obfsType',
      label: '混淆类型',
      type: 'select',
      group: '性能与混淆',
      options: [
        { value: '', label: '不启用混淆' },
        { value: 'salamander', label: 'Salamander' }
      ]
    },
    { key: 'obfsPassword', label: '混淆密码', group: '性能与混淆', required: true, placeholder: 'Salamander 密码', showWhen: { field: 'obfsType', values: ['salamander'] } }
  ],
  tuic: [
    { key: 'uuid', label: 'UUID', group: '认证信息', required: true, placeholder: 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx' },
    { key: 'password', label: '密码', group: '认证信息', required: true, placeholder: 'TUIC 密码' },
    { key: 'tlsServerName', label: 'SNI', group: 'TLS 设置', placeholder: 'front.example.com' },
    { key: 'tlsInsecure', label: '跳过证书验证', type: 'bool', group: 'TLS 设置' },
    { key: 'tlsAlpn', label: 'ALPN', group: 'TLS 设置', defaultValue: 'h3', placeholder: 'h3', help: '多个值用逗号分隔。' },
    {
      key: 'congestion_control',
      label: '拥塞控制',
      type: 'select',
      group: '连接设置',
      defaultValue: 'bbr',
      options: ['bbr', 'cubic', 'new_reno']
    },
    {
      key: 'udp_relay_mode',
      label: 'UDP 转发模式',
      type: 'select',
      group: '连接设置',
      defaultValue: 'native',
      options: [
        { value: 'native', label: 'native' },
        { value: 'quic', label: 'quic' }
      ]
    },
    { key: 'zero_rtt_handshake', label: '启用 0-RTT', type: 'bool', group: '连接设置' }
  ]
};

let state = {
  subscriptionNodes: [],
  disabledSubscriptionTags: [],
  manualNodes: [],
  groups: [],
  chains: [],
  availableOutbounds: [],
  fallbackStates: {}
};

async function load() {
  const response = await fetch('/api/nodes');
  const data = await response.json();
  if (!response.ok) {
    throw new Error(data?.error?.message || '加载节点失败');
  }
  state = data;
  render();
}

function render() {
  renderImportTabs();
  renderFormFields();
  renderNodeList();
}

function renderImportTabs() {
  setTab('form');
  if (!formTypeEl.options.length) {
    for (const key of Object.keys(FORM_PROTOCOLS)) {
      const option = document.createElement('option');
      option.value = key;
      option.textContent = PROTOCOL_LABELS[key] || key;
      formTypeEl.appendChild(option);
    }
    formTypeEl.value = 'vless';
    setDefaultFormPort('vless');
  }
}

function setTab(tab) {
  const isForm = tab === 'form';
  document.getElementById('node-import-tab-form').classList.toggle('is-active', isForm);
  document.getElementById('node-import-tab-raw').classList.toggle('is-active', !isForm);
  document.getElementById('node-import-form-panel').classList.toggle('is-hidden', !isForm);
  document.getElementById('node-import-form-panel').classList.toggle('is-active', isForm);
  document.getElementById('node-import-raw-panel').classList.toggle('is-hidden', isForm);
  document.getElementById('node-import-raw-panel').classList.toggle('is-active', !isForm);
}

function renderFormFields({ resetPort = false } = {}) {
  const type = formTypeEl.value || 'vless';
  const fields = FORM_PROTOCOLS[type] || [];
  let currentGroup = '';
  const content = [];
  for (const field of fields) {
    if (field.group && field.group !== currentGroup) {
      currentGroup = field.group;
      content.push(`<div class="manual-form-group-heading"><span>${escapeHtml(currentGroup)}</span></div>`);
    }
    content.push(renderManualField(type, field));
  }
  formFieldsEl.innerHTML = content.join('');
  if (resetPort || !formPortEl.value) {
    setDefaultFormPort(type);
  }
  updateConditionalFields();
}

function renderManualField(type, field) {
  const value = field.defaultValue ?? '';
  const requiredMark = field.required ? '<b class="required-mark" aria-hidden="true">*</b>' : '';
  const help = field.help ? `<small class="field-help">${escapeHtml(field.help)}</small>` : '';
  const showWhenAttrs = field.showWhen
    ? ` data-show-when-field="${escapeHtmlAttr(field.showWhen.field)}" data-show-when-values="${escapeHtmlAttr(field.showWhen.values.join(','))}"`
    : '';
  const commonAttrs = [
    `data-manual-field="${escapeHtmlAttr(field.key)}"`,
    field.required ? 'required' : '',
    field.placeholder ? `placeholder="${escapeHtmlAttr(field.placeholder)}"` : '',
    field.min !== undefined ? `min="${escapeHtmlAttr(field.min)}"` : '',
    field.max !== undefined ? `max="${escapeHtmlAttr(field.max)}"` : '',
    field.step !== undefined ? `step="${escapeHtmlAttr(field.step)}"` : '',
    'autocomplete="off"',
    'spellcheck="false"'
  ].filter(Boolean).join(' ');

  let control = '';
  if (field.type === 'bool') {
    control = `<select ${commonAttrs}>
      <option value="false" ${String(value) === 'true' ? '' : 'selected'}>否</option>
      <option value="true" ${String(value) === 'true' ? 'selected' : ''}>是</option>
    </select>`;
  } else if (field.type === 'select') {
    const options = (field.options || []).map((option) => {
      const normalized = typeof option === 'object' ? option : { value: option, label: option };
      const selected = String(normalized.value) === String(value) ? ' selected' : '';
      return `<option value="${escapeHtmlAttr(normalized.value)}"${selected}>${escapeHtml(normalized.label)}</option>`;
    }).join('');
    control = `<select ${commonAttrs}>${options}</select>`;
  } else if (field.type === 'datalist') {
    const listID = `manual-${type}-${field.key}-options`;
    const options = (field.options || []).map((option) => `<option value="${escapeHtmlAttr(option)}"></option>`).join('');
    control = `<input ${commonAttrs} list="${escapeHtmlAttr(listID)}" value="${escapeHtmlAttr(value)}" />
      <datalist id="${escapeHtmlAttr(listID)}">${options}</datalist>`;
  } else {
    const inputType = field.type === 'number' ? 'number' : 'text';
    control = `<input ${commonAttrs} type="${inputType}" value="${escapeHtmlAttr(value)}" />`;
  }

  return `
    <label class="manual-form-field" data-manual-field-wrap="${escapeHtmlAttr(field.key)}"${showWhenAttrs}>
      <span class="manual-form-field-label">${escapeHtml(field.label)}${requiredMark}</span>
      ${control}
      ${help}
    </label>
  `;
}

function setDefaultFormPort(type) {
  formPortEl.value = String(PROTOCOL_DEFAULT_PORTS[type] || 443);
}

function updateConditionalFields() {
  for (const wrapper of formFieldsEl.querySelectorAll('[data-show-when-field]')) {
    const controller = formFieldsEl.querySelector(`[data-manual-field="${wrapper.dataset.showWhenField}"]`);
    const allowedValues = String(wrapper.dataset.showWhenValues || '').split(',');
    wrapper.classList.toggle('is-hidden', !controller || !allowedValues.includes(controller.value));
  }
}

function renderNodeList() {
  nodeListEl.innerHTML = '';
  const nodes = [
    ...state.subscriptionNodes.map((node) => ({ ...node, source: 'subscription' })),
    ...state.manualNodes.map((node) => ({ ...node, source: 'manual' }))
  ];
  if (!nodes.length) {
    nodeListEl.innerHTML = '<div class="timeline-item"><div class="title">暂无节点</div></div>';
    return;
  }

  for (const node of nodes) {
    const isDisabled = node.source === 'subscription'
      ? state.disabledSubscriptionTags.includes(node.tag)
      : false;
    const card = document.createElement('div');
    card.className = 'node-edit-card';
    const actionAttr = node.source === 'manual'
      ? `data-delete-manual-node="${escapeHtmlAttr(node.tag)}"`
      : isDisabled
        ? `data-enable-subscription-node="${escapeHtmlAttr(node.tag)}"`
        : `data-delete-subscription-node="${escapeHtmlAttr(node.tag)}"`;
    const actionClass = node.source === 'manual'
      ? 'danger-icon-button'
      : isDisabled
        ? 'success-text-button'
        : 'danger-icon-button';
    const actionText = node.source === 'manual'
      ? '🗑'
      : isDisabled
        ? '启用'
        : '🗑';
    const titleClass = isDisabled ? 'node-pill-title is-disabled' : 'node-pill-title';
    card.innerHTML = `
      <div class="node-pill">
        <div class="${titleClass}">${escapeHtml(node.tag || '')}</div>
        <div class="node-pill-tags">
          <span class="node-pill-tag">${escapeHtml(node.type || '')}</span>
          <span class="node-pill-tag is-source">${node.source === 'manual' ? '手动' : '订阅'}</span>
        </div>
      </div>
      <button type="button" class="${actionClass}" ${actionAttr} title="${node.source === 'manual' ? '删除节点' : (isDisabled ? '启用节点' : '禁用节点')}">${actionText}</button>
    `;
    nodeListEl.appendChild(card);
  }
}

function renderImportResult(result) {
  if (!result) {
    importResultEl.innerHTML = '';
    return;
  }

  const warnings = Array.isArray(result.warnings) ? result.warnings : [];
  const items = [];
  if (result.nodes?.length) {
    items.push(`<div class="timeline-item"><div class="title">成功解析 ${result.nodes.length} 个节点</div></div>`);
  }
  for (const warning of warnings) {
    items.push(`<div class="timeline-item"><div class="title">提示</div><div class="details">${escapeHtml(warning)}</div></div>`);
  }
  importResultEl.innerHTML = items.join('') || '<div class="timeline-item"><div class="title">没有可导入节点</div></div>';
}

function buildFormNode() {
  const type = formTypeEl.value;
  const serverPort = Number(formPortEl.value);
  const node = {
    type,
    tag: formTagEl.value.trim(),
    server: formServerEl.value.trim(),
    server_port: serverPort
  };

  if (!node.tag || !node.server || !Number.isInteger(serverPort) || serverPort < 1 || serverPort > 65535) {
    throw new Error('表单节点至少需要名称、服务器和端口');
  }

  const fields = FORM_PROTOCOLS[type] || [];
  const values = {};
  for (const field of fields) {
    const fieldEl = formFieldsEl.querySelector(`[data-manual-field="${field.key}"]`);
    const wrapper = fieldEl?.closest('[data-manual-field-wrap]');
    if (!fieldEl || wrapper?.classList.contains('is-hidden')) continue;

    const value = String(fieldEl.value || '').trim();
    if (!value) {
      if (field.required) {
        throw new Error(`${field.label}不能为空`);
      }
      continue;
    }

    if (field.type === 'bool') {
      values[field.key] = value === 'true';
      continue;
    }
    if (field.type === 'number') {
      const numberValue = Number(value);
      if (!Number.isFinite(numberValue) || (field.step === 1 && !Number.isInteger(numberValue))) {
        throw new Error(`${field.label}必须是有效数字`);
      }
      if (field.min !== undefined && numberValue < field.min) {
        throw new Error(`${field.label}不能小于 ${field.min}`);
      }
      if (field.max !== undefined && numberValue > field.max) {
        throw new Error(`${field.label}不能大于 ${field.max}`);
      }
      values[field.key] = numberValue;
      continue;
    }
    values[field.key] = value;
  }

  const specialFields = new Set([
    'tlsMode',
    'tlsServerName',
    'tlsInsecure',
    'tlsFingerprint',
    'realityPublicKey',
    'realityShortID',
    'tlsAlpn',
    'serverPorts',
    'obfsType',
    'obfsPassword',
    'transportType',
    'transportHost',
    'transportPath',
    'grpcServiceName'
  ]);
  for (const field of fields) {
    if (!specialFields.has(field.key) && values[field.key] !== undefined) {
      node[field.key] = values[field.key];
    }
  }

  const tlsRequired = ['trojan', 'hysteria2', 'tuic'].includes(type);
  const tlsMode = values.tlsMode || (tlsRequired ? 'tls' : 'none');
  if (tlsMode === 'reality' && !values.tlsFingerprint) {
    throw new Error('Reality 必须选择 TLS 指纹');
  }
  if (tlsMode !== 'none') {
    const tls = {
      enabled: true,
      server_name: values.tlsServerName || node.server,
      insecure: Boolean(values.tlsInsecure)
    };
    if (values.tlsAlpn) {
      tls.alpn = splitCommaSeparatedValues(values.tlsAlpn);
    }
    if (values.tlsFingerprint && !['hysteria2', 'tuic'].includes(type)) {
      tls.utls = {
        enabled: true,
        fingerprint: values.tlsFingerprint
      };
    }
    if (tlsMode === 'reality') {
      tls.reality = {
        enabled: true,
        public_key: values.realityPublicKey
      };
      if (values.realityShortID) {
        tls.reality.short_id = values.realityShortID;
      }
    }
    node.tls = tls;
  }

  if (values.serverPorts) {
    node.server_ports = parseServerPorts(values.serverPorts);
  }

  if (values.obfsType) {
    node.obfs = { type: values.obfsType };
    if (values.obfsPassword) {
      node.obfs.password = values.obfsPassword;
    }
  }

  if (values.transportType && values.transportType !== 'tcp') {
    const transport = { type: values.transportType };
    if (values.transportType === 'ws') {
      transport.path = values.transportPath || '/';
      if (values.transportHost) {
        transport.headers = { Host: values.transportHost };
      }
    } else if (values.transportType === 'http') {
      transport.path = values.transportPath || '/';
      if (values.transportHost) {
        transport.host = [values.transportHost];
      }
    } else if (values.transportType === 'grpc' && values.grpcServiceName) {
      transport.service_name = values.grpcServiceName;
    }
    node.transport = transport;
  }

  return node;
}

function splitCommaSeparatedValues(value) {
  return String(value || '').split(',').map((item) => item.trim()).filter(Boolean);
}

function parseServerPorts(value) {
  return splitCommaSeparatedValues(value).map((item) => {
    const normalized = item.replace(/^(\d+)\s*-\s*(\d+)$/, '$1:$2');
    const match = normalized.match(/^(\d+)(?::(\d+))?$/);
    if (!match) {
      throw new Error(`端口跳跃格式无效：${item}`);
    }
    const start = Number(match[1]);
    const end = Number(match[2] || match[1]);
    if (start < 1 || start > 65535 || end < 1 || end > 65535 || start > end) {
      throw new Error(`端口跳跃范围无效：${item}`);
    }
    return `${start}:${end}`;
  });
}

function setStatus(message, kind = 'idle') {
  statusEl.textContent = message;
  statusEl.className = `status-bar is-${kind}`;
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

document.getElementById('back-nodes').addEventListener('click', () => {
  window.location.href = '/nodes.html';
});

document.getElementById('node-import-tab-form').addEventListener('click', () => setTab('form'));
document.getElementById('node-import-tab-raw').addEventListener('click', () => setTab('raw'));
formTypeEl.addEventListener('change', () => renderFormFields({ resetPort: true }));
formFieldsEl.addEventListener('change', (event) => {
  if (event.target instanceof HTMLElement && event.target.dataset.manualField) {
    updateConditionalFields();
  }
});

document.getElementById('add-manual-form-node').addEventListener('click', () => {
  try {
    const node = buildFormNode();
    state.manualNodes.push(node);
    renderNodeList();
    setStatus(`已添加表单节点 ${node.tag}，请记得保存`, 'success');
  } catch (error) {
    setStatus(error.message, 'error');
  }
});

document.getElementById('import-edit-manual-nodes').addEventListener('click', async () => {
  try {
    const response = await fetch('/api/nodes/import', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ raw: inputEl.value })
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data?.error?.message || '导入失败');
    }
    const normalized = normalizeImportedNodes(data.nodes || []);
    state.manualNodes.push(...normalized);
    inputEl.value = '';
    renderImportResult({ ...data, nodes: normalized });
    renderNodeList();
    setStatus(`成功导入 ${normalized.length || 0} 个节点`, 'success');
  } catch (error) {
    setStatus(error.message, 'error');
  }
});

document.getElementById('save-edit-nodes').addEventListener('click', async () => {
  try {
    const duplicateManualTag = findDuplicateTag(state.manualNodes);
    if (duplicateManualTag) {
      throw new Error(`手动节点 tag 重复：${duplicateManualTag}`);
    }
    const response = await fetch('/api/nodes', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        manualNodes: state.manualNodes,
        groups: state.groups,
        chains: state.chains,
        disabledSubscriptionTags: state.disabledSubscriptionTags
      })
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data?.error?.message || '保存失败');
    }
    localStorage.setItem(NODES_UPDATED_KEY, String(Date.now()));
    setStatus('节点配置已保存', 'success');
    await load();
  } catch (error) {
    setStatus(error.message, 'error');
  }
});

document.addEventListener('click', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLElement)) return;

  if (target.dataset.deleteManualNode) {
    state.manualNodes = state.manualNodes.filter((node) => node.tag !== target.dataset.deleteManualNode);
    renderNodeList();
    setStatus('已移除手动节点，请记得保存', 'idle');
  }

  if (target.dataset.deleteSubscriptionNode) {
    if (!state.disabledSubscriptionTags.includes(target.dataset.deleteSubscriptionNode)) {
      state.disabledSubscriptionTags.push(target.dataset.deleteSubscriptionNode);
    }
    renderNodeList();
    setStatus('已禁用订阅节点，请记得保存', 'idle');
  }

  if (target.dataset.enableSubscriptionNode) {
    state.disabledSubscriptionTags = state.disabledSubscriptionTags.filter((tag) => tag !== target.dataset.enableSubscriptionNode);
    renderNodeList();
    setStatus('已重新启用订阅节点，请记得保存', 'success');
  }
});

load().catch((error) => setStatus(error.message, 'error'));

function findDuplicateTag(nodes) {
  const seen = new Set();
  for (const node of nodes || []) {
    const tag = String(node?.tag || '').trim();
    if (!tag) continue;
    if (seen.has(tag)) return tag;
    seen.add(tag);
  }
  return '';
}

function normalizeImportedNodes(nodes) {
  const list = Array.isArray(nodes) ? nodes : [];
  return list.map((node) => normalizeSingleNode(node)).filter(Boolean);
}

function normalizeSingleNode(node) {
  if (!node || typeof node !== 'object') return null;

  const protocol = String(node.type || node.protocol || '').toLowerCase().trim();
  if (!protocol) return node;

  if (!node.type && node.protocol) {
    if (protocol === 'hysteria') {
      return normalizeV2rayHysteriaToHy2(node);
    }
    return node;
  }

  const normalized = { ...node };
  if (normalized.type === 'ss') normalized.type = 'shadowsocks';
  if (normalized.type === 'socks5') normalized.type = 'socks';
  return normalized;
}

function normalizeV2rayHysteriaToHy2(node) {
  const settings = node.settings || {};
  const streamSettings = node.streamSettings || {};
  const tlsSettings = streamSettings.tlsSettings || {};
  const hysteriaSettings = streamSettings.hysteriaSettings || {};
  const finalmask = streamSettings.finalmask || {};
  const udpMasks = Array.isArray(finalmask.udp) ? finalmask.udp : [];
  const salamander = udpMasks.find((m) => String(m?.type || '').toLowerCase() === 'salamander');
  const out = {
    type: 'hysteria2',
    tag: String(node.tag || 'hysteria2-node').trim(),
    server: String(settings.address || '').trim(),
    server_port: Number(settings.port || 0),
    password: String(hysteriaSettings.auth || '').trim()
  };

  out.tls = {
    enabled: true,
    server_name: String(tlsSettings.serverName || settings.address || '').trim(),
    insecure: Boolean(tlsSettings.allowInsecure)
  };

  const up = parseRateMbps(hysteriaSettings.up);
  const down = parseRateMbps(hysteriaSettings.down);
  if (up > 0) out.up_mbps = up;
  if (down > 0) out.down_mbps = down;

  if (salamander?.settings?.password) {
    out.obfs = {
      type: 'salamander',
      password: String(salamander.settings.password).trim()
    };
  }
  return out;
}

function parseRateMbps(value) {
  const text = String(value || '').toLowerCase().trim().replace(/mbps$/i, '').replace(/m$/i, '').trim();
  const n = Number(text);
  return Number.isFinite(n) && n > 0 ? Math.round(n) : 0;
}
