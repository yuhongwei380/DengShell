import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

// lrzsz (rz / sz) has no ZModem peer here, so the bridge turns it into SFTP
// transfers and cancels the handshake. Exercise the real module with the real
// path helper; no browser, no SSH.
const bridgeSource = readFileSync(new URL('../../web/js/zmodem-bridge.js', import.meta.url), 'utf8');
const appSource = readFileSync(new URL('../../web/js/app.js', import.meta.url), 'utf8');
const normalizePathSource = appSource.split('\n').find(line => line.startsWith('function normalizePath('));
assert.ok(normalizePathSource, 'app.js normalizePath helper moved; update this test');

const cancelRounds = 3;
const can = '\x18\x18';

function fixture(options = {}) {
  const sent = [], lines = [], notices = [], downloads = [], uploads = [];
  const state = {
    id: 'qa-session', connected: true, ready: true, closed: false, serial: false, localOnly: false,
    cwd: '/srv/app', terminalDirectory: '/srv/logs', sftpAvailable: options.sftpAvailable ?? true,
    term: { writeln: line => lines.push(line) },
    shellIntegration: { atPrompt: options.atPrompt ?? false },
  };
  const context = vm.createContext({
    window: {}, console, TextDecoder, TextEncoder, setTimeout, clearTimeout,
    normalizePath: new Function(`return (${normalizePathSource})`)(),
    sendInput: (_state, data) => sent.push(data),
    toast: message => notices.push(message),
    chooseUploads: async (_state, directory) => { uploads.push([_state.id, directory]); },
    sessions: new Map([[state.id, state]]),
    DengFileTools: { downloadFile: async (_state, target) => { downloads.push(target); } },
  });
  vm.runInContext(bridgeSource, context);
  const bridge = context.window.DengZmodemBridge;
  const feed = text => bridge.observe(state, new TextEncoder().encode(text));
  // The cancel sequence is paced out before the transfer starts, so waiting has
  // to outlast it.
  const settle = (ms = 1200) => new Promise(resolve => setTimeout(resolve, ms));
  return { state, bridge, feed, settle, sent, lines, notices, downloads, uploads };
}

const zmlink = '\r\n**0800000021bf71c\r\n\x01';
const overstrike = '\r\x08\x08B0800000021bf71c';

// Command parsing only counts the shell-confirmed line.
{
  const { bridge } = fixture();
  assert.deepEqual(Array.from(bridge.parse('sz -a b.txt "log one.txt"').files), ['b.txt', 'log one.txt']);
  assert.equal(bridge.parse('sz -a b.txt "log one.txt"').kind, 'sz');
  assert.equal(bridge.parse('/usr/bin/rz -be').kind, 'rz');
  assert.deepEqual(Array.from(bridge.parse('sz -- -odd.txt').files), ['-odd.txt']);
  assert.deepEqual(Array.from(bridge.parse('sz ./ ../ hidden.txt').files), ['hidden.txt']);
  assert.deepEqual(Array.from(bridge.parse('sz').files), []);
  for (const noise of ['ls -l', 'rsz', 'szutil', 'echo "sz"', '', 'sudo sz']) assert.equal(bridge.parse(noise), null, noise);
}

// sz <files> downloads exactly those names against the terminal directory.
{
  const harness = fixture();
  harness.bridge.noteCommand(harness.state, 'sz -a b.txt "log one.txt"');
  harness.feed(zmlink);
  await harness.settle();
  assert.deepEqual(harness.downloads, ['/srv/logs/b.txt', '/srv/logs/log one.txt']);
  assert.ok(harness.sent.every(value => value === can), '只发送中止字节');
  assert.equal(harness.sent.length, cancelRounds, '握手在开始传输前被反复中止');
  assert.equal(harness.lines.length, 1);
  assert.match(harness.lines[0], /SFTP 下载/);
  // lrzsz repeats the advertisement until it dies: a repeat may only poke the
  // cancel sequence again, never open a second transfer or print a second note.
  harness.feed(zmlink);
  await harness.settle();
  assert.equal(harness.downloads.length, 2, '重试的广告不得重复下载');
  assert.equal(harness.lines.length, 1, '重试的广告不得再提示一次');
  assert.equal(harness.sent.length, cancelRounds + 1, '重试只补发一次中止序列');
  // But a fresh shell-confirmed command is a new attempt, not a retry.
  harness.bridge.noteCommand(harness.state, 'rz');
  harness.feed(overstrike);
  await harness.settle();
  assert.deepEqual(harness.uploads, [['qa-session', '/srv/logs']]);
  assert.equal(harness.sent.length, cancelRounds * 2 + 1);
}

// rz uploads through the existing picker, into the directory the shell is in.
{
  const harness = fixture();
  harness.bridge.noteCommand(harness.state, '/usr/bin/rz -be');
  harness.feed(overstrike);
  await harness.settle();
  assert.deepEqual(harness.uploads, [['qa-session', '/srv/logs']]);
  assert.equal(harness.sent.length, cancelRounds);
  assert.match(harness.lines[0], /SFTP 上传，目标目录：\/srv\/logs/);
}

