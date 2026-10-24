import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const source = fs.readFileSync(new URL('../../web/js/workspace-refinements.js', import.meta.url), 'utf8');
const functions = source.slice(source.indexOf('let installerQuitRequested'), source.indexOf('function initializeRefinements()'));
const handlersStart = source.indexOf("  $('#cancel-exit').onclick");
const handlers = source.slice(handlersStart, source.indexOf('\n}', handlersStart));
function fixture({dirty=0,saving=0,transfers=0,startup=false}={}) {
  const nodes=new Map(),events=new Map(),timers=[];
  const state={dirty,saving,quits:0,cancels:0,flushes:0,toasts:[]};
  const $=key=>{if(!nodes.has(key))nodes.set(key,{open:false,hidden:false,textContent:'',showModal(){this.open=true},close(){this.open=false},focus(){this.focused=true}});return nodes.get(key)};
  const sessions=new Map([['ssh',{connected:true}]]),localTasks=new Map();
  for(let i=0;i<transfers;i++)localTasks.set(i,{status:'uploading'});
  const bridge={ConfirmQuit:async()=>{state.quits++},CancelQuit:async()=>{state.cancels++},WindowState:async()=>({quitPending:startup,installerQuitPending:startup})};
  const context=vm.createContext({$,sessions,localTasks,safe:f=>f,quitListenerReady:false,
    native:()=>bridge,closeSession:async id=>sessions.delete(id),dropSessionView:id=>sessions.delete(id),toast:text=>state.toasts.push(text),
    setTimeout:callback=>{const timer={callback,cancelled:false};timers.push(timer);return timer},clearTimeout:timer=>{if(timer)timer.cancelled=true},
    window:{runtime:{EventsOn:(name,callback)=>events.set(name,callback)},DengTextEditors:{unsavedCount:()=>state.dirty,savingCount:()=>state.saving},DengPortablePreferences:{flush:async()=>{state.flushes++}},DengCommandHistory:{flush:async()=>{}},DengSecurityLock:{isLocked:()=>false,confirmExit:()=>false}}});
  vm.runInContext(functions+'\n'+handlers,context);
  return {context,state,$,events,sessions,localTasks,async tick(){const timer=timers.shift();assert(timer,'expected timer');if(!timer.cancelled)await timer.callback();await new Promise(resolve=>setImmediate(resolve))}};
}

{
  const f=fixture();await f.context.requestQuit({installer:true});assert.equal(f.state.quits,1);assert.equal(f.sessions.size,0);assert.equal(f.$('#exit-dialog').open,false);
}
{
  const f=fixture({dirty:1});await f.context.requestQuit({installer:true});assert.equal(f.state.quits,0);assert.equal(f.sessions.size,1);assert.equal(f.$('#exit-dialog').open,true);assert.match(f.$('#confirm-exit').textContent,/丢弃/);assert.equal(f.$('#cancel-exit').textContent,'返回保存');
  await f.$('#cancel-exit').onclick();assert.equal(f.state.cancels,1);assert.equal(f.sessions.size,1);
}
{
  const f=fixture({transfers:1});await f.context.requestQuit({installer:true});assert.equal(f.$('#wait-exit-transfers').hidden,false);f.$('#wait-exit-transfers').onclick();assert.equal(f.state.quits,0);f.localTasks.get(0).status='done';await f.tick();assert.equal(f.state.quits,1);
}
{
  const f=fixture({transfers:1});await f.context.initializeQuitConfirmation();await f.context.requestQuit({installer:true});f.$('#wait-exit-transfers').onclick();f.events.get('dengshell:installer-quit-cancelled')();f.localTasks.get(0).status='done';await f.tick();assert.equal(f.state.quits,0);assert.equal(f.sessions.size,1);
}
{
  const f=fixture({saving:1});await f.context.requestQuit({installer:true});assert.equal(f.state.quits,0);assert.equal(f.state.cancels,0);f.state.saving=0;await f.tick();assert.equal(f.state.quits,1);
}
{
  const f=fixture({startup:true});await f.context.initializeQuitConfirmation();await new Promise(resolve=>setImmediate(resolve));assert.equal(f.state.quits,1);
}
{
  const f=fixture();await f.context.requestQuit();assert.equal(f.state.quits,0);assert.equal(f.$('#exit-dialog').open,true);assert.match(f.$('#exit-description').textContent,/其他独立窗口会保持运行/);
}
{
  const f=fixture();f.context.window.DengPortablePreferences.flush=async()=>{f.state.dirty=1};await f.context.requestQuit({installer:true});assert.equal(f.state.quits,0);assert.equal(f.sessions.size,1);assert.equal(f.state.cancels,1);
}
{
  const f=fixture();let restored=false;f.context.window.DengSecurityLock={isLocked:()=>true,confirmExit:()=>true,cancelExitConfirmation:()=>{restored=true}};
  await f.context.initializeQuitConfirmation();await f.context.requestQuit({installer:true});assert.equal(f.state.quits,0);f.events.get('dengshell:installer-quit-cancelled')();assert.equal(restored,true);assert.equal(f.sessions.size,1);
}
console.log('PASS: installer auto-exit, dirty edit protection, transfer wait/cancel, save wait, startup recovery, ordinary quit and late edits');
