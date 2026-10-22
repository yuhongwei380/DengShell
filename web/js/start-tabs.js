'use strict';
// Start pages are frontend tabs, never fake backend sessions to poll or close.
window.DengStartTabs = (() => {
  const pages = new Map();
  let sequence = 0;
  function activatePage(id) {
    if (id !== null && !pages.has(id)) return;
    if (activeID !== id) window.DengLocalPrompt?.reset();
    window.DengRDP?.deactivate();
    window.DengProcessView?.activate(null, 'terminal');
    activeID = id;
    $('#file-filter').value = '';
    for (const state of sessions.values()) state.host.hidden = true;
    window.dispatchEvent(new CustomEvent('dengshell:session-context-change'));
    renderTabs(); renderSessionInfo(); renderFiles();
    requestAnimationFrame(() => {
      if (activeID !== id) return;
      $('#session-tabs .session-tab.active')?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
      $('#command-input').focus({ preventScroll: true });
    });
  }
  function create() {
    const page = { id: `start:${crypto.randomUUID()}`, name: `新选项卡 ${++sequence}`, tabOrder: nextSessionOrder++ };
    pages.set(page.id, page);
    activatePage(page.id);
    return page;
  }
  function close(id) {
    if (!pages.has(id)) return;
    const tabs = [...$('#session-tabs').children];
    const index = tabs.findIndex(tab => tab.dataset.startTabId === id);
    const neighbor = tabs[index + 1] || tabs[index - 1];
    pages.delete(id);
    if (activeID === id) {
      if (neighbor) neighbor.querySelector('[role="tab"]').click();
      else activatePage(null);
    } else renderTabs();
  }
  function appendTabs(host) {
    for (const page of pages.values()) {
      const selected = activeID === page.id && !window.DengRDP?.active();
      const tab = node('div', `session-tab start-tab${selected ? ' active' : ''}`);
      tab.dataset.startTabId = page.id;
      tab.dataset.tabOrder = page.tabOrder;
      const button = node('button'); button.type = 'button';
      button.setAttribute('role', 'tab'); button.setAttribute('aria-selected', String(selected));
      button.title = page.name;
      button.append(node('span', 'status-dot'), node('span', 'session-tab-label', page.name));
      button.onclick = () => activatePage(page.id);
      const remove = node('button', 'tab-close'); remove.type = 'button';
      remove.title = '关闭选项卡'; remove.setAttribute('aria-label', `关闭 ${page.name}`);
      remove.append(icon('close')); remove.onclick = () => close(page.id);
      tab.append(button, remove); host.append(tab);
    }
  }
  // Foreground opens replace their start page, preserving its dragged order.
  function take(id = activeID, replacementKey = null) {
    const page = pages.get(id);
    if (!page) return null;
    pages.delete(id);
    window.DengTabOrder.replace(`start:${id}`, replacementKey || `ssh:order:${page.tabOrder}`);
    return page;
  }
  return { create, activate: activatePage, appendTabs, close, take, has: id => pages.has(id), first: () => pages.keys().next().value };
})();
