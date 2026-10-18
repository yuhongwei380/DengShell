'use strict';
// Pinned connections bar: pin the current session (saving a quick-connect
// session first) and it appears in the top bar for one-click reconnect.
// The id list lives in appearance.layout under one key, so no backend change.
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
  function render() {
    if (!bar) return;
    readPinned();
    const items = pinned.map(id => connectionProfile(id)).filter(Boolean);
    bar.hidden = !items.length;
    bar.replaceChildren(...items.map(profile => {
      const chip = node('button', 'pinned-chip');
      chip.type = 'button';
      chip.append(icon('server'), node('span', '', profile.name));
      chip.title = `${profile.name} · ${profile.user}@${profile.host}:${profile.port}\n左键快速连接 · 右键取消固定`;
      chip.onclick = () => {
        const existing = [...sessions.values()].find(s => s.profileId === profile.id);
        if (existing) activate(existing.id); else connect(profile.id);
      };
      chip.oncontextmenu = event => { event.preventDefault(); safe(() => unpin(profile.id))(); };
      return chip;
    }));
  }
  async function pin(id) { pinned = [...pinned, id]; await writePinned(); render(); reflect(); }
  async function unpin(id) { pinned = pinned.filter(x => x !== id); await writePinned(); render(); reflect(); toast('已从导航栏取消固定'); }
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
    }
    await pin(profile.id);
    toast('已固定到顶部导航栏');
  }
  function reflect() {
    if (!pinButton) return;
    const state = current(), profile = profileFor(state);
    pinButton.hidden = !state || !profile;
    if (!state || !profile) return;
    const on = isPinned(profile.id);
    pinButton.setAttribute('aria-pressed', String(on));
    pinButton.classList.toggle('active', on);
    pinButton.title = on ? '从顶部导航栏取消固定' : temporaryProfiles.has(profile.id) ? '保存并固定到顶部导航栏' : '固定到顶部导航栏';
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
