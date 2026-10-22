// Opt-in browser regression for the pinned links bar: every chip and overflow
// item exposes a close button, and unpinning must go through the shared
// confirmation dialog before the layout is rewritten. Right-click renames the
// connection in place: the saved profile name (the same field the server
// manager edits) updates everywhere, while cancel changes nothing.
// Edge on Windows hands the browser process off from the launcher stub, so the
// browser is spawned directly and driven over CDP instead of puppeteer.launch.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { pathToFileURL } from 'node:url';
const [stage, chrome, modulePath] = process.argv.slice(2);
const { default: puppeteer } = await import(path.isAbsolute(modulePath) ? pathToFileURL(modulePath).href : modulePath);
const fixture = JSON.parse(fs.readFileSync(stage + '/browser-fixture.json'));
const endpoint = new URL(fixture.url).origin;
async function api(pathname, body) { const r = await fetch(endpoint + pathname, { method: body === undefined ? 'GET' : 'POST', headers: { 'Content-Type': 'application/json', 'X-CloudShell-Token': fixture.token }, body: body === undefined ? undefined : JSON.stringify(body) }); const data = await r.json(); return { status: r.status, data }; }
await api('/api/appearance/patch', { onboardingCompleted: true, uiScale: 1 });
const created = [];
for (let i = 0; i < 8; i++) { const { status, data } = await api('/api/profiles', { name: `固定测试-${i + 1}`, host: '192.0.2.77', port: 22, user: 'root' }); assert.equal(status, 200, `create profile ${i + 1}`); created.push(data.id); }
await api('/api/appearance/patch', { layout: { 'dengshell.pinned.v1': created } });
const profileDir = fs.mkdtempSync(path.join(path.resolve(stage), 'edge-profile-'));
const child = spawn(chrome, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--noerrdialogs', `--user-data-dir=${profileDir}`, '--remote-debugging-port=0', 'about:blank'], { stdio: 'ignore', windowsHide: true });
let browser;
const checks = [], errors = []; let page;
try {
  let wsEndpoint = null;
  for (let attempt = 0; attempt < 100 && !wsEndpoint; attempt++) {
    const portFile = path.join(profileDir, 'DevToolsActivePort');
    if (fs.existsSync(portFile)) {
      const [port, wsPath] = fs.readFileSync(portFile, 'utf8').trim().split(/\r?\n/);
      if (port && wsPath) wsEndpoint = `ws://127.0.0.1:${port.trim()}${wsPath.trim()}`;
    }
    if (!wsEndpoint) await new Promise(resolve => setTimeout(resolve, 200));
  }
  if (!wsEndpoint) throw new Error('headless Edge did not expose DevToolsActivePort');
  browser = await puppeteer.connect({ browserWSEndpoint: wsEndpoint });
  page = await browser.newPage(); page.on('pageerror', e => errors.push(e.message)); await page.setViewport({ width: 2800, height: 900 });
  await page.goto(fixture.url, { waitUntil: 'networkidle0' });
  await page.waitForFunction(() => window.DengPinnedNav && document.querySelectorAll('.pinned-chip:not([hidden])').length === 8);
  await page.evaluate(() => reflectWindowState({ frameless: true }));
  assert.equal(await page.$eval('.pinned-more-wrap', el => el.hidden), true);
  checks.push('A wide bar shows all 8 connections without an arbitrary 7-item limit');
  const inspectLayout = async () => page.evaluate(() => {
    const bar = document.querySelector('#pinned-bar'), rect = bar.getBoundingClientRect();
    const chips = [...bar.querySelectorAll('.pinned-chip:not([hidden])')];
    const wrap = bar.querySelector('.pinned-more-wrap');
    const shown = [...chips, ...(!wrap.hidden ? [wrap] : [])];
    return {
      visible: chips.length,
      overflow: bar.querySelectorAll('.pinned-more-item').length,
      more: !wrap.hidden,
      clipped: shown.some(el => { const r = el.getBoundingClientRect(); return r.left < rect.left - 1 || r.right > rect.right + 1; }),
      labels: chips.map(el => el.querySelector('.pinned-chip-open span').textContent),
    };
  });
  // A real resize must redistribute the same pins, not crop partial chips or
  // lose entries. Exercise the narrow-header breakpoint and application zoom.
  for (const width of [1440, 1100, 800, 700, 520]) {
    await page.setViewport({ width, height: 900 });
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    const layout = await inspectLayout();
    assert.equal(layout.clipped, false, `no clipped connection at width ${width}`);
    assert.equal(layout.visible + (layout.more ? layout.overflow : 0), 8, `all pins accessible at width ${width}`);
  }
  await page.setViewport({ width: 1440, height: 900 });
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  await page.waitForFunction(() => !document.querySelector('.pinned-more-wrap').hidden);
  const collapsed = await inspectLayout();
  assert(collapsed.visible > 0 && collapsed.visible < 8);
  assert.equal(await page.$eval('.pinned-more', el => el.textContent), '︙');
  await page.click('.pinned-more');
  await page.waitForFunction(() => !document.querySelector('.pinned-more-menu').hidden);
  assert.equal(await page.$$eval('.pinned-more-item .pinned-chip-remove', els => els.length), collapsed.overflow);
  const menuState = await page.$eval('.pinned-more-menu', el => {
    const r = el.getBoundingClientRect(), hit = document.elementFromPoint(r.left + 20, r.top + 15);
    return { visible: !!hit?.closest('.pinned-more-menu'), hidden: el.hidden, rect: r.toJSON(), hit: hit?.outerHTML.slice(0, 200) };
  });
  assert(menuState.visible, JSON.stringify(menuState));
  if (process.env.DENGSHELL_PINNED_SCREENSHOT) await page.screenshot({ path: process.env.DENGSHELL_PINNED_SCREENSHOT });
  await page.keyboard.press('Escape');
  assert.equal(await page.$eval('.pinned-more', el => el.getAttribute('aria-expanded')), 'false');
  await page.click('.pinned-more');
  await page.click('.pinned-label');
  assert.equal(await page.$eval('.pinned-more-menu', el => el.hidden), true);
  checks.push('Resizing at 1440/1100/800/700/520px keeps every pin accessible; ellipsis opens the list, Escape/outside click close it');
  // At very limited width all entries remain reachable from the one menu.
  await page.$eval('#pinned-bar', el => { el.style.maxWidth = '115px'; });
  await page.waitForFunction(() => document.querySelectorAll('.pinned-chip:not([hidden])').length === 0);
  const compact = await inspectLayout();
  assert.equal(compact.clipped, false); assert.equal(compact.overflow, 8);
  await page.click('.pinned-more');
  await page.evaluate(() => {
    window.qaPinnedConnect = connect; window.qaPinnedCalls = [];
    connect = id => { window.qaPinnedCalls.push(id); };
  });
  await page.click('.pinned-more-item-open');
  const overflowClick = await page.evaluate(() => {
    connect = window.qaPinnedConnect;
    const calls = window.qaPinnedCalls;
    delete window.qaPinnedConnect; delete window.qaPinnedCalls;
    return calls;
  });
  assert.deepEqual(overflowClick, [created[0]]);
  assert.equal(await page.$eval('.pinned-more-menu', el => el.hidden), true);
  await page.$eval('#pinned-bar', el => { el.style.maxWidth = ''; });
  await page.waitForFunction(() => document.querySelectorAll('.pinned-chip:not([hidden])').length > 0);
  await page.evaluate(() => { appearance.uiScale = 1.5; applyUIScale(); });
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  assert.equal((await inspectLayout()).clipped, false);
  await page.click('.pinned-more');
  assert.equal(await page.$eval('.pinned-more-menu', el => { const r = el.getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth + 1 && r.bottom <= innerHeight + 1; }), true);
  await page.keyboard.press('Escape');
  await page.evaluate(() => { appearance.uiScale = 1; applyUIScale(); });
  await page.waitForFunction(() => document.querySelectorAll('.pinned-chip:not([hidden])').length > 0);
  checks.push('A menu-only bar opens the chosen connection; 150% application zoom keeps chips and menu in the viewport');
  // Delete asks for confirmation; cancelling keeps the pin and the layout.
  let chips = await page.$$('.pinned-chip');
  await (await chips[0].$('.pinned-chip-remove')).click();
  await page.waitForSelector('#action-dialog[open]');
  assert((await page.$eval('#action-title', e => e.textContent)).includes('取消固定 固定测试-1'));
  await page.click('#action-cancel');
  await page.waitForFunction(() => !document.querySelector('#action-dialog').open);
  assert.equal(await page.$$eval('.pinned-chip', els => els.length), 8);
  assert.deepEqual(await page.evaluate(() => appearance.layout['dengshell.pinned.v1']), created);
  checks.push('Delete button opens a confirmation; cancel keeps the pin and layout');
  // Confirming removes exactly that pin and persists the layout. The visible
  // prefix and overflow list are recalculated against the current width.
  chips = await page.$$('.pinned-chip');
  await (await chips[0].$('.pinned-chip-remove')).click();
  await page.waitForSelector('#action-dialog[open]');
  await page.click('#action-confirm');
  await page.waitForFunction(() => appearance.layout['dengshell.pinned.v1'].length === 7 && document.querySelectorAll('.pinned-chip').length === 7);
  assert.deepEqual(await page.evaluate(() => appearance.layout['dengshell.pinned.v1']), created.slice(1));
  checks.push('Confirming removes the pin, persists the layout and reflows the overflow');
  // Right-click opens the rename dialog prefilled with the current name;
  // cancelling changes nothing anywhere.
  await page.evaluate(() => document.querySelector('.pinned-chip').dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true })));
  await page.waitForSelector('#action-dialog[open]');
  assert((await page.$eval('#action-title', e => e.textContent)).includes('重命名连接'));
  assert.equal(await page.$eval('#action-input', e => e.value), '固定测试-2');
  await page.click('#action-cancel');
  await page.waitForFunction(() => !document.querySelector('#action-dialog').open);
  assert.equal(await page.$eval('.pinned-chip .pinned-chip-open span', e => e.textContent), '固定测试-2');
  checks.push('Right-click opens a prefilled rename dialog; cancel changes nothing');
  // Confirming renames the saved profile: the chip, the in-app profile list
  // (the same name the server manager edits) and the persisted config all
  // update, while the pin layout stays untouched.
  const renamed = '改名后的连接';
  await page.evaluate(() => document.querySelector('.pinned-chip').dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true })));
  await page.waitForSelector('#action-dialog[open]');
  await page.$eval('#action-input', (el, value) => { el.value = value; }, renamed);
  await page.click('#action-confirm');
  await page.waitForFunction(name => document.querySelector('.pinned-chip .pinned-chip-open span')?.textContent === name, {}, renamed);
  assert.equal(await page.evaluate(id => profiles.find(profile => profile.id === id)?.name, created[1]), renamed);
  assert.equal((await api('/api/config')).data.servers.find(profile => profile.id === created[1]).name, renamed);
  assert.deepEqual(await page.evaluate(() => appearance.layout['dengshell.pinned.v1']), created.slice(1));
  checks.push('Renaming updates the chip, the shared profile name and the persisted config; pins stay put');
  // Clicking a chip always opens a fresh session, even when one for that
  // profile is already open and active: the old activate-if-exists path made
  // repeat clicks look completely dead.
  const clickCalls = await page.evaluate(() => {
    const calls = { connect: [], activate: [] };
    const originalConnect = connect, originalActivate = activate;
    connect = id => { calls.connect.push(id); return Promise.resolve(null); };
    activate = id => { calls.activate.push(id); };
    const target = appearance.layout['dengshell.pinned.v1'][0];
    sessions.set('qa-existing', { id: 'qa-existing', profileId: target, connected: true });
    activeID = 'qa-existing';
    document.querySelector('.pinned-chip .pinned-chip-open').click();
    sessions.delete('qa-existing'); activeID = null;
    connect = originalConnect; activate = originalActivate;
    return calls;
  });
  assert.deepEqual(clickCalls.connect, [created[1]], 'chip click must call connect for the pinned profile');
  assert.deepEqual(clickCalls.activate, [], 'chip click must not merely activate the existing session');
  checks.push('Clicking a chip with an existing active session still opens a new connection');
  // Long labels and moving/resizing the monitor column must also reflow the
  // header, even when there is no window resize event.
  await page.evaluate(() => {
    profiles.find(profile => profile.id === appearance.layout['dengshell.pinned.v1'][1]).name = '很长的服务器连接名称 '.repeat(5);
    DengPinnedNav.render();
    appearance.monitorSide = 'right'; applyWorkspacePlacement();
  });
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  assert.equal((await inspectLayout()).clipped, false);
  await page.evaluate(() => { appearance.monitorSide = 'left'; applyWorkspacePlacement(); document.documentElement.style.setProperty('--sidebar-width', '540px'); });
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  assert.equal((await inspectLayout()).clipped, false);
  await page.evaluate(() => { document.documentElement.style.removeProperty('--sidebar-width'); });
  checks.push('Long names, monitor placement and sidebar width changes reflow without cropping');
  await page.setViewport({ width: 2800, height: 900 });
  await page.waitForFunction(() => document.querySelectorAll('.pinned-chip:not([hidden])').length === 7 && document.querySelector('.pinned-more-wrap').hidden);
  checks.push('Growing the window restores all remaining connections and hides the ellipsis');
  assert.deepEqual(errors, []);
  fs.writeFileSync(stage + '/pinned-nav.json', JSON.stringify({ passed: true, checks, errors }, null, 2)); console.log('PASS', checks);
} catch (error) {
  if (page) {
    await page.screenshot({ path: stage + '/pinned-nav-failure.png' });
    console.error('Browser state:', await page.evaluate(() => {
      const r = document.querySelector('.brand').getBoundingClientRect();
      return { dialogs: [...document.querySelectorAll('dialog[open]')].map(el => el.id), brand: r.toJSON(), target: document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)?.outerHTML.slice(0, 300) };
    }));
  }
  throw error;
} finally { try { await browser?.close(); } catch {} try { child.kill(); } catch {} try { fs.rmSync(profileDir, { recursive: true, force: true, maxRetries: 10, retryDelay: 500 }); } catch {} }
