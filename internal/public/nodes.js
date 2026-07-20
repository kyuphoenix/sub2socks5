const statusEl = document.getElementById('node-status');
const groupsEl = document.getElementById('groups');
const chainsEl = document.getElementById('chains');
const availableNodeListEl = document.getElementById('available-node-list');
const groupsSectionBodyEl = document.getElementById('groups-section-body');
const chainsSectionBodyEl = document.getElementById('chains-section-body');
const toggleGroupsSectionButton = document.getElementById('toggle-groups-section');
const toggleChainsSectionButton = document.getElementById('toggle-chains-section');
const NODES_UPDATED_KEY = 'sub2socks5:nodes-updated-at';
const CHECK_BATCH_SIZE = 5;
const GROUP_TEST_URL_PRESETS = [
  'https://www.gstatic.com/generate_204',
  'https://www.google.com/generate_204',
  'https://cp.cloudflare.com/generate_204'
];

let state = {
  subscriptionNodes: [],
  manualNodes: [],
  groups: [],
  chains: [],
  availableOutbounds: [],
  fallbackStates: {}
};

let nodeDelayState = {};
const checkingNodeTags = new Set();
const expandedGroups = new Set();
const expandedChains = new Set();
let groupsSectionCollapsed = false;
let chainsSectionCollapsed = false;

async function load() {
  const response = await fetch('/api/nodes');
  const data = await response.json();
  if (!response.ok) {
    throw new Error(data?.error?.message || '加载节点失败');
  }
  state = {
    ...state,
    ...data,
    chains: Array.isArray(data.chains) ? data.chains : [],
    fallbackStates: data.fallbackStates || {}
  };
  render();
}

function render() {
  renderAvailableNodes();
  renderGroups();
  renderChains();
  renderSectionToggles();
}

function renderSectionToggles() {
  groupsSectionBodyEl.classList.toggle('is-hidden', groupsSectionCollapsed);
  chainsSectionBodyEl.classList.toggle('is-hidden', chainsSectionCollapsed);
  toggleGroupsSectionButton.textContent = groupsSectionCollapsed ? '展开' : '收起';
  toggleChainsSectionButton.textContent = chainsSectionCollapsed ? '展开' : '收起';
}

function renderAvailableNodes() {
  const visibleNodes = getSelectableNodes();
  availableNodeListEl.innerHTML = '';

  if (!visibleNodes.length) {
    availableNodeListEl.innerHTML = '<div class="timeline-item"><div class="title">暂无节点</div></div>';
    return;
  }

  for (const node of visibleNodes) {
    const delayState = nodeDelayState[node.tag];
    const isChecking = checkingNodeTags.has(node.tag) || delayState?.loading;
    const delayText = isChecking ? '测试中...' : delayState?.text || '延迟';
    const card = document.createElement('div');
    card.className = 'node-pill node-pill-checkable';
    card.innerHTML = `
      <div class="node-pill-main">
        <div class="node-pill-title">${escapeHtml(node.tag || '')}</div>
        <div class="node-pill-tags">
          <span class="node-pill-tag">${escapeHtml(node.type || '')}</span>
          <span class="node-pill-tag is-source">${escapeHtml(sourceLabel(node.source))}</span>
        </div>
      </div>
      <button type="button" class="node-check-button ${isChecking ? 'is-loading' : ''}" data-check-node="${escapeHtmlAttr(node.tag)}" title="点击测试延迟">${escapeHtml(delayText)}</button>
    `;
    availableNodeListEl.appendChild(card);
  }
}

function renderGroups() {
  groupsEl.innerHTML = '';
  if (!state.groups.length) {
    groupsEl.innerHTML = '<div class="timeline-item"><div class="title">暂无节点组</div></div>';
    return;
  }

  const selectableNodes = getSelectableNodesWithoutChains();
  for (const [index, group] of state.groups.entries()) {
    groupsEl.appendChild(buildGroupPanel(index, group, selectableNodes));
  }
}

