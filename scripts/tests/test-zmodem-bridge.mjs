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

function fixture(options = {}) {
  const sent = [], lines = [], notices = [], downloads = [], uploads = [];
  const state = {
    id: 'qa-session', connected: true, ready: true, closed: false, serial: false, localOnly: false,
    cwd: '/srv/app', terminalDirectory: '/srv/logs', sftpAvailable: options.sftpAvailable ?? true,
    term: { writeln: line => lines.push(line) },
    shellIntegration: { atPrompt: options.atPrompt ?? false },
  };
  const context = vm.createContext({
    window: {}, console, TextDecoder, TextEncoder,
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
  const settle = () => new Promise(resolve => setTimeout(resolve, 0));
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
  assert.deepEqual(harness.sent, ['\x18\x18'], 'the half-open handshake is cancelled once');
  assert.equal(harness.lines.length, 1);
  assert.match(harness.lines[0], /SFTP 下载/);
  // A repeated advertisement inside lrzsz's retry window must not re-download.
  harness.feed(zmlink);
  await harness.settle();
  assert.equal(harness.downloads.length, 2);
  assert.equal(harness.sent.length, 1);
  // But a fresh shell-confirmed command is a new attempt, not a retry.
  harness.bridge.noteCommand(harness.state, 'rz');
  harness.feed(overstrike);
  await harness.settle();
  assert.deepEqual(harness.uploads, [['qa-session', '/srv/logs']]);
  assert.equal(harness.sent.length, 2);
}

// rz uploads through the existing picker, into the directory the shell is in.
{
  const harness = fixture();
  harness.bridge.noteCommand(harness.state, '/usr/bin/rz -be');
  harness.feed(overstrike);
  await harness.settle();
  assert.deepEqual(harness.uploads, [['qa-session', '/srv/logs']]);
  assert.deepEqual(harness.sent, ['\x18\x18']);
  assert.match(harness.lines[0], /SFTP 上传，目标目录：\/srv\/logs/);
}

// A handshake seen without a shell-confirmed command still stops lrzsz, but
// cannot invent file names.
{
  const harness = fixture();
  harness.feed('\r\n**8800000000000000\r\n');
  await harness.settle();
  assert.deepEqual(harness.uploads, []); assert.deepEqual(harness.downloads, []);
  assert.deepEqual(harness.sent, ['\x18\x18']);
  assert.match(harness.lines[0], /不支持该协议/);
}

// Byte-for-byte captures from lrzsz 0.12.20 (/usr/bin/rz, /usr/bin/sz) on a pty.
// `**` is followed by a CAN byte, so an advertisement-only match used to miss
// the whole thing and let the terminal fill with `**B0100000023be50`.
const realRz = 'rz waiting to receive.**\x18B0100000023be50\r\x8a\x11**\x18B0100000023be50\r\x8a\x11';
const realSz = 'rz\r**\x18B00000000000000\r\x8a\x11';

// Without shell integration there is no command report, yet lrzsz's own text
// names the direction: rz still becomes an SFTP upload.
{
  const harness = fixture();
  harness.state.shellIntegration = undefined;
  harness.feed(realRz);
  await harness.settle();
  assert.deepEqual(harness.sent, ['\x18\x18'], 'the repeated advertisement is cancelled once');
  assert.deepEqual(harness.uploads, [['qa-session', '/srv/logs']]);
  assert.match(harness.lines[0], /SFTP 上传/);
}

// sz's advertisement with a shell-confirmed command downloads the named files.
{
  const harness = fixture();
  harness.bridge.noteCommand(harness.state, 'sz -a probe.txt');
  harness.feed(realSz);
  await harness.settle();
  assert.deepEqual(harness.downloads, ['/srv/logs/probe.txt']);
  assert.deepEqual(harness.sent, ['\x18\x18']);
  // sz alone, without a command report, cannot invent names but still cancels.
  const blind = fixture();
  blind.feed(realSz);
  await blind.settle();
  assert.deepEqual(blind.downloads, []);
  assert.deepEqual(blind.sent, ['\x18\x18']);
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
  assert.deepEqual(harness.sent, ['\x18\x18'], 'the advertisement is still cancelled');

  const cleared = fixture();
  cleared.bridge.noteCommand(cleared.state, 'sz b.txt');
  cleared.bridge.clear(cleared.state);
  cleared.feed('\r\n**0800000021bf71c');
  await cleared.settle();
  assert.deepEqual(cleared.downloads, []);
}

console.log('PASS: rz / sz 桥接到 SFTP 上传与下载，握手只中止一次，普通输出不误触发。');
