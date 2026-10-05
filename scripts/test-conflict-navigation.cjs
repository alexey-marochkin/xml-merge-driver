const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
function fixture(count=300){
 const elements=new Map();
 class Element{
  constructor(){this.children=[];this.events={};this.classList={add(){},remove(){},toggle(){}};this.open=false}
  append(...nodes){this.children.push(...nodes)}
  replaceChildren(...nodes){this.children=nodes}
  addEventListener(name,fn){this.events[name]=fn}
  setAttribute(){}
  close(){this.open=false}
  showModal(){this.open=true}
  querySelector(){return null}
 }
 const $=id=>{if(!elements.has(id))elements.set(id,new Element());return elements.get(id)};
 const conflicts=Array.from({length:count},(_,i)=>({TargetID:'n'+Math.floor(i/2),Path:'/node/'+Math.floor(i/2),Reason:'Reason '+i}));
 const context=vm.createContext({$,state:{Conflicts:conflicts},node:(tag,text)=>{const el=new Element();el.textContent=text;return el},document:{querySelectorAll:()=>[]},window:{addEventListener(){}},requestAnimationFrame(){},confirm:()=>false,action:fn=>fn()});
 vm.runInContext(fs.readFileSync('internal/ui/assets/review.js','utf8'),context);
 const run=code=>vm.runInContext(code,context);
 run("reviewMap=new Map(state.Conflicts.map(c=>[c.TargetID,{ID:c.TargetID,Name:c.TargetID,Conflict:true}]));drawTree=()=>{};renderComparison=renderConflictSummary;");
 return {run,$,context};
}
test('300 conflicts occupy one summary; the full list is built only on request',()=>{
 const {run,$}=fixture();run('renderConflictSummary()');
 assert.equal($('conflictPosition').textContent,'1 из 300');
 assert.equal($('currentConflictReason').textContent,'Reason 0');
 assert.equal($('conflictList').children.length,0);
 run('renderConflictList()');assert.equal($('conflictList').children.length,300);
});
test('navigation visits separate reasons on the same node, wraps, and list selection updates the summary',async()=>{
 const {run,$}=fixture();await run("focusNode('n0',true,0)");
 await run("navigateReview('conflict',1)");
 assert.equal($('conflictPosition').textContent,'2 из 300');
 assert.equal($('currentConflictReason').textContent,'Reason 1');
 await run("navigateReview('conflict',-1)");await run("navigateReview('conflict',-1)");
 assert.equal($('conflictPosition').textContent,'300 из 300');
 run('renderConflictList()');await $('conflictList').children[11].events.click();
 assert.equal($('conflictPosition').textContent,'12 из 300');
 assert.equal(run('activeNode'),'n5');
});
test('selection survives list updates; removing the current conflict picks the next and zero hides the bar',()=>{
 const {run,$}=fixture(4);run('selectConflict(1);renderConflictSummary();state.Conflicts.shift();renderConflictSummary()');
 assert.equal($('conflictPosition').textContent,'1 из 3');
 assert.equal($('currentConflictReason').textContent,'Reason 1');
 run('state.Conflicts.shift();renderConflictSummary()');
 assert.equal($('currentConflictReason').textContent,'Reason 2');
 $('conflictListDialog').open=true;run('state.Conflicts=[];renderConflictSummary()');
 assert.equal($('conflicts').hidden,true);assert.equal($('conflictListDialog').open,false);
});
test('cancelling navigation with uncommitted text keeps the current conflict',async()=>{
 const {run,$}=fixture(4);await run("focusNode('n0',true,0)");
 run('draftDirty=true');await run("navigateReview('conflict',1)");
 assert.equal($('conflictPosition').textContent,'1 из 4');assert.equal(run('conflictCursor'),0);
});

test('move conflicts mark both full paths and navigation visits the actual elements',async()=>{
 const {run}=fixture(0);
 run(`reviewMap=new Map([
  ['root',{ID:'root',Children:['branch']}],
  ['branch',{ID:'branch',Parent:'root',Children:['from','to','unrelated']}],
  ['from',{ID:'from',Parent:'branch',Children:['old']}],
  ['old',{ID:'old',Parent:'from'}],
  ['to',{ID:'to',Parent:'branch',Children:['new']}],
  ['new',{ID:'new',Parent:'to'}],
  ['unrelated',{ID:'unrelated',Parent:'branch'}]
 ]);state.Conflicts=[{TargetID:'branch',RelatedIDs:['old','new','missing'],Reason:'Move'}];`);
 assert.deepEqual([...run('conflictBranches()')].sort(),['branch','from','new','old','root','to']);
 await run("focusNode('branch',true,0)");
 assert.equal(run('activeNode'),'old');
 for(const id of ['root','branch','from','to'])assert.equal(run(`expanded.has('${id}')`),true);
 await run("navigateReview('conflict',1)");assert.equal(run('activeNode'),'new');
 await run("navigateReview('conflict',1)");assert.equal(run('activeNode'),'old');
 await run("navigateReview('conflict',-1)");assert.equal(run('activeNode'),'new');
 run('state.Conflicts=[]');assert.equal(run('conflictBranches().size'),0);
});