function buildGroupPanel(index, group, selectableNodes) {
  const fallbackState = state.fallbackStates?.[group.tag] || null;
  const groupTitle = group.tag || `节点组 ${index + 1}`;
  const expanded = expandedGroups.has(index);
  const selectedMembers = Array.isArray(group.members) ? group.members : [];
  const summaryCards = selectedMembers.map((memberTag) => {
    const node = selectableNodes.find((item) => item.tag === memberTag);
    return node ? renderNodePill(node) : '';
  }).join('');

  const statusHtml = group.strategy === 'fallback'
    ? `
      <div class="kv-grid">
        <div class="kv-item">
          <div class="key">当前活动节点</div>
          <div class="value">${escapeHtml(fallbackState?.current || group.members?.[0] || '')}</div>
        </div>
        <div class="kv-item">
          <div class="key">最近切换时间</div>
          <div class="value">${escapeHtml(fallbackState?.updatedAt || '')}</div>
        </div>
      </div>
    `
    : '';

  const item = document.createElement('div');
  item.className = 'timeline-item group-panel';
  item.innerHTML = `
    <div class="group-panel-header">
      <div>
        <div class="title">${escapeHtml(groupTitle)}</div>
        <div class="node-pill-tags">
          <span class="node-pill-tag">${escapeHtml(group.strategy || 'urltest')}</span>
          <span class="node-pill-tag is-source">${selectedMembers.length} 个节点</span>
        </div>
      </div>
      <button type="button" class="group-toggle" data-toggle-group="${index}">${expanded ? '收起' : '展开'}</button>
    </div>
    <div class="node-pill-grid group-summary">
      ${summaryCards || '<div class="timeline-item"><div class="title">暂无成员</div></div>'}
    </div>
    <div class="group-panel-body ${expanded ? '' : 'is-hidden'}">
      <div class="form-grid">
        <label><span>tag</span><input data-kind="group" data-index="${index}" data-field="tag" value="${escapeHtmlAttr(group.tag || '')}" /></label>
        <label>
          <span>策略</span>
          <select data-kind="group" data-index="${index}" data-field="strategy">
            <option value="urltest" ${group.strategy === 'urltest' ? 'selected' : ''}>urltest</option>
            <option value="fallback" ${group.strategy === 'fallback' ? 'selected' : ''}>fallback</option>
          </select>
        </label>
        <label>
          <span>测试地址</span>
          <select data-kind="group-preset" data-index="${index}">
            ${buildGroupUrlPresetOptions(group.url)}
          </select>
        </label>
        <label><span>测试间隔</span><input data-kind="group" data-index="${index}" data-field="interval" value="${escapeHtmlAttr(group.interval || '10m')}" /></label>
        <label><span>超时毫秒</span><input data-kind="group" data-index="${index}" data-field="timeoutMs" type="number" value="${escapeHtmlAttr(String(group.timeoutMs || 5000))}" /></label>
        <label class="${GROUP_TEST_URL_PRESETS.includes(group.url) ? 'is-hidden' : ''}"><span>自定义测试地址</span><input data-kind="group" data-index="${index}" data-field="url" value="${escapeHtmlAttr(group.url || 'https://www.gstatic.com/generate_204')}" /></label>
      </div>
      ${statusHtml}
      <div class="member-selector">${renderGroupMembers(index, group, selectableNodes)}</div>
      <div class="section-heading-actions">
        <button type="button" data-remove-group="${index}">删除</button>
      </div>
    </div>
  `;
  return item;
}

function renderChains() {
  chainsEl.innerHTML = '';
  if (!state.chains.length) {
    chainsEl.innerHTML = '<div class="timeline-item"><div class="title">暂无链式代理</div></div>';
    return;
  }

  const selectableNodes = getSelectableNodesWithoutChains();
  for (const [index, chain] of state.chains.entries()) {
    const expanded = expandedChains.has(index);
    const selectedMembers = Array.isArray(chain.members) ? chain.members : [];
    const chainCards = selectedMembers.map((memberTag, memberIndex) => {
      const node = selectableNodes.find((item) => item.tag === memberTag);
      const card = node ? renderNodePill(node) : '';
      const arrow = memberIndex < selectedMembers.length - 1 ? '<div class="chain-arrow">→</div>' : '';
      return `<div class="chain-segment">${card}${arrow}</div>`;
    }).join('');

    const item = document.createElement('div');
    item.className = 'timeline-item group-panel';
    item.innerHTML = `
      <div class="group-panel-header">
        <div>
          <div class="title">${escapeHtml(chain.tag || `链式代理 ${index + 1}`)}</div>
          <div class="node-pill-tags">
            <span class="node-pill-tag">chain</span>
            <span class="node-pill-tag is-source">${selectedMembers.length} 个节点</span>
          </div>
        </div>
        <button type="button" class="group-toggle" data-toggle-chain="${index}">${expanded ? '收起' : '展开'}</button>
      </div>
      <div class="chain-summary">${chainCards || '<div class="timeline-item"><div class="title">暂无成员</div></div>'}</div>
      <div class="group-panel-body ${expanded ? '' : 'is-hidden'}">
        <div class="form-grid">
          <label><span>名称</span><input data-kind="chain" data-index="${index}" data-field="tag" value="${escapeHtmlAttr(chain.tag || '')}" /></label>
        </div>
        <div class="member-selector">${renderChainMembers(index, chain, selectableNodes)}</div>
        <div class="section-heading-actions">
          <button type="button" data-remove-chain="${index}">删除</button>
        </div>
      </div>
    `;
    chainsEl.appendChild(item);
  }
}