// A handshake seen without a shell-confirmed command still stops lrzsz, but
// cannot invent file names.
{
  const harness = fixture();
  harness.feed('\r\n**8800000000000000\r\n');
  await harness.settle();
  assert.deepEqual(harness.uploads, []); assert.deepEqual(harness.downloads, []);
  assert.equal(harness.sent.length, cancelRounds);
  assert.match(harness.lines[0], /不支持该协议/);
}

// Byte-for-byte captures from lrzsz 0.12.20 (/usr/bin/rz, /usr/bin/sz) on a pty.
// `**` is followed by a CAN byte, so an advertisement-only match used to miss
// the whole thing and let the terminal fill with `**B0100000023be50`.
const realRz = 'rz waiting to receive.**\x18B0100000023be50\r\x8a\x11**\x18B0100000023be50\r\x8a\x11';
const realSz = 'rz\r**\x18B00000000000000\r\x8a\x11';

// Without shell integration there is no command report, yet lrzsz's own text
// names the direction: rz still becomes an SFTP upload. Its own retries must not
// stack a second picker on top of the first one.
{
  const harness = fixture();
  harness.state.shellIntegration = undefined;
  harness.feed(realRz);
  await harness.settle();
  assert.equal(harness.sent.length, cancelRounds, '中止序列在打开选择框前就发出');
  assert.deepEqual(harness.uploads, [['qa-session', '/srv/logs']]);
  assert.match(harness.lines[0], /SFTP 上传/);
  // The retries lrzsz keeps sending while it waits: still one transfer, one note.
  harness.feed(realRz);
  harness.feed('**\x18B0100000023be50\r\x8a\x11');
  await harness.settle();
  assert.equal(harness.uploads.length, 1, '重试的广告不得再开一次上传');
  assert.equal(harness.lines.length, 1, '重试的广告不得再提示一次');
  assert.equal(harness.sent.length, cancelRounds + 2, '重试继续补发中止序列');
}

// sz's advertisement with a shell-confirmed command downloads the named files.
{
  const harness = fixture();
  harness.bridge.noteCommand(harness.state, 'sz -a probe.txt');
  harness.feed(realSz);
  await harness.settle();
  assert.deepEqual(harness.downloads, ['/srv/logs/probe.txt']);
  assert.equal(harness.sent.length, cancelRounds);
  // sz alone, without a command report, cannot invent names but still cancels.
  const blind = fixture();
  blind.feed(realSz);
  await blind.settle();
  assert.deepEqual(blind.downloads, []);
  assert.equal(blind.sent.length, cancelRounds);
}

// Ordinary output that merely looks hex-ish is ignored.
{
  for (const text of ['**hello**', '**0800', 'BB08123456', '\x08\x08B0zzzzzz']) {
    const harness = fixture();
    harness.bridge.noteCommand(harness.state, 'sz b.txt');
    harness.feed(text);
    await harness.settle();
    assert.deepEqual(harness.downloads, [], text);
    assert.deepEqual(harness.sent, [], text);
  }
  // Without a shell-confirmed rz / sz, only the unambiguous form counts.
  for (const text of ['**0800000021bf71c', 'echo **0800000021bf71c** done']) {
    const harness = fixture();
    harness.feed(text);
    await harness.settle();
    assert.deepEqual(harness.lines, [], text);
    assert.deepEqual(harness.sent, [], text);
  }
}

// The advertisement may straddle two output frames.
{
  const harness = fixture();
  harness.bridge.noteCommand(harness.state, 'sz b.txt');
  harness.feed('\r\n**08000000');
  await harness.settle();
  assert.deepEqual(harness.downloads, []);
  harness.feed('21bf71c\r\n');
  await harness.settle();
  assert.deepEqual(harness.downloads, ['/srv/logs/b.txt']);
}

// Without a shell prompt the shell is gone: no injected cancel bytes, but the
// transfer bridge still works. When SFTP is unavailable nothing is queued.
{
  const waiting = fixture({ atPrompt: true });
  waiting.bridge.noteCommand(waiting.state, 'sz b.txt');
  waiting.feed(zmlink);
  await waiting.settle();
  assert.deepEqual(waiting.sent, []);
  assert.deepEqual(waiting.downloads, ['/srv/logs/b.txt']);

  const noSftp = fixture({ sftpAvailable: false });
  noSftp.bridge.noteCommand(noSftp.state, 'rz');
  noSftp.feed(zmlink);
  await noSftp.settle();
  assert.deepEqual(noSftp.uploads, []);
  assert.match(noSftp.lines[0], /scp \/ sftp/);
}

// A new shell command or prompt drops a stale intent.
{
  const harness = fixture();
  harness.bridge.noteCommand(harness.state, 'sz b.txt');
  harness.bridge.noteCommand(harness.state, 'ls');
  harness.feed(zmlink);
  await harness.settle();
  assert.deepEqual(harness.downloads, []);
  assert.equal(harness.sent.length, cancelRounds, 'the advertisement is still cancelled');

  const cleared = fixture();
  cleared.bridge.noteCommand(cleared.state, 'sz b.txt');
  cleared.bridge.clear(cleared.state);
  cleared.feed('\r\n**0800000021bf71c');
  await cleared.settle();
  assert.deepEqual(cleared.downloads, []);
}

console.log('PASS: rz / sz 桥接到 SFTP 上传与下载，一次尝试只走一次传输，重试的广告只补发中止序列。');
