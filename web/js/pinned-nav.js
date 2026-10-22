'use strict';
// Pinned connections bar: pin the current session (saving a quick-connect
// session first) and it appears in the top bar; each click opens a fresh
// session tab, like double-clicking the server list.
// The id list lives in appearance.layout under one key, whitelisted by the
// backend layout validator (validSavedLayoutKey in preferences.go).
window.DengPinnedNav = (() => {
  const LAYOUT_KEY = 'dengshell.pinned.v1';
  let bar, pinButton, pinned = [], overflowControl, layoutChips = () => {};
  function readPinned() {
    const raw = appearance.layout?.[LAYOUT_KEY];
    pinned = Array.isArray(raw) ? raw.filter(id => typeof id === 'string') : [];
  }
  function isPinned(id) { return pinned.includes(id); }
  async function writePinned() {
    await persistAppearance({ layout: { [LAYOUT_KEY]: pinned } });
  }
  const profileSubtitle = profile => profile.protocol === 'serial' ? window.DengSerial.label(profile) : `${profile.user}@${profile.host}:${profile.port}`;
  function openConnection(profile) {
    // Same semantics as double-clicking in the server list: every click opens a
    // fresh session tab, even when a session for this profile already exists
    // (activating the old one made repeat clicks look dead). connect() coalesces
    // only in-flight requests, so accidental double clicks cannot stack up.
    connect(profile.id);
  }
  function removeButtonFor(profile) {
    const remove = node('button', 'pinned-chip-remove');
    remove.type = 'button';
    remove.append(icon('close'));
    remove.title = '关闭（取消固定）';
    remove.setAttribute('aria-label', `取消固定 ${profile.name}`);
    remove.onclick = event => { event.stopPropagation(); safe(() => confirmUnpin(profile))(); };
    return remove;
  }
  // Unpinning is destructive enough to ask first: the close button is the only
  // unpin affordance and always routes through the shared confirmation dialog.
  async function confirmUnpin(profile) {
    if (!await ask({ title: `取消固定 ${profile.name}？`, description: `将从链接栏移除 ${profile.name}（${profileSubtitle(profile)}）。连接配置与已保存数据不受影响，之后可重新固定。`, confirm: '取消固定' })) return;
    await unpin(profile.id);
  }
  // Right-click renames in place: the name is the same field the server
  // manager edits, so the saved profile is re-posted with only the name
  // changed. Secrets and proxy passwords round-trip through the public view
  // untouched (store.Save keeps them while the auth shape is unchanged).
  async function renameProfile(profile) {
    const answer = await ask({ title: '重命名连接', description: `修改「${profile.name}」的名称，与服务器管理里该连接的名称同步。`, input: true, value: profile.name, confirm: '保存' });
    if (answer === null) return;
    const name = String(answer).trim();
    if (!name) { toast('名称不能为空'); return; }
    if (name === profile.name) return;
    const saved = await post('/api/profiles', { ...profile, name, clearSecret: false });
    const index = profiles.findIndex(entry => entry.id === saved.id);
    if (index >= 0) profiles[index] = saved; else profiles.push(saved);
    renderConnections(); renderTabs(); renderSessionInfo();
    render(); reflect();
    toast(`已重命名为「${saved.name}」`);
  }
  const chipTitle = profile => `${profile.name} · ${profileSubtitle(profile)}\n左键新建会话 · 右键重命名 · × 关闭（取消固定需确认）`;
  function chipFor(profile) {
    const chip = node('span', 'pinned-chip');
    const open = node('button', 'pinned-chip-open');
    open.type = 'button';
    open.append(icon('server'), node('span', '', profile.name));
    open.onclick = () => openConnection(profile);
    chip.oncontextmenu = event => { event.preventDefault(); safe(() => renameProfile(profile))(); };
    chip.append(open, removeButtonFor(profile));
    chip.title = chipTitle(profile);
    return chip;
  }
  function moreFor() {
    const wrap = node('span', 'pinned-more-wrap');
    const btn = node('button', 'pinned-more');
    btn.type = 'button';
    btn.textContent = '︙';
    btn.setAttribute('aria-haspopup', 'true');
    btn.setAttribute('aria-expanded', 'false');
    btn.setAttribute('aria-controls', 'pinned-more-menu');
    const menu = node('div', 'pinned-more-menu');
    menu.id = 'pinned-more-menu';
    menu.hidden = true;
    const close = () => { menu.hidden = true; btn.classList.remove('open'); btn.setAttribute('aria-expanded', 'false'); };
    const update = overflow => {
      close();
      btn.title = `还有 ${overflow.length} 个固定连接`;
      btn.setAttribute('aria-label', `展开其余 ${overflow.length} 个固定连接`);
      menu.replaceChildren();
      overflow.forEach(profile => {
        const item = node('span', 'pinned-more-item');
        const open = node('button', 'pinned-more-item-open');
        open.type = 'button';
        open.append(icon('server'), node('span', '', profile.name));
        open.onclick = () => { close(); openConnection(profile); };
        item.oncontextmenu = event => { event.preventDefault(); safe(() => renameProfile(profile))(); };
        item.append(open, removeButtonFor(profile));
        item.title = chipTitle(profile);
        menu.append(item);
      });
    };
    btn.onclick = event => {
      event.stopPropagation();
      if (menu.hidden) {
        const r = btn.getBoundingClientRect();
        menu.hidden = false;
        const top = r.bottom / effectiveScale + 7;
        menu.style.left = Math.max(8, Math.min(r.left / effectiveScale, logicalWidth() - menu.offsetWidth - 8)) + 'px';
        menu.style.top = top + 'px';
        menu.style.maxHeight = Math.min(320, Math.max(0, logicalHeight() - top - 8)) + 'px';
        btn.classList.add('open');
        btn.setAttribute('aria-expanded', 'true');
      } else {
        close();
      }
    };
    wrap.append(btn, menu);
    return { wrap, btn, menu, close, update };
  }
  function render() {
    if (!bar) return;
    readPinned();
    const items = pinned.map(id => connectionProfile(id)).filter(Boolean);
    bar.hidden = false;
    const label = node('span', 'pinned-label');
    label.append(icon('bookmark'), node('span', '', '链接栏'));
    const row = node('div', 'pinned-links'), chips = items.map(chipFor);
    overflowControl?.close();
    const more = moreFor();
    overflowControl = more;
    row.append(label, ...chips, more.wrap);
    bar.replaceChildren(row);
    let previousCount = -1;
    layoutChips = () => {
      more.close();
      label.hidden = false;
      more.wrap.hidden = false;
      chips.forEach(chip => { chip.hidden = false; });
      // The outer grid item always spans its available track; the highlighted
      // row remains compact. Measuring the shrinking row itself would prevent
      // chips from returning when the window or sidebar grows again.
      const style = getComputedStyle(row), gap = parseFloat(style.columnGap) || 0;
      const available = bar.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
        - parseFloat(style.borderLeftWidth) - parseFloat(style.borderRightWidth);
      const width = element => element.getBoundingClientRect().width / effectiveScale;
      const labelWidth = width(label), moreWidth = width(more.wrap);
      const widths = chips.map(width);
      let used = labelWidth, count = 0;
      const allWidth = widths.reduce((total, value) => total + gap + value, labelWidth);
      if (allWidth <= available - 1) count = chips.length;
      else {
        if (labelWidth + gap + moreWidth > available) { label.hidden = true; used = -gap; }
        for (const chipWidth of widths) {
          if (used + gap + chipWidth + gap + moreWidth > available - 1) break;
          used += gap + chipWidth;
          count++;
        }
      }
      chips.forEach((chip, index) => { chip.hidden = index >= count; });
      more.wrap.hidden = count === items.length;
      if (count !== previousCount) { more.update(items.slice(count)); previousCount = count; }
    };
    layoutChips();
  }
  async function pin(id) { pinned = [...pinned, id]; await writePinned(); render(); reflect(); }
  async function unpin(id) { pinned = pinned.filter(x => x !== id); await writePinned(); render(); reflect(); toast('已从链接栏取消固定'); }
  async function toggleCurrent() {
    const state = current(), profile = profileFor(state);
    if (!state || !profile) return;
    if (isPinned(profile.id)) { await unpin(profile.id); return; }
    let note = '已固定到链接栏';
    if (temporaryProfiles.has(profile.id)) {
      // Pinning a quick-connect session persists it first, like "保存连接",
      // carrying the credentials used this run so the pin reconnects silently.
      // The credential lives in this window's memory only, so say out loud when
      // there was nothing to carry instead of saving a silently password-less row.
      const remembered = profile.auth !== 'agent' && credentials.has(profile.id);
      const saved = await post(`/api/quick-connect/${profile.id}/save`, { name: profile.name, group: '快速连接', notes: '', remember: remembered, secret: credentials.get(profile.id) || '' });
      temporaryProfiles.delete(profile.id);
      profiles = profiles.filter(p => p.id !== saved.id);
      profiles.push(saved);
      renderConnections(); renderTabs();
      // The session is no longer temporary even if pinning below fails, so
      // refresh the "保存连接" button instead of leaving it visibly dead.
      window.DengQuickConnect?.reflect();
      if (!saved.hasSecret) note = '已固定到链接栏；这个连接没有保存凭据（本窗口未输入过密码），下次连接仍需输入';
    }
    await pin(profile.id);
    toast(note);
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
    let frame;
    const scheduleLayout = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => layoutChips());
    };
    new ResizeObserver(scheduleLayout).observe(bar);
    window.addEventListener('resize', scheduleLayout);
    document.fonts.ready.then(scheduleLayout);
    document.fonts.addEventListener('loadingdone', scheduleLayout);
    document.addEventListener('click', event => {
      if (overflowControl && !overflowControl.wrap.contains(event.target)) overflowControl.close();
    }, true);
    document.addEventListener('keydown', event => {
      if (event.key === 'Escape' && overflowControl && !overflowControl.menu.hidden) {
        overflowControl.close(); overflowControl.btn.focus();
      }
    });
  }, { once: true });
  return { render, reflect };
})();