function renderGroupMembers(index, group, selectableNodes) {
  const selected = Array.isArray(group.members) ? group.members : [];
  const rows = selected.map((memberTag, memberIndex) => `
    <div class="member-row">
      <select data-group-member-select="${index}" data-member-index="${memberIndex}">
        ${buildMemberOptions(selectableNodes, selected, memberTag)}
      </select>
      <button type="button" class="member-remove" data-remove-member="${index}" data-member-index="${memberIndex}">删除</button>
    </div>
  `);
  const remaining = selectableNodes.filter((node) => !selected.includes(node.tag));
  rows.push(`<button type="button" class="member-add" data-add-member="${index}" ${remaining.length ? '' : 'disabled'}>+ 添加节点</button>`);
  return rows.join('');
}

function renderChainMembers(index, chain, selectableNodes) {
  const selected = Array.isArray(chain.members) ? chain.members : [];
  const rows = selected.map((memberTag, memberIndex) => `
    <div class="member-row">
      <select data-chain-member-select="${index}" data-member-index="${memberIndex}">
        ${buildMemberOptions(selectableNodes, selected, memberTag)}
      </select>
      <button type="button" class="member-remove" data-remove-chain-member="${index}" data-member-index="${memberIndex}">删除</button>
    </div>
  `);
  const remaining = selectableNodes.filter((node) => !selected.includes(node.tag));
  rows.push(`<button type="button" class="member-add" data-add-chain-member="${index}" ${remaining.length ? '' : 'disabled'}>+ 添加节点</button>`);
  return rows.join('');
}

function buildMemberOptions(selectableNodes, selectedTags, currentTag) {
  return selectableNodes
    .filter((node) => node.tag === currentTag || !selectedTags.includes(node.tag))
    .map((node) => `<option value="${escapeHtmlAttr(node.tag)}" ${node.tag === currentTag ? 'selected' : ''}>${escapeHtml(node.label || node.tag)}</option>`)
    .join('');
}

function buildGroupUrlPresetOptions(currentUrl) {
  const preset = GROUP_TEST_URL_PRESETS.includes(currentUrl) ? currentUrl : 'custom';
  return [
    ...GROUP_TEST_URL_PRESETS.map((url) => `<option value="${escapeHtmlAttr(url)}" ${preset === url ? 'selected' : ''}>${escapeHtml(url)}</option>`),
    `<option value="custom" ${preset === 'custom' ? 'selected' : ''}>自定义</option>`
  ].join('');
}

function getSelectableNodesWithoutChains() {
  return state.availableOutbounds.filter((item) => !['proxy', 'auto', 'block', 'direct'].includes(item.tag) && item.source !== 'chain');
}

function getSelectableNodes() {
  return state.availableOutbounds.filter((item) => !['proxy', 'auto', 'block', 'direct'].includes(item.tag));
}

function renderNodePill(node) {
  return `
    <div class="node-pill">
      <div class="node-pill-title">${escapeHtml(node.tag || '')}</div>
      <div class="node-pill-tags">
        <span class="node-pill-tag">${escapeHtml(node.type || '')}</span>
        <span class="node-pill-tag is-source">${escapeHtml(sourceLabel(node.source))}</span>
      </div>
    </div>
  `;
}

function sourceLabel(source) {
  if (source === 'subscription') return '订阅';
  if (source === 'manual') return '手动';
  if (source === 'group') return '节点组';
  if (source === 'chain') return '链式代理';
  if (source === 'builtin') return '内置';
  return source || '';
}

async function checkNode(tag) {
  if (checkingNodeTags.has(tag)) return;
  checkingNodeTags.add(tag);
  nodeDelayState[tag] = { loading: true, text: '测试中...' };
  renderAvailableNodes();
  try {
    const runtimeWasRunning = await ensureRuntimeRunning();
    const response = await fetch('/api/nodes/check', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ tags: [tag] })
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data?.error?.message || '延迟测试失败');
    }
    const result = data.results?.[tag];
    nodeDelayState[tag] = result?.ok
      ? {
          text: result.text,
          checkedAt: result.checkedAt,
          checkedTag: result.checkedTag
        }
      : { text: '失败', error: result?.error || '延迟测试失败' };
    setStatus(`节点 ${tag} 延迟测试完成`, 'success');
    if (!runtimeWasRunning) {
      await stopRuntime();
    }
  } catch (error) {
    nodeDelayState[tag] = { text: '失败', error: error.message };
    setStatus(error.message, 'error');
    try {
      await stopRuntimeIfNeeded();
    } catch {}
  } finally {
    checkingNodeTags.delete(tag);
    renderAvailableNodes();
  }
}

