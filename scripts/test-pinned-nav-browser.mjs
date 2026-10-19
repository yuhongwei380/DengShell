// Opt-in browser regression for the pinned links bar: every chip and overflow
// item exposes a delete button, and unpinning (button or right-click) must go
// through the shared confirmation dialog before the layout is rewritten.
// Edge on Windows hands the browser process off from the launcher stub, so the
// browser is spawned directly and driven over CDP instead of puppeteer.launch.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
const [stage, chrome, modulePath] = process.argv.slice(2);
const { default: puppeteer } = await import(modulePath);
const fixture = JSON.parse(fs.readFileSync(stage + '/browser-fixture.json'));
const endpoint = new URL(fixture.url).origin;
async function api(pathname, body) { const r = await fetch(endpoint + pathname, { method: body === undefined ? 'GET' : 'POST', headers: { 'Content-Type': 'application/json', 'X-CloudShell-Token': fixture.token }, body: body === undefined ? undefined : JSON.stringify(body) }); const data = await r.json(); return { status: r.status, data }; }
await api('/api/appearance/patch', { onboardingCompleted: true, uiScale: 1 });
const created = [];
for (let i = 0; i < 8; i++) { const { status, data } = await api('/api/profiles', { name: `固定测试-${i + 1}`, host: '192.0.2.77', port: 22, user: 'root' }); assert.equal(status, 200, `create profile ${i + 1}`); created.push(data.id); }
await api('/api/appearance/patch', { layout: { 'dengshell.pinned.v1': created } });
const profileDir = fs.mkdtempSync(path.join(path.resolve(stage), 'edge-profile-'));
const child = spawn(chrome, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--noerrdialogs', `--user-data-dir=${profileDir}`, '--remote-debugging-port=0', 'about:blank'], { stdio: 'ignore' });
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
  page = await browser.newPage(); page.on('pageerror', e => errors.push(e.message)); await page.setViewport({ width: 1440, height: 900 });
  await page.goto(fixture.url, { waitUntil: 'networkidle0' });
  await page.waitForFunction(() => window.DengPinnedNav && document.querySelectorAll('.pinned-chip').length === 7);
  assert.equal(await page.$$eval('.pinned-chip .pinned-chip-remove', els => els.length), 7);
  await page.click('.pinned-more');
  await page.waitForFunction(() => !document.querySelector('.pinned-more-menu').hidden);
  assert.equal(await page.$$eval('.pinned-more-item .pinned-chip-remove', els => els.length), 1);
  await page.click('.pinned-more');
  checks.push('7 visible chips and the overflow item each expose a delete button');
  // Delete asks for confirmation; cancelling keeps the pin and the layout.
  let chips = await page.$$('.pinned-chip');
  await (await chips[0].$('.pinned-chip-remove')).click();
  await page.waitForSelector('#action-dialog[open]');
  assert((await page.$eval('#action-title', e => e.textContent)).includes('取消固定 固定测试-1'));
  await page.click('#action-cancel');
  await page.waitForFunction(() => !document.querySelector('#action-dialog').open);
  assert.equal(await page.$$eval('.pinned-chip', els => els.length), 7);
  assert.deepEqual(await page.evaluate(() => appearance.layout['dengshell.pinned.v1']), created);
  checks.push('Delete button opens a confirmation; cancel keeps the pin and layout');
  // Confirming removes exactly that pin and persists the layout. With 8 pins
  // the bar shows 7 chips plus overflow; after one removal all 7 fit and the
  // overflow button disappears.
  chips = await page.$$('.pinned-chip');
  await (await chips[0].$('.pinned-chip-remove')).click();
  await page.waitForSelector('#action-dialog[open]');
  await page.click('#action-confirm');
  await page.waitForFunction(() => appearance.layout['dengshell.pinned.v1'].length === 7 && document.querySelectorAll('.pinned-chip').length === 7 && !document.querySelector('.pinned-more'));
  assert.deepEqual(await page.evaluate(() => appearance.layout['dengshell.pinned.v1']), created.slice(1));
  checks.push('Confirming removes the pin, persists the layout and reflows the overflow');
  // Right-click now routes through the same confirmation instead of unpinning.
  await page.evaluate(() => document.querySelector('.pinned-chip').dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true })));
  await page.waitForSelector('#action-dialog[open]');
  await page.click('#action-confirm');
  await page.waitForFunction(() => document.querySelectorAll('.pinned-chip').length === 6);
  assert.deepEqual(await page.evaluate(() => appearance.layout['dengshell.pinned.v1']), created.slice(2));
  checks.push('Right-click unpin also requires confirmation');
  assert.deepEqual(errors, []);
  fs.writeFileSync(stage + '/pinned-nav.json', JSON.stringify({ passed: true, checks, errors }, null, 2)); console.log('PASS', checks);
} catch (error) { if (page) await page.screenshot({ path: stage + '/pinned-nav-failure.png' }); throw error } finally { try { await browser?.close(); } catch {} try { child.kill(); } catch {} try { fs.rmSync(profileDir, { recursive: true, force: true, maxRetries: 10, retryDelay: 500 }); } catch {} }
