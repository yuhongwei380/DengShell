import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import vm from 'node:vm';
const source=readFileSync(new URL('../../web/js/workspace-refinements.js',import.meta.url),'utf8');
// The whole history region: the per-server queue, the exported DengCommandHistory
// object, and the disconnected command bar's own list.
const history=source.slice(source.indexOf('function historyKey('),source.indexOf('async function copyText('));
assert(history.includes('function recallLocalCommand(')&&history.includes('recordLocal: recordLocalCommand'),'the local history block must stay next to the command history code');
// Fake of the Go store: append with consecutive dedup, keep the newest 50,
// bump the revision per mutation, and replay the same operation ID as a no-op.
let entries=[],revision=0,loseAck=false;const operations=new Set(),notices=[];
const stored=()=>({entries:[...entries],revision});
const context=vm.createContext({structuredClone,crypto:{randomUUID},Map,Set,Promise,
  toast:message=>notices.push(message),
  window:{DengPortablePreferences:{cache(){}}},
  readSaved:(key,fallback)=>fallback,$:()=>({open:false}),renderCommandHistory(){},sessions:new Map(),historySession:null,
  api:async(path,options={})=>{
    if(path!=='/api/local-history')throw new Error('unexpected request '+path);
    if(!options.method)return stored();
    const {operation,command}=JSON.parse(options.body);
    if(!/^[A-Za-z0-9_-]{24,96}$/.test(operation)||!String(command||'').trim())throw new Error('本机历史请求无效');
    if(operations.has(operation))return stored();
    operations.add(operation);
    if(!entries.length||entries.at(-1)!==command.trim())entries.push(command.trim());
    if(entries.length>50)entries=entries.slice(-50);
    revision++;
    if(loseAck){loseAck=false;throw new Error('已提交但应答丢失');}
    return stored();
  },
});
vm.runInContext(history+'\nthis.history=()=>globalCommandHistory;',context);
const api=()=>context.window.DengCommandHistory;

// 欢迎页 / 已退出会话里回车的内容进入独立的 50 条列表。
for(let i=1;i<=60;i++)await api().recordLocal(`ping host-${String(i).padStart(2,'0')}`);
assert.equal(api().localEntries().length,50,'only the newest 50 local commands survive');
assert.equal(api().localEntries()[0],'ping host-11','the oldest retained command');
assert.equal(api().localEntries().at(-1),'ping host-60','the newest command stays last');
assert.deepEqual(Array.from(api().localEntries()),stored().entries,'the store holds the same newest 50');

// Repeating the newest command adds nothing; typing an older one again appends it.
await api().recordLocal('ping host-60');
assert.equal(api().localEntries().length,50,'repeating the newest command must not grow the list');
await api().recordLocal('ping host-11');
assert.equal(api().localEntries()[0],'ping host-12','the oldest entry drops out, not the newest');
assert.equal(api().localEntries().at(-1),'ping host-11','an older command typed again is recorded again');

// ↑ walks newest → oldest, ↓ walks back, past the newest the box clears, and
// editing a recalled command restarts at the newest entry.
const box={value:'',direction:-1};
const recall=()=>{box.value=context.window.DengCommandHistory.recallLocal(box.value,box.direction);return box.value;};
assert.equal(recall(),'ping host-11','↑ offers the command just run, like a shell');
box.direction=-1;assert.equal(recall(),'ping host-60');
box.direction=1;assert.equal(recall(),'ping host-11');
assert.equal(recall(),'','↓ past the newest entry clears the input');
box.value='ping host-11 -c 4';box.direction=-1;
assert.equal(recall(),'ping host-11','edited text restarts at the newest command');

// A lost acknowledgement keeps the command visible, and the retry adds it once.
loseAck=true;
await api().recordLocal('ssh root@192.0.2.44');
assert(notices.some(text=>text.includes('已提交但应答丢失')),'a failed local save is reported to the user');
assert.equal(api().localEntries().at(-1),'ssh root@192.0.2.44','an unacknowledged command stays visible');
const committed=stored().revision;
await api().flush();
assert.equal(stored().revision,committed,'replaying the same operation ID changes nothing in the store');
assert.equal(stored().entries.filter(command=>command==='ssh root@192.0.2.44').length,1,'the retry records the command exactly once');
assert.equal(api().localEntries().at(-1),'ssh root@192.0.2.44','the acknowledged list matches what the user typed');

// An older window's snapshot must not roll back the shared list, and the startup
// config payload seeds the same list.
api().acceptConfig({localHistory:{entries:['stale'],revision:0}});
assert.equal(api().localEntries().at(-1),'ssh root@192.0.2.44','stale snapshots are rejected');
api().acceptConfig({localHistory:stored()});
assert.equal(api().localEntries().length,stored().entries.length,'the startup config carries the local list');
console.log('PASS: 未连接时命令条保有独立的最近 50 条历史，重发不重复，↑/↓ 正常回溯。');