async function checkAllNodes() {
  const tags = getSelectableNodes().map((item) => item.tag);
  if (!tags.length) {
    setStatus('当前没有可测试延迟的节点', 'idle');
    return;
  }

  const runtimeWasRunning = await ensureRuntimeRunning();
  setStatus('正在分批测试全部节点延迟...', 'loading');
  for (const tag of tags) {
    nodeDelayState[tag] = { loading: true, text: '测试中...' };
  }
  renderAvailableNodes();

  let hasError = false;
  try {
    for (let index = 0; index < tags.length; index += CHECK_BATCH_SIZE) {
      const batch = tags.slice(index, index + CHECK_BATCH_SIZE);
      const response = await fetch('/api/nodes/check', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ tags: batch })
      });
      const data = await response.json();
      if (!response.ok) {
        throw new Error(data?.error?.message || '批量延迟测试失败');
      }
      for (const tag of batch) {
        const result = data.results?.[tag];
        nodeDelayState[tag] = result?.ok
          ? { text: result.text, checkedAt: result.checkedAt, checkedTag: result.checkedTag }
          : { text: '失败', error: result?.error || '延迟测试失败' };
        if (!result?.ok) hasError = true;
      }
      renderAvailableNodes();
    }
  } catch (error) {
    hasError = true;
    setStatus(error.message, 'error');
  } finally {
    if (!runtimeWasRunning) {
      await stopRuntime();
    }
  }

  if (hasError) {
    setStatus('部分节点延迟测试失败，请重试', 'error');
    return;
  }
  setStatus('全部节点延迟测试完成', 'success');
}

async function ensureRuntimeRunning() {
  const configResponse = await fetch('/api/config');
  const configData = await configResponse.json();
  if (!configResponse.ok) {
    throw new Error(configData?.error?.message || '读取运行状态失败');
  }
  const runtimeWasRunning = Boolean(configData?.runtime?.running);
  if (!runtimeWasRunning) {
    const startResponse = await fetch('/api/runtime/start', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: '{}'
    });
    const startData = await startResponse.json();
    if (!startResponse.ok) {
      throw new Error(startData?.error?.message || '启动 sing-box 失败');
    }
    await waitForRuntimeReady();
  }
  return runtimeWasRunning;
}

async function stopRuntime() {
  await fetch('/api/runtime/stop', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: '{}'
  });
}

async function stopRuntimeIfNeeded() {
  const response = await fetch('/api/config');
  const data = await response.json();
  if (!response.ok) return;
  if (data?.runtime?.running) {
    await stopRuntime();
  }
}

async function waitForRuntimeReady(timeoutMs = 8000) {
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    const response = await fetch('/api/config');
    const data = await response.json();
    if (response.ok && data?.runtime?.running) {
      await sleep(700);
      return;
    }
    await sleep(250);
  }
  throw new Error('sing-box 启动超时，请检查内核日志');
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
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

document.getElementById('back-home').addEventListener('click', () => {
  window.location.href = '/';
});

document.getElementById('open-node-editor').addEventListener('click', () => {
  window.location.href = '/nodes-edit.html';
});

document.getElementById('check-all-nodes').addEventListener('click', () => {
  checkAllNodes().catch((error) => setStatus(error.message, 'error'));
});

document.getElementById('add-group').addEventListener('click', () => {
  state.groups.push({ tag: '', strategy: 'urltest', url: 'https://www.gstatic.com/generate_204', interval: '10m', timeoutMs: 5000, members: [] });
  expandedGroups.add(state.groups.length - 1);
  renderGroups();
});

document.getElementById('add-chain').addEventListener('click', () => {
  state.chains.push({ tag: '', members: [] });
  expandedChains.add(state.chains.length - 1);
  renderChains();
});

toggleGroupsSectionButton.addEventListener('click', () => {
  groupsSectionCollapsed = !groupsSectionCollapsed;
  renderSectionToggles();
});

toggleChainsSectionButton.addEventListener('click', () => {
  chainsSectionCollapsed = !chainsSectionCollapsed;
  renderSectionToggles();
});

