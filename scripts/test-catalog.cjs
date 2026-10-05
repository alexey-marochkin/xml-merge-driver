// State-transition tests for the actual catalog script, with DOM/network stand-ins.
// Run with node --test scripts/test-catalog.cjs (no additional packages).
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const assets=path.join(__dirname,'../internal/ui/assets');
const script=fs.readFileSync(path.join(assets,'catalog.js'),'utf8');
const html=fs.readFileSync(path.join(assets,'catalog.html'),'utf8');
async function fixture(){
 const all=[];
 class Element{
  constructor(tag='div'){this.tag=tag;this.children=[];this.dataset={};this.events={};this.value='';this.hidden=false;this.valid=true;this.classList={toggle(){}};all.push(this)}
  append(...nodes){for(const n of nodes){n.parent=this;this.children.push(n)}}
  replaceChildren(...nodes){this.children=[];this.append(...nodes)}
  addEventListener(name,fn){(this.events[name]??=[]).push(fn)}
  async fire(name,event={}){for(const f of this.events[name]||[])await f({preventDefault(){},...event})}
  setAttribute(){} focus(){} reportValidity(){return this.valid}
  showModal(){this.open=true} close(){this.open=false}
  remove(){this.parent.children=this.parent.children.filter(n=>n!==this)}
  querySelectorAll(selector){const match=/\[data-(field|flag)="([^"]+)"\]/.exec(selector),found=[];const walk=n=>{for(const c of n.children){if(match?c.dataset[match[1]]===match[2]:c.tag===selector)found.push(c);walk(c)}};walk(this);return found}
  querySelector(s){return this.querySelectorAll(s)[0]}
 }
 const ids=Object.fromEntries([...html.matchAll(/id="([^"]+)"/g)].map(m=>[m[1],new Element()]));
 for(const [id,element] of Object.entries(ids))element.id=id;
 ids.empty.append(new Element('h2'),new Element('p'));
 const doc=new Element();doc.getElementById=id=>ids[id];doc.createElement=tag=>new Element(tag);doc.querySelectorAll=()=>all;
 let disk={Version:2,Profiles:[{Root:'Root',Namespace:'',KnownElements:[{Name:'Known'}],Rules:[{Selector:'*',Mode:'text',Order:'significant',Fields:[]},{Selector:'Item',Mode:'text',Order:'default',Fields:[]}]}]},revision='1',fail=false,cancelled=false;
 const writes=[];const snapshot=()=>({Options:{Rules:'test.xml'},Database:structuredClone(disk),Revision:revision});
 const window=new Element();
 const context=vm.createContext({document:doc,window,location:{hash:'#test',pathname:'/'},sessionStorage:{getItem(){},setItem(){}},history:{replaceState(){}},structuredClone,fetch:async(url,options)=>{
  if(url==='/api/database'){
   if(fail)return {ok:false,json:async()=>({Error:'External change'})};
   const body=JSON.parse(options.body);assert.equal(body.Revision,revision);disk=body.Database;revision=String(+revision+1);writes.push(structuredClone(disk));
  }
  if(url==='/api/cancel')cancelled=true;
  return {ok:true,json:async()=>snapshot()};
 }});
 vm.runInContext(script,context);
 const flush=()=>new Promise(resolve=>setImmediate(resolve));await flush();
 const run=code=>vm.runInContext(code,context);
 return {ids,doc,window,run,flush,writes,disk:()=>disk,cancelled:()=>cancelled,fail:()=>fail=true};
}
test('navigation stages edits; one Ctrl+S writes all rules and keeps the catalog',async()=>{
 const f=await fixture();f.ids.order.value='insignificant';f.run('changed();navigate(()=>editRule(1))');
 assert.equal(f.disk().Profiles[0].Rules[0].Order,'significant');assert.equal(f.writes.length,0);
 f.ids.trim.checked=true;f.run('changed()');await f.doc.fire('keydown',{ctrlKey:true,key:'s'});await f.flush();
 assert.equal(f.writes.length,1);assert.equal(f.disk().Profiles[0].Rules[0].Order,'insignificant');assert.equal(f.disk().Profiles[0].Rules[1].TrimSpace,true);
 assert.equal(f.disk().Profiles[0].KnownElements[0].Name,'Known');assert.equal(f.run('dirty||staged'),false);assert.match(f.ids.status.textContent,/Сохранено/);
});
test('failed save keeps edits and cancellation leaves them intact',async()=>{
 const f=await fixture();f.ids.order.value='insignificant';f.run('changed()');f.fail();assert.equal(await f.run('saveFile()'),false);
 assert.equal(f.run('staged'),true);assert.equal(f.writes.length,0);
 f.run('requestClose()');assert.equal(f.ids.unsavedDialog.open,true);await f.ids.leaveCancel.fire('click');
 assert.equal(f.cancelled(),false);assert.equal(f.run('staged'),true);
 f.run('requestClose()');await f.ids.leaveSave.fire('click');assert.equal(f.cancelled(),false);assert.equal(f.run('staged'),true);
});
test('reload prompts before discarding; save-and-close includes current form',async()=>{
 const f=await fixture();f.ids.order.value='insignificant';f.run('changed()');await f.ids.reload.fire('click');assert.equal(f.ids.unsavedDialog.open,true);
 await f.ids.leaveDiscard.fire('click');await f.flush();assert.equal(f.run('dirty||staged'),false);assert.equal(f.ids.order.value,'significant');assert.equal(f.writes.length,0);
 f.ids.order.value='insignificant';f.run('changed();requestClose()');await f.ids.leaveSave.fire('click');await f.flush();
 assert.equal(f.disk().Profiles[0].Rules[0].Order,'insignificant');assert.equal(f.cancelled(),true);
});
test('invalid form blocks save and navigation without discarding the draft',async()=>{
 const f=await fixture();f.ids.editor.valid=false;f.run('changed();navigate(()=>editRule(1))');assert.equal(f.run('ruleIndex'),0);
 assert.equal(await f.run('saveFile()'),false);assert.equal(f.run('dirty'),true);assert.equal(f.writes.length,0);
});
test('new profiles and rule deletion wait for the shared Save command',async()=>{
 const f=await fixture();f.run('editRule(1)');await f.ids.accept.fire('click');
 assert.equal(f.disk().Profiles[0].Rules.length,2);assert.equal(f.run('state.Database.Profiles[0].Rules.length'),1);
 await f.ids.newProfile.fire('click');f.ids.rootName.value='AnotherRoot';f.ids.rootNamespace.value='';f.run('changed()');
 await f.run('saveFile()');assert.equal(f.writes.length,1);assert.equal(f.disk().Profiles.length,2);assert.equal(f.disk().Profiles[0].Rules.length,1);assert.equal(f.disk().Profiles[1].Rules[0].Selector,'*');
});
