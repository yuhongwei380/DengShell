import fs from 'node:fs';
import assert from 'node:assert/strict';
const [stage,chrome,modulePath]=process.argv.slice(2);
const {default:puppeteer}=await import(modulePath),fixture=JSON.parse(fs.readFileSync(stage+'/browser-fixture.json'));
const browser=await puppeteer.launch({executablePath:chrome,headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
try {
 const page=await browser.newPage();await page.setViewport({width:1280,height:800});await page.goto(fixture.url,{waitUntil:'networkidle0'});
 const lines=await page.evaluate(async()=>{
  document.querySelectorAll('dialog[open]').forEach(d=>d.close());
  // A detached xterm is enough: the bug is how termSink feeds newlines to the
  // terminal parser, which happens before any renderer is involved.
  const term=new Terminal({cols:80,rows:24,allowProposedApi:false});
  const sink=window.DengLocalPrompt.termSink(term);
  sink('[本机] $ ping baidu.com\n','cmd');
  sink('本机 ICMP → baidu.com (110.242.74.102)\n1  110.242.74.102  51.00 ms\n2  110.242.74.102  50.00 ms\n收到 2 / 2 个响应\n\n');
  await new Promise(resolve=>term.write('',resolve)); // drain the async write queue
  const out=[];
  for(let i=0;i<term.buffer.active.length;i++){const line=term.buffer.active.getLine(i);if(line){const text=line.translateToString(true);if(text)out.push(text);}}
  term.dispose();
  return out;
 });
 assert.deepEqual(lines,[
  '[本机] $ ping baidu.com',
  '本机 ICMP → baidu.com (110.242.74.102)',
  '1  110.242.74.102  51.00 ms',
  '2  110.242.74.102  50.00 ms',
  '收到 2 / 2 个响应',
 ],'multi-line local prompt output must stay left-aligned in the terminal');
 fs.writeFileSync(stage+'/local-prompt-terminal.json',JSON.stringify({passed:true,noStaircase:true}));console.log('PASS local prompt terminal output uses CRLF and never staircases');
}finally{await browser.close()}
