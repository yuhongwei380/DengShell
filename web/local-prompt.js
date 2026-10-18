'use strict';
// Xshell-style local prompt: while no session is active, "ping 主机" typed in
// the command bar prints its result straight into the terminal area instead of
// opening the quick-connect dialog. "ping 主机 -t" probes continuously until
// stopped. The dialog keeps its own in-form probe.
window.DengLocalPrompt = (() => {
  let view = null, generation = 0;
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
  function line(box, text, className) {
    const span = node('span', className || '');
    span.textContent = text;
    box.append(span);
    trimHistory(box);
    box.scrollTop = box.scrollHeight;
    return span;
  }
  function parse(command) {
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
  async function run(command) {
    const box = ensure();
    box.hidden = false;
    $('#welcome-state').hidden = true;
    const runId = ++generation; // supersedes any running continuous probe
    line(box, `[本机] $ ${command.trim()}\n`, 'local-prompt-line');
    const parsed = parse(command);
    if (!parsed) { line(box, 'ping 后只接受主机名或 IP，可加 -t 持续探测。\n\n'); return; }
    if (parsed.continuous) { await continuous(box, parsed.host, runId); return; }
    const pending = line(box, '正在从本机探测…\n');
    try {
      const payload = await post('/api/quick-connect/ping', { host: parsed.host });
      pending.textContent = `${(payload?.output || '').trimEnd()}\n\n`;
    } catch (error) {
      pending.textContent = `${error.message || error}\n\n`;
    }
    if (runId !== generation) pending.textContent = '（已被新命令取代）\n';
    box.scrollTop = box.scrollHeight;
  }
  async function continuous(box, host, runId) {
    const status = line(box, '持续探测中，可随时停止…\n');
    const actions = node('div', 'local-prompt-actions');
    const stop = node('button', 'local-prompt-stop');
    stop.type = 'button';
    stop.textContent = '停止';
    let stopped = false;
    stop.onclick = () => { stopped = true; stop.disabled = true; };
    actions.append(stop);
    box.append(actions);
    box.scrollTop = box.scrollHeight;
    let sent = 0, received = 0, header = false;
    for (;;) {
      let payload = null, failure = null;
      try { payload = await post('/api/quick-connect/ping', { host, count: 1 }); }
      catch (error) { failure = error; }
      if (runId !== generation) return; // a newer command owns the scratchpad
      if (failure) {
        sent++;
        line(box, `${sent}  ${failure.message || failure}\n`);
      } else {
        if (!header) { header = true; status.textContent = `本机 ICMP → ${host} (${payload?.resolved || host})\n`; }
        sent++;
        if (payload?.ok) received++;
        line(box, `${sent}  ${payload?.output || ''}\n`);
      }
      box.append(actions);
      if (stopped) break;
      await new Promise(resolve => setTimeout(resolve, 1000));
      if (stopped || runId !== generation) break;
    }
    actions.remove();
    if (!header) status.textContent = `本机 ICMP → ${host}\n`;
    line(box, `收到 ${received} / ${sent} 个响应（已停止）\n\n`);
    box.scrollTop = box.scrollHeight;
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
  return { run, reflect, handleClear };
})();
