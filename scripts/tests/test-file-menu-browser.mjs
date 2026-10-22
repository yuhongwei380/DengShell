import assert from 'node:assert/strict';
const [url, module] = process.argv.slice(2);
// Windows absolute paths are not valid ESM specifiers; only this test runs on
// both hosts, so normalize the puppeteer-core entry to a file URL.
const { default: puppeteer } = await import(/^[a-zA-Z]:[\\/]/.test(module) ? new URL('file:///' + module.replace(/\\/g, '/')) : module);
const browser = process.env.DENGSHELL_BROWSER_WS
  ? await puppeteer.connect({browserWSEndpoint:process.env.DENGSHELL_BROWSER_WS})
  : await puppeteer.launch({executablePath:process.env.CHROME_BIN || '/usr/bin/google-chrome',headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
let page;
try {
  page = await browser.newPage();
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const origin=new URL(url).origin, token=new URL(url).hash.split('=')[1];
  await fetch(origin+'/api/appearance/patch',{method:'POST',headers:{'Content-Type':'application/json','X-CloudShell-Token':token},body:JSON.stringify({onboardingCompleted:true,uiScale:1})});
  await page.setViewport({width:1280,height:800});await page.goto(url,{waitUntil:'networkidle0'});
  const result = await page.evaluate(async () => {
    document.querySelectorAll('dialog[open]').forEach(d=>d.close());
    const state={id:'menu-fake',profileId:'menu-fake-profile',connected:true,sftpAvailable:true,ready:true,cwd:'/srv',home:'/srv',
      entries:[{name:'a.txt',kind:'file',bytes:4,modifiedAt:0},{name:'folder',kind:'folder',bytes:0,modifiedAt:0}],
      term:{options:{},modes:{},focus(){},refresh(){},dispose(){}},host:document.createElement('div')};
    sessions.set(state.id,state);activeID=state.id;
    const open=(target,isDirectory,targets=[])=>{DengFileTools.showMenu({preventDefault(){},clientX:80,clientY:80},current(),target,isDirectory,null,targets);return {items:[...document.querySelectorAll('.file-context-menu > button')].map(b=>b.textContent.trim()),submenus:[...document.querySelectorAll('.file-context-menu .file-submenu-wrap > button')].map(b=>b.textContent.trim())};};
    const clickItem=label=>{const button=[...document.querySelectorAll('.file-context-menu > button')].find(b=>b.textContent.trim()===label);if(!button)throw new Error('菜单缺少 '+label);button.click();};
    const file=open('/srv/a.txt',false), folder=open('/srv/folder',true), batch=open('/srv/a.txt',false,[{path:'/srv/a.txt',isDirectory:false},{path:'/srv/folder',isDirectory:true}]);
    const fileMenu=file.items, folderMenu=folder.items, batchMenu=batch.items;
    const statusButtons=document.querySelectorAll('.file-status button').length;
    window.qaDownloads=[],window.qaArchives=[];
    window.go={desktop:{Desktop:{
      Download:async(id,path)=>{qaDownloads.push({id,path});return '/fixture/a.txt';},
      DownloadSelectionArchive:async(id,paths)=>{qaArchives.push({id,paths});return '/fixture/output.tar.gz';},
    }}};
    open('/srv/a.txt',false);clickItem('下载');
    await new Promise(resolve=>setTimeout(resolve,60));
    open('/srv/a.txt',false);clickItem('打包并下载');
    await new Promise(resolve=>setTimeout(resolve,60));
    delete window.go;
    return {fileMenu,submenus:file.submenus,folderMenu,batchMenu,statusButtons,downloads:qaDownloads,archives:qaArchives};
  });
  assert.equal(result.statusButtons,0,'文件状态栏不再常驻下载 / 重命名 / 删除按钮');
  assert.deepEqual(result.fileMenu,['打开','重命名','删除','快速删除','下载','打包并下载','复制路径','文件权限'],'单个文件的右键菜单顺序');
  assert.deepEqual(result.submenus,['打开方式'],'打开方式是子菜单，不占顶层条目');
  assert.ok(result.folderMenu.includes('打包并下载')&&!result.folderMenu.includes('下载'),'目录只提供打包下载');
  assert.deepEqual(result.batchMenu,['删除选中的 2 项','打包并下载 2 项','复制所选路径'],'多选的右键菜单不直接下载单个文件');
  assert.deepEqual(result.downloads,[{id:'menu-fake',path:'/srv/a.txt'}],'下载走单个文件的直接下载通道');
  assert.deepEqual(result.archives,[{id:'menu-fake',paths:['/srv/a.txt']}],'打包并下载仍是单独的通道');
  // The lrzsz bridge has to be loaded by the page itself and reach the real
  // upload / cancel globals, so the wiring cannot rot silently.
  const bridge = await page.evaluate(async () => {
    const state = { id: 'zmodem-fake', connected: true, ready: true, closed: false, serial: false, localOnly: false,
      cwd: '/srv/app', terminalDirectory: '/srv/logs', sftpAvailable: true, shellIntegration: { atPrompt: false },
      term: { writeln: line => { window.qaNotes = (window.qaNotes || []).concat(line); } } };
    sessions.set(state.id, state); activeID = state.id;
    const calls = {};
    const realUploads = window.chooseUploads, realInput = window.sendInput;
    window.chooseUploads = async (target, directory) => { (calls.uploads ||= []).push([target.id, directory]); };
    window.sendInput = (_state, data) => { (calls.sent ||= []).push(data); };
    const realDownload = DengFileTools.downloadFile;
    DengFileTools.downloadFile = async (_state, path) => { (calls.downloads ||= []).push(path); };
    try {
      window.DengZmodemBridge.noteCommand(state, 'sz -a b.txt');
      window.DengZmodemBridge.observe(state, new TextEncoder().encode('rz\r**\x18B00000000000000\r\x8a\x11'));
      await new Promise(resolve => setTimeout(resolve, 1200));
      window.DengZmodemBridge.observe(state, new TextEncoder().encode('**\x18B00000000000000\r\x8a\x11'));
      await new Promise(resolve => setTimeout(resolve, 300));
      window.DengZmodemBridge.noteCommand(state, 'rz');
      window.DengZmodemBridge.observe(state, new TextEncoder().encode('rz waiting to receive.**\x18B0100000023be50\r\x8a\x11'));
      await new Promise(resolve => setTimeout(resolve, 1200));
      return { typeofBridge: typeof window.DengZmodemBridge.observe, typeofChooser: typeof chooseUploads, calls, notes: window.qaNotes || [] };
    } finally { DengFileTools.downloadFile = realDownload; window.chooseUploads = realUploads; window.sendInput = realInput; }
  });
  assert.equal(bridge.typeofBridge, 'function'); assert.equal(bridge.typeofChooser, 'function');
  assert.deepEqual(bridge.calls.downloads, ['/srv/logs/b.txt'], 'sz 通过 SFTP 下载终端目录里的同名文件（重试的广告不重复下载）');
  assert.deepEqual(bridge.calls.uploads, [['zmodem-fake', '/srv/logs']], 'rz 走 SFTP 上传到终端目录');
  assert.equal(bridge.calls.sent.length, 7, '两次尝试各补发中止序列，重试的广告只补发一次');
  assert.ok(bridge.calls.sent.every(value => value === '\x18\x18'), '只发送中止字节');
  assert.equal(bridge.notes.length, 2, '一次尝试只提示一次');
  assert.match(bridge.notes.join('\n'), /SFTP 下载[\s\S]*SFTP 上传/);
  // Real socket handler + real xterm OSC parser: a single SSH output frame can
  // contain command/cwd reports and the first sz header. No retry is sent here;
  // the first frame must download the right file immediately.
  const coalesced = await page.evaluate(async () => {
    const originalSocket = window.WebSocket, realDownload = DengFileTools.downloadFile;
    const inputs = [], downloads = [];
    window.WebSocket = class {
      static OPEN = 1;
      readyState = 1;
      send(value) { inputs.push(JSON.parse(value)); }
      close() {}
    };
    const state = makeSessionState({ id: 'coalesced-sz', profileId: 'coalesced-profile', home: '/old-directory', sftpAvailable: true });
    sessions.set(state.id, state);
    try {
      createTerminal(state);
      state.ready = true;
      const nonce = 'a'.repeat(48);
      acceptShellIntegration(state, { nonce, shell: 'bash' });
      let started;
      const downloadStarted = new Promise(resolve => { started = resolve; });
      DengFileTools.downloadFile = async (_state, path) => { downloads.push(path); started(); };
      const osc = (kind, value) => `\x1b]777;DengShell;${kind};${nonce}${value === undefined ? '' : ';' + btoa(value)}\x07`;
      const frame = osc('busy') + osc('command', 'sz test.yml') + osc('cwd', '/srv/logs') + 'rz\r**\x18B00000000000000\r\x8a\x11';
      const start = performance.now();
      state.ws.onmessage({ data: new TextEncoder().encode(frame).buffer });
      await Promise.race([downloadStarted, new Promise((_, reject) => setTimeout(() => reject(new Error('首帧未触发 sz 下载：' + JSON.stringify({integration:state.shellIntegration,directory:state.terminalDirectory,inputs,lines:state.term.buffer.active.getLine(0)?.translateToString()}))), 2000))]);
      const elapsed = performance.now() - start;
      // A real prompt followed by another command invalidates the old CAN timers.
      state.ws.onmessage({ data: new TextEncoder().encode(osc('prompt') + osc('busy') + osc('command', 'vim next.txt')).buffer });
      await new Promise(resolve => setTimeout(resolve, 600));
      return { downloads, elapsed, cancellations: inputs.filter(value => value.type === 'input').map(value => value.data) };
    } finally {
      state.closed = true;
      DengZmodemBridge.clear(state);
      state.term?.dispose(); state.host?.remove(); sessions.delete(state.id);
      window.WebSocket = originalSocket; DengFileTools.downloadFile = realDownload;
    }
  });
  assert.deepEqual(coalesced.downloads, ['/srv/logs/test.yml'], '同帧内的命令和目录先于握手生效');
  assert.ok(coalesced.elapsed < 500, `弹窗不应等待 750ms 的中止序列：${coalesced.elapsed}ms`);
  assert.deepEqual(coalesced.cancellations, ['\x18\x18'], '收到提示符后不再向下一条命令补发 CAN');
  assert.deepEqual(errors,[]);
  console.log(`PASS: 文件右键下载与打包、rz / sz 桥接；同帧 OSC + sz 首次触发 ${coalesced.elapsed.toFixed(1)}ms，无重试等待和残留 CAN。`);
} finally { await browser.close(); }
