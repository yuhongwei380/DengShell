'use strict';
// Pinned connections bar: pin the current session (saving a quick-connect
// session first) and it appears in the top bar for one-click reconnect.
// The id list lives in appearance.layout under one key, whitelisted by the
// backend layout validator (validSavedLayoutKey in preferences.go).
window.DengPinnedNav = (() => {
  const LAYOUT_KEY = 'dengshell.pinned.v1';
  let bar, pinButton, pinned = [];
  function readPinned() {
    const raw = appearance.layout?.[LAYOUT_KEY];
    pinned = Array.isArray(raw) ? raw.filter(id => typeof id === 'string') : [];
  }
  function isPinned(id) { return pinned.includes(id); }
  async function writePinned() {
    await persistAppearance({ layout: { [LAYOUT_KEY]: pinned } });
  }
  const MAX_CHIPS = 7;
  function openConnection(profile) {
    const existing = [...sessions.values()].find(s => s.profileId === profile.id);
    if (existing) activate(existing.id); else connect(profile.id);
  }
  function chipFor(profile) {
    const chip = node('button', 'pinned-chip');
    chip.type = 'button';
    chip.append(icon('server'), node('span', '', profile.name));
    chip.title = `${profile.name} · ${profile.user}@${profile.host}:${profile.port}\n左键快速连接 · 右键取消固定`;
    chip.onclick = () => openConnection(profile);
    chip.oncontextmenu = event => { event.preventDefault(); safe(() => unpin(profile.id))(); };
    return chip;
  }
  function moreFor(overflow) {
    const wrap = node('span', 'pinned-more-wrap');
    const btn = node('button', 'pinned-more');
    btn.type = 'button';
    btn.title = `还有 ${overflow.length} 个固定连接`;
    btn.setAttribute('aria-label', `展开其余 ${overflow.length} 个固定连接`);
    btn.append(icon('chevron'));
    const menu = node('div', 'pinned-more-menu');
    menu.hidden = true;
    overflow.forEach(profile => {
      const item = node('button', 'pinned-more-item');
      item.type = 'button';
      item.append(icon('server'), node('span', '', profile.name));
      item.title = `${profile.name} · ${profile.user}@${profile.host}:${profile.port}\n左键快速连接 · 右键取消固定`;
      item.onclick = () => { menu.hidden = true; openConnection(profile); };
      item.oncontextmenu = event => { event.preventDefault(); menu.hidden = true; safe(() => unpin(profile.id))(); };
      menu.append(item);
    });
    btn.onclick = event => {
      event.stopPropagation();
      if (menu.hidden) {
        const r = btn.getBoundingClientRect();
        menu.style.left = Math.max(8, Math.min(r.left, innerWidth - 210)) + 'px';
        menu.style.top = (r.bottom + 7) + 'px';
      }
      menu.hidden = !menu.hidden;
      btn.classList.toggle('open', !menu.hidden);
    };
    document.addEventListener('click', event => { if (!menu.hidden && !wrap.contains(event.target)) { menu.hidden = true; btn.classList.remove('open'); } });
    wrap.append(btn, menu);
    return wrap;
  }
  function render() {
    if (!bar) return;
    readPinned();
    const items = pinned.map(id => connectionProfile(id)).filter(Boolean);
    bar.hidden = false;
    const label = node('span', 'pinned-label');
    label.append(icon('bookmark'), node('span', '', '链接栏'));
    const visible = items.slice(0, MAX_CHIPS);
    const overflow = items.slice(MAX_CHIPS);
    bar.replaceChildren(label, ...visible.map(chipFor), ...(overflow.length ? [moreFor(overflow)] : []));
  }
  async function pin(id) { pinned = [...pinned, id]; await writePinned(); render(); reflect(); }
  async function unpin(id) { pinned = pinned.filter(x => x !== id); await writePinned(); render(); reflect(); toast('已从链接栏取消固定'); }
  async function toggleCurrent() {
    const state = current(), profile = profileFor(state);
    if (!state || !profile) return;
    if (isPinned(profile.id)) { await unpin(profile.id); return; }
    if (temporaryProfiles.has(profile.id)) {
      // Pinning a quick-connect session persists it first, like "保存连接".
      const saved = await post(`/api/quick-connect/${profile.id}/save`, { name: profile.name, group: '快速连接', notes: '', remember: false, secret: '' });
      temporaryProfiles.delete(profile.id);
      profiles = profiles.filter(p => p.id !== saved.id);
      profiles.push(saved);
      renderConnections(); renderTabs();
      // The session is no longer temporary even if pinning below fails, so
      // refresh the "保存连接" button instead of leaving it visibly dead.
      window.DengQuickConnect?.reflect();
    }
    await pin(profile.id);
    toast('已固定到链接栏');
  }
  function reflect() {
    if (!pinButton) return;
    const state = current(), profile = profileFor(state);
    pinButton.hidden = !state || !profile;
    if (!state || !profile) return;
    const on = isPinned(profile.id);
    pinButton.setAttribute('aria-pressed', String(on));
    pinButton.classList.toggle('active', on);
    pinButton.title = on ? '从链接栏取消固定' : temporaryProfiles.has(profile.id) ? '保存并固定到链接栏' : '固定到链接栏';
  }
  document.addEventListener('DOMContentLoaded', () => {
    bar = $('#pinned-bar');
    pinButton = node('button', 'terminal-popout');
    pinButton.id = 'pin-session';
    pinButton.hidden = true;
    pinButton.append(icon('bookmark'), node('span', '', '固定'));
    pinButton.onclick = () => safe(toggleCurrent)();
    $('#toggle-sftp').before(pinButton);
    render(); reflect();
  }, { once: true });
  return { render, reflect };
})();
