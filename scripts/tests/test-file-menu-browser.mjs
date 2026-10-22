import assert from 'node:assert/strict';
const [url, module] = process.argv.slice(2);
// Windows absolute paths are not valid ESM specifiers; only this test runs on
// both hosts, so normalize the puppeteer-core entry to a file URL.
const { default: puppeteer } = await import(/^[a-zA-Z]:[\\/]/.test(module) ? new URL('file:///' + module.replace(/\\/g, '/')) : module);
const browser = await puppeteer.launch({executablePath:process.env.CHROME_BIN || '/usr/bin/google-chrome',headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
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
    window.chooseUploads = async (target, directory) => { (calls.uploads ||= []).push([target.id, directory]); };
    window.sendInput = (_state, data) => { (calls.sent ||= []).push(data); };
    const realDownload = DengFileTools.downloadFile;
    DengFileTools.downloadFile = async (_state, path) => { (calls.downloads ||= []).push(path); };
    try {
      window.DengZmodemBridge.noteCommand(state, 'sz -a b.txt');
      window.DengZmodemBridge.observe(state, new TextEncoder().encode('rz\r**\x18B00000000000000\r\x8a\x11'));
      await new Promise(resolve => setTimeout(resolve, 80));
      window.DengZmodemBridge.noteCommand(state, 'rz');
      window.DengZmodemBridge.observe(state, new TextEncoder().encode('rz waiting to receive.**\x18B0100000023be50\r\x8a\x11'));
      await new Promise(resolve => setTimeout(resolve, 80));
      return { typeofBridge: typeof window.DengZmodemBridge.observe, typeofChooser: typeof chooseUploads, calls, notes: window.qaNotes || [] };
    } finally { DengFileTools.downloadFile = realDownload; }
  });
  assert.equal(bridge.typeofBridge, 'function'); assert.equal(bridge.typeofChooser, 'function');
  assert.deepEqual(bridge.calls.downloads, ['/srv/logs/b.txt'], 'sz 通过 SFTP 下载终端目录里的同名文件');
  assert.deepEqual(bridge.calls.uploads, [['zmodem-fake', '/srv/logs']], 'rz 走 SFTP 上传到终端目录');
  assert.deepEqual(bridge.calls.sent, ['\x18\x18', '\x18\x18'], '两次握手各中止一次');
  assert.equal(bridge.notes.length, 2);
  assert.match(bridge.notes.join('\n'), /SFTP 下载[\s\S]*SFTP 上传/);
  assert.deepEqual(errors,[]);
  console.log('PASS: 文件命令全部在右键菜单内（下载与打包并下载分开、目录只打包、多选批量），状态栏无操作按钮；rz / sz 在页面里桥接到 SFTP 上传与下载。');
} finally { await browser.close(); }
