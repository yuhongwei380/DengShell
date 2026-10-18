'use strict';
// Xshell-style local prompt: while no session is active, "ping 主机" typed in
// the command bar prints its result straight into the terminal area instead of
// opening the quick-connect dialog. The dialog keeps its own in-form probe.
window.DengLocalPrompt = (() => {
  let view = null, busy = false;
  function ensure() {
    if (view?.isConnected) return view;
    view = node('pre', 'local-prompt-output');
    view.id = 'local-prompt-output';
    view.setAttribute('role', 'log');
    view.hidden = true;
    $('#terminal-output').append(view);
    return view;
  }
  function trimHistory(box) { while (box.children.length > 40) box.firstChild.remove(); }
  async function run(command) {
    const trimmed = command.trim();
    const host = trimmed.replace(/^ping(?:\s+|$)/i, '').trim();
    if (busy) { toast('本机探测仍在进行，请稍候'); return; }
    const box = ensure();
    box.hidden = false;
    $('#welcome-state').hidden = true;
    const prompt = node('span', 'local-prompt-line');
    prompt.textContent = `[本机] $ ${trimmed}\n`;
    const result = node('span', 'local-prompt-result');
    result.textContent = '正在从本机探测…\n';
    box.append(prompt, result);
    trimHistory(box);
    box.scrollTop = box.scrollHeight;
    busy = true;
    try {
      const payload = host ? await post('/api/quick-connect/ping', { host }) : null;
      result.textContent = host ? `${(payload?.output || '').trimEnd()}\n\n` : 'ping 后需要主机名或 IP。\n\n';
    } catch (error) {
      result.textContent = `${error.message || error}\n\n`;
    } finally {
      busy = false;
      box.scrollTop = box.scrollHeight;
    }
  }
  function reflect() {
    if (!view?.isConnected) return;
    // The scratchpad belongs to the disconnected terminal; a live session owns
    // the area, and returning to no session starts from the welcome screen.
    if (current()) { view.textContent = ''; view.hidden = true; return; }
    if (view.textContent) { view.hidden = false; $('#welcome-state').hidden = true; }
    else { view.hidden = true; $('#welcome-state').hidden = false; }
  }
  function handleClear() {
    if (current() || !view?.isConnected || view.hidden || !view.textContent) return false;
    view.textContent = '';
    view.hidden = true;
    $('#welcome-state').hidden = false;
    return true;
  }
  return { run, reflect, handleClear };
})();
