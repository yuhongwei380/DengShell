import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { spawn } from 'node:child_process';

const [stage, chrome, modulePath] = process.argv.slice(2);
const { default: puppeteer } = await import(path.isAbsolute(modulePath) ? pathToFileURL(modulePath).href : modulePath);
const fixture = JSON.parse(fs.readFileSync(path.join(stage, 'browser-fixture.json')));
await fetch(new URL('/api/appearance/patch', fixture.url), { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CloudShell-Token': fixture.token }, body: JSON.stringify({ onboardingCompleted: true, uiScale: 1 }) });
const profileDir = fs.mkdtempSync(path.join(stage, 'edge-tabs-'));
const child = spawn(chrome, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--disable-background-networking', '--remote-debugging-port=0', '--user-data-dir=' + profileDir, 'about:blank'], { stdio: 'ignore', windowsHide: true });
let browser, page;
const errors = [], checks = [];
const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
try {
  let endpoint;
  for (let i = 0; i < 100; i++) {
    try { const [port, suffix] = fs.readFileSync(path.join(profileDir, 'DevToolsActivePort'), 'utf8').trim().split(/\r?\n/); endpoint = `ws://127.0.0.1:${port}${suffix}`; break; } catch {}
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  assert(endpoint, 'browser started');
  browser = await puppeteer.connect({ browserWSEndpoint: endpoint });
  page = await browser.newPage(); page.on('pageerror', error => errors.push(error.message));
  await page.setViewport({ width: 1440, height: 1000 });
  await page.goto(fixture.url, { waitUntil: 'networkidle0' });
  await page.waitForFunction(() => window.DengStartTabs && window.DengQuickConnect);
  await page.evaluate(() => reflectWindowState({ frameless: true }));
  assert.equal(await page.$eval('#add-session', el => el.title), '新选项卡');
  await page.click('#add-session');
  await page.waitForSelector('.start-tab.active');
  assert.equal(await page.$eval('#welcome-state', el => el.hidden), false);
  assert.equal(await page.$eval('#quick-connect-dialog', el => el.open), false);
  assert.equal(await page.evaluate(() => sessions.size), 0);
  await page.click('#welcome-quick-connect');
  await page.waitForSelector('#quick-connect-dialog[open]');
  await page.click('#quick-connect-dialog [data-close]');
  checks.push('The right-side + creates a frontend start page; quick connect remains available inside that page');
  // Create a real terminal view with a fake transport, never an SSH connection.
  await page.evaluate(() => {
    const profile = { id: 'tabs-live-profile', name: '保留的 SSH', host: 'example.invalid', user: 'qa', port: 22 };
    profiles.push(profile);
    const state = makeSessionState({ id: 'tabs-live', profileId: profile.id, home: '/', sftpAvailable: false });
    Object.assign(state, { localOnly: true, connected: false, tabOrder: nextSessionOrder++ });
    sessions.set(state.id, state); createTerminal(state);
    state.connected = true; state.localOnly = false;
    window.qaSocketCloses = 0; state.ws = { readyState: WebSocket.OPEN, send() {}, close() { qaSocketCloses++; } };
    activate(state.id);
  });
  await page.click('#add-session'); await settle();
  assert.equal(await page.evaluate(() => sessions.get('tabs-live').connected && sessions.get('tabs-live').host.hidden), true);
  assert.equal(await page.evaluate(() => qaSocketCloses), 0);
  await page.click('[data-session-id="tabs-live"] [role="tab"]'); await settle();
  assert.equal(await page.evaluate(() => activeID), 'tabs-live');
  assert.equal(await page.$eval('#welcome-state', el => el.hidden), true);
  checks.push('Start pages hide the current terminal while preserving its connection; selecting its tab restores it');
  await page.evaluate(() => { for (let i = 0; i < 40; i++) DengStartTabs.create(); });
  await settle();
  assert.equal(await page.$$eval('.session-tab', els => new Set(els.map(el => el.offsetTop)).size), 1);
  await page.$eval('#session-tabs', el => { el.scrollLeft = 0; }); await settle();
  assert.equal(await page.$eval('#session-tabs-previous', el => el.disabled), true);
  assert.equal(await page.$eval('#session-tabs-next', el => el.disabled), false);
  await page.click('#session-tabs-next'); await settle();
  assert((await page.$eval('#session-tabs', el => el.scrollLeft)) > 0);
  await page.click('#session-tabs-previous'); await settle();
  assert((await page.$eval('#session-tabs', el => el.scrollLeft)) <= 1);
  await page.click('#session-tabs-list');
  await page.waitForSelector('#session-tabs-menu:not([hidden])');
  assert.equal(await page.$$eval('#session-tabs-menu button', els => els.length), 43);
  assert.equal(await page.$$eval('#session-tabs-menu [aria-current="page"]', els => els.length), 1);
  await page.click('#session-tabs-menu button'); await settle();
  assert.equal(await page.evaluate(() => DengStartTabs.has(activeID)), true);
  assert.equal(await page.$eval('#session-tabs-menu', el => el.hidden), true);
  checks.push('43 tabs stay on one row; arrows move the viewport and the dropdown lists every tab with the active tab checked');
  // The list must use each tab's real action for SSH, RDP and process views.
  const kinds = await page.evaluate(() => {
    const calls = [];
    for (const [kind, label] of [['rdp', '测试 RDP'], ['process', '进程 · 测试']]) {
      const tab = node('div', `session-tab ${kind}-tab`), button = node('button');
      button.setAttribute('role', 'tab'); button.append(node('span', 'session-tab-label', label));
      button.onclick = () => calls.push(kind); tab.append(button); $('#session-tabs').append(tab);
    }
    window.qaKindCalls = calls;
    return calls.length;
  });
  assert.equal(kinds, 0); await settle();
  await page.click('#session-tabs-list');
  await page.evaluate(() => [...$('#session-tabs-menu').children].find(el => el.textContent.includes('测试 RDP')).click());
  await page.click('#session-tabs-list');
  await page.evaluate(() => [...$('#session-tabs-menu').children].find(el => el.textContent.includes('进程 · 测试')).click());
  assert.deepEqual(await page.evaluate(() => qaKindCalls), ['rdp', 'process']);
  await page.evaluate(() => { document.querySelectorAll('#session-tabs > .rdp-tab, #session-tabs > .process-tab').forEach(el => el.remove()); });
  checks.push('The dropdown dispatches RDP and process tab actions without treating them as SSH tabs');
  for (const [width, height, scale] of [[1440,1000,1], [1024,768,1], [800,600,1], [640,480,1], [480,360,1], [1440,1000,1.5], [1440,1000,2]]) {
    await page.setViewport({ width, height });
    await page.evaluate(value => { appearance.uiScale = value; applyUIScale(); }, scale); await settle();
    const metrics = await page.evaluate(() => {
      const plus = $('#add-session').getBoundingClientRect(), tabs = $('#session-tabs').getBoundingClientRect();
      const hit = document.elementFromPoint(plus.x + plus.width / 2, plus.y + plus.height / 2);
      return { left: plus.left, right: plus.right, tabsRight: tabs.right, visible: !!hit?.closest('#add-session'), rows: new Set([...$('#session-tabs').children].map(el => el.offsetTop)).size };
    });
    assert(metrics.visible && metrics.right <= width + 1 && metrics.left >= metrics.tabsRight - 1 && metrics.rows === 1, JSON.stringify({width, scale, metrics}));
    await page.click('#session-tabs-list');
    assert.equal(await page.$eval('#session-tabs-menu', el => { const r = el.getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth + 1 && r.bottom <= innerHeight + 1; }), true);
    await page.keyboard.press('Escape');
  }
  checks.push('The + remains clickable on the right at 480–1440px and 100–200% zoom; dropdown stays inside the viewport');
  await page.setViewport({ width: 1440, height: 1000 });
  await page.evaluate(() => { appearance.uiScale = 1; applyUIScale(); }); await settle();
  // Reordering a start page before a live tab must survive conversion to SSH.
  const replacement = await page.evaluate(async () => {
    const page = DengStartTabs.create();
    const tab = [...$('#session-tabs').children].find(el => el.dataset.startTabId === page.id);
    DengTabOrder.commit(tab, { before: $('#session-tabs').firstElementChild.dataset.tabKey });
    const oldApi = api, oldCreate = createTerminal, oldRefresh = DengCommandHistory.refresh;
    const profile = { id: 'tabs-replace-profile', name: '起始页建立的连接', host: 'example.invalid', user: 'qa', port: 22, auth: 'agent' };
    profiles.push(profile);
    let opened = 0;
    api = async (url, options) => url === '/api/sessions' ? { id: ++opened === 1 ? 'tabs-replaced' : 'tabs-background', profileId: profile.id, home: '/', sftpAvailable: false } : oldApi(url, options);
    createTerminal = state => { oldCreate(state); state.openTerminalSocket = () => {}; };
    DengCommandHistory.refresh = async () => {};
    try {
      const state = await connect(profile.id, false, { refreshHistory: false });
      const backgroundPage = DengStartTabs.create();
      await connect(profile.id, false, { background: true, refreshHistory: false });
      return { connected: !!state?.connected, removed: !DengStartTabs.has(page.id), first: $('#session-tabs').firstElementChild.dataset.sessionId, preserved: sessions.get('tabs-live').connected, socketCloses: qaSocketCloses, background: activeID === backgroundPage.id && DengStartTabs.has(backgroundPage.id) };
    } finally { api = oldApi; createTerminal = oldCreate; DengCommandHistory.refresh = oldRefresh; }
  });
  assert.deepEqual(replacement, { connected: true, removed: true, first: 'tabs-replaced', preserved: true, socketCloses: 0, background: true });
  checks.push('Foreground connects replace a reordered start page in place; background connects preserve that page and selection');
  await page.click('#add-session'); await settle();
  await page.click('.start-tab.active .tab-close'); await settle();
  assert.equal(await page.evaluate(() => qaSocketCloses), 0);
  assert.equal(await page.$$eval('.session-tab.active', els => els.length), 1);
  checks.push('Closing a start page activates a neighbor without disconnecting another session');
  if (process.env.DENGSHELL_TABS_SCREENSHOT) {
    await page.evaluate(() => { CloudShellTheme.set('dark'); DengStartTabs.create(); });
    await page.waitForFunction(() => !document.documentElement.dataset.lightSwitch); await settle();
    await page.click('#session-tabs-list');
    await page.screenshot({ path: process.env.DENGSHELL_TABS_SCREENSHOT });
  }
  assert.deepEqual(errors, []);
  console.log('PASS', checks);
} catch (error) {
  if (page && process.env.DENGSHELL_TABS_SCREENSHOT) await page.screenshot({ path: process.env.DENGSHELL_TABS_SCREENSHOT });
  console.error('Page errors:', errors); throw error;
} finally {
  try { await browser?.close(); } catch {}
  try { child.kill(); } catch {}
  try { fs.rmSync(profileDir, { recursive: true, force: true, maxRetries: 10, retryDelay: 150 }); } catch {}
}
