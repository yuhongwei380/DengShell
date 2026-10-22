import assert from 'node:assert/strict';
import {readFile,writeFile} from 'node:fs/promises';
import fs from 'node:fs';
import {join,isAbsolute} from 'node:path';
import {pathToFileURL} from 'node:url';
import {spawn} from 'node:child_process';
const [stage,chrome,modulePath]=process.argv.slice(2);
const {default:puppeteer}=await import(isAbsolute(modulePath)?pathToFileURL(modulePath).href:modulePath);
const fixture=JSON.parse(await readFile(join(stage,'browser-fixture.json'),'utf8'));
await fetch(new URL('/api/appearance/patch',fixture.url),{method:'POST',headers:{'Content-Type':'application/json','X-CloudShell-Token':fixture.token},body:JSON.stringify({onboardingCompleted:true,uiScale:1})});
const profileDir=fs.mkdtempSync(join(stage,'edge-monitor-'));
const child=spawn(chrome,['--headless=new','--no-sandbox','--disable-dev-shm-usage','--disable-background-networking','--remote-debugging-port=0','--user-data-dir='+profileDir,'about:blank'],{stdio:'ignore',windowsHide:true});
let browser;
try {
 let endpoint;
 for(let i=0;i<100;i++){try{const [port,suffix]=fs.readFileSync(join(profileDir,'DevToolsActivePort'),'utf8').trim().split(/\r?\n/);endpoint=`ws://127.0.0.1:${port}${suffix}`;break;}catch{}await new Promise(resolve=>setTimeout(resolve,100));}
 assert(endpoint,'browser started');
 browser=await puppeteer.connect({browserWSEndpoint:endpoint});
 const page=await browser.newPage();await page.setViewport({width:1440,height:1000});await page.emulateMediaFeatures([{name:'prefers-reduced-motion',value:'reduce'}]);const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(fixture.url,{waitUntil:'networkidle0'});
 const result=await page.evaluate(()=>{
  const original=activeID, state={connected:true,stats:{processSample:{available:true,sampledAt:new Date().toISOString(),intervalMilliseconds:5000},processes:Array.from({length:20},(_,i)=>({pid:i+1,name:'process-'+(i+1),cpu:i+1,cpuReady:true,memory:1024*(21-i),memoryReady:true,memoryEstimated:true})),processMemoryTop:Array.from({length:5},(_,i)=>({pid:100+i,name:'memory-'+i,cpu:0,cpuReady:true,memory:1024*(100-i),memoryReady:true,memoryEstimated:true}))},processSort:{key:'cpu',ascending:true}};
  sessions.set('monitor-fixture',state); activeID='monitor-fixture';
  const rows=()=>[...document.querySelectorAll('#process-list tr')].map(r=>Number(r.dataset.pid));
  try {
   renderProcesses(state.stats);const cpu=rows();sortProcesses('memory');const memory=rows();
   state.stats={processSample:{available:false,error:'fixture timeout'}};renderProcesses(state.stats);const cached=rows(),cacheLabel=document.querySelector('#process-count').textContent;
   return {cpu,memory,cached,cacheLabel,alwaysVisible:document.querySelector('.process-details').tagName==='SECTION'&&document.querySelector('.process-table').getBoundingClientRect().height>0,memoryCells:[...document.querySelectorAll('#process-list tr')].map(row=>row.cells[0].textContent),trafficWindow:trafficWindowMilliseconds};
  } finally {activeID=original; sessions.delete('monitor-fixture');}
 });
 assert.deepEqual(result.cpu,[20,19,18,17,16]);assert.deepEqual(result.memory,[100,101,102,103,104]);assert.deepEqual(result.cached,result.memory);assert.match(result.cacheLabel,/保留缓存/);assert(result.alwaysVisible);assert.deepEqual(result.memoryCells,Array(5).fill('—'),'estimated memory must not be presented as exact RSS');assert.equal(result.trafficWindow,60000);
 await (await page.$('.process-details')).screenshot({path:join(stage,'process-top-five.png')});
 // Consume a snapshot from the Go parser regression, then check the actual
 // summary in both themes, tab changes, missing data and unchanged I/O rates.
 const diskFixture=await readFile(join(stage,'disk-fixture.json'),'utf8').then(JSON.parse).catch(error=>{if(error.code==='ENOENT')return {cores:4,cpu:0.6,memoryUsed:3.9*1024**3,memoryTotal:7.8*1024**3,swapUsed:0,swapTotal:0,uptime:90000,disks:[{path:'/',used:10*1024**3,total:100*1024**3,available:90*1024**3},{path:'/home',used:1024**4,total:3*1024**4,available:2*1024**4},{path:'/boot/efi',used:6*1024**2,total:1024**3,available:1018*1024**2}]};throw error;});
 const diskChecks=[];
 if(diskFixture){
  await page.evaluate(()=>{document.querySelectorAll('dialog[open]').forEach(d=>d.close());setDrawer(false);});
  for(const theme of ['dark','light']){
   const check=await page.evaluate(({stats,theme})=>{
    CloudShellTheme.set(theme);
    stats.sampleReady=true;stats.diskRead=326.3*1024;stats.diskWrite=9.2*1024**2;
    const text=id=>document.querySelector('#disk-'+id).textContent;
    const capacity=()=>document.querySelector('#disk-meter em').textContent;
    renderMonitor(stats);
    const meter=document.querySelector('#disk-meter'),io=document.querySelector('.disk-io'),network=document.querySelector('.traffic-card');
    const used=stats.disks.reduce((sum,disk)=>sum+disk.used,0),total=stats.disks.reduce((sum,disk)=>sum+disk.total,0);
    const totals={capacity:capacity(),percent:meter.querySelector('b').textContent,read:text('read'),write:text('write'),tooltip:meter.title};
    const expected=`${prettySize(used)} / ${prettySize(total)}`;
    const layout={oldCard:!!document.querySelector('.disk-section,.disk-summary'),lastResource:meter.parentElement.nextElementSibling.className==='network-grid',ioAboveNetwork:io.nextElementSibling===network,matchedStyle:getComputedStyle(meter).height===getComputedStyle(document.querySelector('#cpu-meter')).height};
    renderMonitor({...stats,disks:[{path:'/',used:1024**3,total:10*1024**3,available:9*1024**3}]});
    const otherTab=capacity();renderMonitor(null);const empty=[capacity(),meter.querySelector('b').textContent,text('read'),text('write')],emptyWidth=meter.querySelector('i').style.width;
    renderMonitor({...stats,disks:[{path:'/',used:7*1024**3,total:29*1024**3,available:21.9*1024**3}]});
    const example={capacity:capacity(),percent:meter.querySelector('b').textContent};
    renderMonitor({...stats,disks:[{path:'/',used:0,total:0,available:0}]});
    const zero=[capacity(),meter.querySelector('b').textContent,meter.querySelector('i').style.width];
    renderMonitor(stats);
    return {theme,totals,expected,expectedPercent:`${(used/total*100).toFixed(1)}%`,otherTab,empty,emptyWidth,example,zero,layout};
   },{stats:diskFixture,theme});
   assert.equal(check.totals.capacity,check.expected);assert.equal(check.totals.percent,check.expectedPercent);
   assert.match(check.totals.capacity,/TB$/);assert.match(check.totals.tooltip,/3 个/);assert.match(check.totals.tooltip,/\/home/);assert.match(check.totals.tooltip,/\/boot\/efi/);assert.match(check.totals.tooltip,/可用/);
   assert.equal(check.totals.read,'326.3 KB/s');assert.equal(check.totals.write,'9.2 MB/s');assert.equal(check.otherTab,'1.0 GB / 10.0 GB');assert.deepEqual(check.empty,Array(4).fill('—'));assert.equal(check.emptyWidth,'0%');
   assert.deepEqual(check.example,{capacity:'7.0 GB / 29.0 GB',percent:'24.1%'});assert.deepEqual(check.zero,['—','—','0%']);
   assert.deepEqual(check.layout,{oldCard:false,lastResource:true,ioAboveNetwork:true,matchedStyle:true});
   await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
   await (await page.$('.sidebar')).screenshot({path:process.env.DENGSHELL_MONITOR_SCREENSHOT?process.env.DENGSHELL_MONITOR_SCREENSHOT+'-'+theme+'.png':join(stage,'disk-capacity-'+theme+'.png')});
   diskChecks.push(check);
  }
 }
 assert.deepEqual(errors,[]);await writeFile(join(stage,'process-ui-test.json'),JSON.stringify({passed:true,...result,diskChecks,errors},null,2));console.log('PASS: permanent CPU/memory top five, stale cache label, unavailable exact RSS, 60-second window'+(diskChecks.length?', disk capacity sums in both themes and independent I/O rates.':'.'));
} finally {try{await browser?.close();}catch{}try{child.kill();}catch{}try{fs.rmSync(profileDir,{recursive:true,force:true,maxRetries:10,retryDelay:150});}catch{}}
