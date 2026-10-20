import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

// The follow-terminal-directory checkbox now defaults to on: once the shell
// integration reports ready the session starts following and the file panel
// jumps to the terminal directory. Shells without prompt integration stay off,
// and a user who unchecked the box before connecting is respected.
const appSource = readFileSync(new URL('../../web/js/app.js', import.meta.url), 'utf8');
function sourceBetween(start, end) {
  const first = appSource.indexOf(start), last = appSource.indexOf(end, first);
  assert.ok(first >= 0 && last > first, `missing production code: ${start}`);
  return appSource.slice(first, last);
}
const filesSource = sourceBetween('function filesUsable(', '\n// The status request');
const followSource = sourceBetween('let pendingFollow', '$(\'#follow-terminal\').onchange');

function fixture() {
  const directories = [], notices = [];
  const context = vm.createContext({
    Uint8Array, TextDecoder,
    navigate: async directory => directories.push(directory),
    toast: message => notices.push(message),
    renderSessionInfo() {},
  });
  vm.runInContext(filesSource + '\n' + followSource, context);
  return { context, directories, notices };
}

// The default is on: a connected session with ready integration starts
// following and navigates to the terminal directory exactly once.
let test = fixture();
assert.equal(vm.runInContext('pendingFollow', test.context), true, 'follow must default to on');
const ready = { id: 'a', connected: true, follow: false, cwd: '/home/x', terminalDirectory: '/srv/应用', sftpAvailable: true, shellIntegration: { ready: true } };
vm.runInContext('applyPendingFollow', test.context)(ready);
assert.equal(ready.follow, true);
assert.deepEqual(test.directories, ['/srv/应用']);
vm.runInContext('applyPendingFollow', test.context)(ready);
assert.deepEqual(test.directories, ['/srv/应用'], 'already-following sessions must not re-navigate');

// Shells without prompt integration (PowerShell, cmd) cannot report the
// terminal directory, so the default must not switch them on.
test = fixture();
const plain = { id: 'b', connected: true, follow: false, cwd: '/', shellIntegration: { ready: false } };
vm.runInContext('applyPendingFollow', test.context)(plain);
assert.equal(plain.follow, false);
assert.equal(test.directories.length, 0);

// Unchecking before connecting records the intent; the session does not start
// following even once integration is ready.
test = fixture();
vm.runInContext('pendingFollow = false', test.context);
const opted = { id: 'c', connected: true, follow: false, cwd: '/', terminalDirectory: '/srv/x', sftpAvailable: true, shellIntegration: { ready: true } };
vm.runInContext('applyPendingFollow', test.context)(opted);
assert.equal(opted.follow, false);
assert.equal(test.directories.length, 0);

console.log('PASS: follow-terminal-directory defaults on at integration ready; integration-less shells and opted-out sessions stay off.');
