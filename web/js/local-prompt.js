'use strict';
// Xshell-style local prompt: while no session is connected, network commands
// typed in the command bar run against the local machine and print straight
// into the terminal area instead of opening the quick-connect dialog. Supported:
// ping 主机 [-t] (Ctrl+C stops), traceroute 主机, nslookup 主机, curl [-I] 网址.
window.DengLocalPrompt = (() => {
  let view = null, generation = 0;
  const commands = /^(ping|traceroute|tracert|nslookup|curl)(?:\s|$)/i;
  function ensure() {
    if (view?.isConnected) return view;
    view = node('pre', 'local-prompt-output');
    view.id = 'local-prompt-output';
    view.setAttribute('role', 'log');
    view.hidden = true;
    $('#terminal-output').append(view);
    return view;
  }
  function trimHistory(box) { while (box.children.length > 80) box.firstChild.remove(); }
  // A sink writes one chunk and returns an in-place updater, or null when the
  // output is append-only. The scratchpad supports updates; a session terminal
  // appends to scrollback.
  function scratchSink(box) {
    return (text, kind) => {
      const span = node('span', kind === 'cmd' ? 'local-prompt-line' : '');
      span.textContent = text;
      box.append(span);
      trimHistory(box);
      box.scrollTop = box.scrollHeight;
      return value => { span.textContent = value; box.scrollTop = box.scrollHeight; };
    };
  }
  function termSink(term) {
    return (text, kind) => {
      // xterm treats a bare LF as "move down, keep the column": multi-line
      // payloads (ping output) must use CRLF, or every line starts where the
      // previous one ended and the output staircases to the right.
      const body = text.replace(/[\r\n]+$/, '').replace(/\r?\n/g, '\r\n');
      term.writeln(kind === 'cmd' ? `\x1b[38;5;114m${body}\x1b[0m` : `\x1b[38;5;245m${body}\x1b[0m`);
      return null;
    };
  }
  function parsePing(command) {
    const tokens = command.trim().replace(/^ping(?:\s+|$)/i, '').trim().split(/\s+/).filter(Boolean);
    let continuous = false;
    const hosts = [];
    for (const token of tokens) {
      if (token === '-t' || token === '-T') continuous = true;
      else hosts.push(token);
    }
    if (hosts.length !== 1 || hosts[0].startsWith('-')) return null;
    return { host: hosts[0], continuous };
  }
  async function run(command, sink) {
    if (!sink) {
      const box = ensure();
      box.hidden = false;
      $('#welcome-state').hidden = true;
      sink = scratchSink(box);
    }
    const runId = ++generation; // supersedes any running continuous probe
    sink(`[本机] $ ${command.trim()}\n`, 'cmd');
    const trimmed = command.trim();
    const verb = ((trimmed.match(/^([a-z]+)/i) || [])[1] || '').toLowerCase();
    if (verb === 'ping') { await runPing(trimmed, sink, runId); return; }
    if (verb === 'traceroute' || verb === 'tracert') { await runHostProbe('traceroute', trimmed, sink); return; }
    if (verb === 'nslookup') { await runHostProbe('nslookup', trimmed, sink); return; }
    if (verb === 'curl') { await runCurl(trimmed, sink); return; }
    sink('未知本机命令。\n\n');
  }
  async function runPing(command, sink, runId) {
    const parsed = parsePing(command);
    if (!parsed) { sink('ping 后只接受主机名或 IP，可加 -t 持续探测。\n\n'); return; }
    if (parsed.continuous) { await continuous(sink, parsed.host, runId); return; }
    const pending = sink('正在从本机探测…\n');
    try {
      const payload = await post('/api/quick-connect/ping', { host: parsed.host });
      const text = `${(payload?.output || '').trimEnd()}\n\n`;
      if (pending) pending(text); else sink(text);
    } catch (error) {
      const text = `${error.message || error}\n\n`;
      if (pending) pending(text); else sink(text);
    }
    if (runId !== generation) { const text = '（已被新命令取代）\n'; if (pending) pending(text); else sink(text); }
  }
  async function runHostProbe(kind, command, sink) {
    const arg = command.replace(/^[a-z]+\s+/i, '').trim();
    if (!arg || /\s/.test(arg)) { sink(`${kind} 后只接受一个主机名或 IP。\n\n`); return; }
    const pending = sink(kind === 'traceroute' ? '正在追踪路由（最多 20 跳）…\n' : '正在解析域名…\n');
    const write = text => { if (pending) pending(text); else sink(text); };
    try {
      const payload = await post(`/api/local-probe/${kind}`, { host: arg });
      write(`${(payload?.output || '').trimEnd()}\n\n`);
    } catch (error) {
      write(`${error.message || error}\n\n`);
    }
  }
  async function runCurl(command, sink) {
    const tokens = command.replace(/^curl\s+/i, '').trim().split(/\s+/).filter(Boolean);
    let head = false;
    const positional = [];
    for (const token of tokens) {
      if (token === '-I' || token === '--head') head = true;
      else if (token.startsWith('-')) { sink('curl 仅支持网址与 -I（只看响应头）。\n\n'); return; }
      else positional.push(token);
    }
    if (positional.length !== 1) { sink('curl 后需要一个 http/https 网址，可加 -I 只看响应头。\n\n'); return; }
    const pending = sink('正在请求网址…\n');
    const write = text => { if (pending) pending(text); else sink(text); };
    try {
      const payload = await post('/api/local-probe/curl', { url: positional[0], head });
      write(`${(payload?.output || '').trimEnd()}\n\n`);
    } catch (error) {
      write(`${error.message || error}\n\n`);
    }
  }
  async function continuous(sink, host, runId) {
    const status = sink('持续探测中，按 Ctrl+C 停止…\n');
    const setStatus = text => { if (status) status(text); else sink(text); };
    // Console-style interrupt: Ctrl+C stops the probe unless it would copy a
    // text selection.
    let stopped = false;
    const onKey = event => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'c') {
        if (window.getSelection()?.toString()) return;
        event.preventDefault();
        stopped = true;
      }
    };
    document.addEventListener('keydown', onKey);
    let sent = 0, received = 0, header = false;
    try {
      for (;;) {
        let payload = null, failure = null;
        try { payload = await post('/api/quick-connect/ping', { host, count: 1 }); }
        catch (error) { failure = error; }
        if (runId !== generation) return; // a newer command owns the scratchpad
        if (failure) {
          sent++;
          sink(`${sent}  ${failure.message || failure}\n`);
        } else {
          if (!header) { header = true; setStatus(`本机 ICMP → ${host} (${payload?.resolved || host})\n`); }
          sent++;
          if (payload?.ok) received++;
          sink(`${sent}  ${payload?.output || ''}\n`);
        }
        if (stopped) break;
        await new Promise(resolve => setTimeout(resolve, 1000));
        if (stopped || runId !== generation) break;
      }
    } finally {
      document.removeEventListener('keydown', onKey);
    }
    if (!header) setStatus(`本机 ICMP → ${host}\n`);
    sink(`收到 ${received} / ${sent} 个响应（已停止）\n\n`);
  }
  function reflect() {
    if (!view?.isConnected) return;
    // The scratchpad belongs to the disconnected terminal; a live session owns
    // the area, and returning to no session starts from the welcome screen.
    if (current()) { generation++; view.textContent = ''; view.hidden = true; return; }
    if (view.textContent) { view.hidden = false; $('#welcome-state').hidden = true; }
    else { view.hidden = true; $('#welcome-state').hidden = false; }
  }
  function handleClear() {
    if (current() || !view?.isConnected || view.hidden || !view.textContent) return false;
    generation++;
    view.textContent = '';
    view.hidden = true;
    $('#welcome-state').hidden = false;
    return true;
  }
  return { run, reflect, handleClear, termSink, commands };
})();