document.getElementById('save-nodes').addEventListener('click', async () => {
  try {
    const duplicateGroupTag = findDuplicateTag(state.groups);
    if (duplicateGroupTag) {
      throw new Error(`节点组 tag 重复：${duplicateGroupTag}`);
    }
    const duplicateChainTag = findDuplicateTag(state.chains);
    if (duplicateChainTag) {
      throw new Error(`链式代理 tag 重复：${duplicateChainTag}`);
    }

    const response = await fetch('/api/nodes', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        manualNodes: state.manualNodes,
        groups: state.groups,
        chains: state.chains,
        disabledSubscriptionTags: state.disabledSubscriptionTags || []
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

document.addEventListener('input', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLInputElement || target instanceof HTMLSelectElement)) return;

  if (target.dataset.kind === 'group') {
    const index = Number(target.dataset.index);
    const field = target.dataset.field;
    state.groups[index][field] = field === 'timeoutMs' ? Number(target.value || 0) : target.value;
  }

  if (target.dataset.kind === 'group-preset') {
    const index = Number(target.dataset.index);
    if (target.value !== 'custom') {
      state.groups[index].url = target.value;
    }
    renderGroups();
  }

  if (target.dataset.kind === 'chain') {
    const index = Number(target.dataset.index);
    const field = target.dataset.field;
    state.chains[index][field] = target.value;
  }

  if (target.dataset.groupMemberSelect) {
    const groupIndex = Number(target.dataset.groupMemberSelect);
    const memberIndex = Number(target.dataset.memberIndex);
    state.groups[groupIndex].members[memberIndex] = target.value;
    renderGroups();
  }

  if (target.dataset.chainMemberSelect) {
    const chainIndex = Number(target.dataset.chainMemberSelect);
    const memberIndex = Number(target.dataset.memberIndex);
    state.chains[chainIndex].members[memberIndex] = target.value;
    renderChains();
  }
});

document.addEventListener('click', (event) => {
  const target = event.target;
  if (!(target instanceof HTMLElement)) return;

  if (target.dataset.toggleGroup) {
    const groupIndex = Number(target.dataset.toggleGroup);
    expandedGroups.has(groupIndex) ? expandedGroups.delete(groupIndex) : expandedGroups.add(groupIndex);
    renderGroups();
  }

  if (target.dataset.toggleChain) {
    const chainIndex = Number(target.dataset.toggleChain);
    expandedChains.has(chainIndex) ? expandedChains.delete(chainIndex) : expandedChains.add(chainIndex);
    renderChains();
  }

  if (target.dataset.removeGroup) {
    const index = Number(target.dataset.removeGroup);
    state.groups.splice(index, 1);
    renderGroups();
  }

  if (target.dataset.removeChain) {
    const index = Number(target.dataset.removeChain);
    state.chains.splice(index, 1);
    renderChains();
  }

  if (target.dataset.addMember) {
    const groupIndex = Number(target.dataset.addMember);
    const selectableNodes = getSelectableNodesWithoutChains();
    const selected = new Set(state.groups[groupIndex].members || []);
    const nextNode = selectableNodes.find((node) => !selected.has(node.tag));
    if (nextNode) {
      state.groups[groupIndex].members.push(nextNode.tag);
      renderGroups();
    }
  }

  if (target.dataset.addChainMember) {
    const chainIndex = Number(target.dataset.addChainMember);
    const selectableNodes = getSelectableNodesWithoutChains();
    const selected = new Set(state.chains[chainIndex].members || []);
    const nextNode = selectableNodes.find((node) => !selected.has(node.tag));
    if (nextNode) {
      state.chains[chainIndex].members.push(nextNode.tag);
      renderChains();
    }
  }

  if (target.dataset.removeMember) {
    const groupIndex = Number(target.dataset.removeMember);
    const memberIndex = Number(target.dataset.memberIndex);
    state.groups[groupIndex].members.splice(memberIndex, 1);
    renderGroups();
  }

  if (target.dataset.removeChainMember) {
    const chainIndex = Number(target.dataset.removeChainMember);
    const memberIndex = Number(target.dataset.memberIndex);
    state.chains[chainIndex].members.splice(memberIndex, 1);
    renderChains();
  }

  if (target.dataset.checkNode) {
    checkNode(target.dataset.checkNode).catch((error) => setStatus(error.message, 'error'));
  }
});

load().catch((error) => setStatus(error.message, 'error'));

function findDuplicateTag(items) {
  const seen = new Set();
  for (const item of items || []) {
    const tag = String(item?.tag || '').trim();
    if (!tag) continue;
    if (seen.has(tag)) return tag;
    seen.add(tag);
  }
  return '';
}
