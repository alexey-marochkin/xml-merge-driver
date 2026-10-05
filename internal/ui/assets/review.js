 'use strict';
let horizontalOffset=0,conflictCursor=0,conflictSelection='';
let reviewRows=[],reviewMap=new Map(),expanded=new Set(['root']),activeNode='',nodeDetail=null,draftDirty=false;
function resetReview(){
 conflictCursor=0;conflictSelection='';$('conflictListDialog').close();
 reviewRows=[];reviewMap=new Map();expanded=new Set(['root']);activeNode='';nodeDetail=null;draftDirty=false;
 $('nodeEditor').hidden=true;$('compareView').classList.remove('editing-text');$('reviewCounts').textContent='';
}
function renderComparison(){
 renderConflictSummary();
 refreshUndo();
 $('saveResult').disabled=!state.CanSave||draftDirty;
 $('status').textContent=draftDirty?'Текст изменён — примените его к узлу':state.CanSave?'Результат готов к сохранению':'Переносите изменения стрелками в центр';
 $('reviewCounts').textContent=`Конфликтов: ${(state.Conflicts||[]).length} · Изменённых узлов: ${state.Changes||0} · Решений: ${state.Decisions||0}`;
 for(const id of ['prevConflict','nextConflict'])$(id).disabled=!state.Conflicts?.length;
 for(const id of ['prevChange','nextChange'])$(id).disabled=!state.Changes;
 $('resultHint').textContent=state.CanSave?'Все конфликты решены. Можно сохранить результат.':'Раскройте спорную ветвь, перенесите нужные узлы стрелками. «Принять результат узла» подтверждает собранную ветвь.';
 const selected=reviewMap.get(activeNode);$('selectedTreeNode').textContent=selected?.Name||'Выберите узел';
 $('openNode').disabled=!selected;$('acceptNode').disabled=!selected;$('resetTreeNode').disabled=!selected?.HasDecision;
}
async function loadTree(){
 reviewRows=await api('navigation');reviewMap=new Map(reviewRows.map(r=>[r.ID,r]));renderComparison();drawTree();
 if(!activeNode&&state.Conflicts?.length)await focusNode(state.Conflicts[conflictCursor].TargetID||'root',true,conflictCursor);
}
function conflictKey(c){return JSON.stringify([c.TargetID||'root',c.Path,c.Reason])}
function currentConflict(){
 const list=state.Conflicts||[];
 if(!list.length){conflictCursor=0;conflictSelection='';return null}
 const found=list.findIndex(c=>conflictKey(c)===conflictSelection);
 conflictCursor=found>=0?found:Math.min(conflictCursor,list.length-1);
 const c=list[conflictCursor];conflictSelection=conflictKey(c);return c;
}
function selectConflict(index){const c=state.Conflicts?.[index];if(c){conflictCursor=index;conflictSelection=conflictKey(c)}}
function conflictTargets(c){
 const related=new Set((c.RelatedIDs||[]).filter(id=>reviewMap.has(id)));
 return related.size?[...reviewMap.keys()].filter(id=>related.has(id)):[c.TargetID||'root'];
}
function conflictBranches(){
 const ids=new Set();
 for(const c of state.Conflicts||[])for(const id of conflictTargets(c))
  for(let row=reviewMap.get(id);row&&!ids.has(row.ID);row=reviewMap.get(row.Parent))ids.add(row.ID);
 return ids;
}
function conflictLabel(c){return reviewMap.get(c.TargetID)?.Name||c.Path||'Документ'}
function renderConflictSummary(){
 const c=currentConflict();$('conflicts').hidden=!c;
 if(!c){if($('conflictListDialog').open)$('conflictListDialog').close();return}
 const targets=conflictTargets(c),part=targets.indexOf(activeNode);
 $('conflictPosition').textContent=`${conflictCursor+1} из ${state.Conflicts.length}`+(targets.length>1&&part>=0?` · участок ${part+1} из ${targets.length}`:'');
 $('currentConflictName').textContent=conflictLabel(c);$('currentConflictReason').textContent=c.Reason;
 $('currentConflict').title=conflictLabel(c)+' · '+c.Reason;
 if($('conflictListDialog').open)renderConflictList();
}
function renderConflictList(){
 $('conflictListTitle').textContent=`Конфликты: ${state.Conflicts.length}`;
 $('conflictList').replaceChildren(...state.Conflicts.map((c,i)=>{
  const b=node('button',undefined,'conflictListItem'+(i===conflictCursor?' selected':''));b.type='button';
  if(i===conflictCursor)b.setAttribute('aria-current','true');
  b.append(node('strong',`${i+1}. ${conflictLabel(c)}`),node('small',c.Path||''),node('span',c.Reason));
  b.addEventListener('click',()=>action(async()=>{$('conflictListDialog').close();await focusNode(c.TargetID||'root',true,i)}));return b;
 }));
}
$('currentConflict').addEventListener('click',()=>action(()=>{const c=currentConflict();if(c)return focusNode(c.TargetID||'root',true,conflictCursor)}));
$('showConflictList').addEventListener('click',()=>{renderConflictList();$('conflictListDialog').showModal();$('conflictList').querySelector('[aria-current]')?.scrollIntoView({block:'nearest'})});
$('closeConflictList').addEventListener('click',()=>$('conflictListDialog').close());
function arrow(label,direction,handler){
 const b=node('button',undefined,'takeArrow');b.type='button';b.title=label;b.setAttribute('aria-label',label);
 const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');svg.setAttribute('viewBox','0 0 16 16');svg.setAttribute('aria-hidden','true');svg.setAttribute('focusable','false');
 const path=document.createElementNS('http://www.w3.org/2000/svg','path');path.setAttribute('d',direction==='→'?'M3 8h10M8 3l5 5-5 5':'M13 8H3M8 3L3 8l5 5');svg.append(path);b.append(svg);
 b.addEventListener('click',e=>{e.stopPropagation();action(()=>handler(e))});return b
}
function originMarkers(sides){
 const wrap=node('span',undefined,'originMarkers');
 sides.forEach((present,i)=>{if(!present)return;const icon=document.createElementNS('http://www.w3.org/2000/svg','svg');icon.setAttribute('viewBox','0 0 10 10');icon.setAttribute('role','img');icon.setAttribute('aria-label',i===0?'Из LOCAL (слева)':'Из REMOTE (справа)');
 const title=document.createElementNS('http://www.w3.org/2000/svg','title');title.textContent=i===0?'Из LOCAL — слева':'Из REMOTE — справа';
 const path=document.createElementNS('http://www.w3.org/2000/svg','path');path.setAttribute('d',i===0?'M9 5L2 1v8Z':'M1 5L8 1v8Z');icon.append(title,path);wrap.append(icon)});
 return wrap;
}
function drawTree(){
 $('tree').replaceChildren();const conflicts=conflictBranches();
 function add(id,depth,parent){const row=reviewMap.get(id);if(!row)return;
  const isConflict=conflicts.has(id),open=row.HasChildren&&expanded.has(id);
  const wrapper=node('div',undefined,'treeEntry'+(open?' expanded':''));wrapper.dataset.id=id;
  const line=node('div',undefined,'treeRow'+(isConflict?' hasConflict':'')+(row.Resolved?' resolved':'')+(id===activeNode?' current':''));line.tabIndex=0;line.setAttribute('aria-label',row.Name+(isConflict?' — конфликт':''));
  if(row.HasChildren)line.setAttribute('aria-expanded',String(!!open));
  const label=node('div',undefined,'treeLabel');label.title=row.Name;label.style.paddingLeft=(8+Math.min(depth,12)*14)+'px';
  const toggle=node('button',row.HasChildren?(expanded.has(id)?'▾':'▸'):'·');toggle.disabled=!row.HasChildren;toggle.title=expanded.has(id)?'Свернуть ветвь':'Раскрыть ветвь';toggle.addEventListener('click',e=>{e.stopPropagation();expanded.has(id)?expanded.delete(id):expanded.add(id);drawTree()});
  label.append(toggle,node('span',row.Name));if(isConflict)label.append(node('span','!','nodeBadge conflictBadge'));else if(row.Resolved)label.append(node('span','✓','nodeBadge'));else if(row.Changed)label.append(node('span','●','nodeBadge'));line.append(label);
  const signature=v=>v?JSON.stringify([Object.entries(v.Attributes).sort(),v.Text]):'absent';
  [row.Values[1],row.Result,row.Values[2]].forEach((v,i)=>{
   const slot=node('div',undefined,'valueSlot '+(i===1?'centerValue ':'')),cell=node('div',undefined,'valueCell '+(!v?'absent ':'')+(signature(v)!==signature(row.Values[0])?'different':''));
   const text=v?[...Object.entries(v.Attributes).map(([k,val])=>'@'+k+' = '+val),...(v.Text?.trim()?[v.Text]:[])].join('\n'):'—';cell.append(node('div',text||' ','cellPreview'));slot.append(cell);
   if(i===1&&row.Origin?.some(Boolean)){slot.classList.add("hasOrigin");slot.append(originMarkers(row.Origin))}
   if(i!==1&&row.Take?.[i===0?0:1])slot.append(arrow(`Взять ${i===0?'LOCAL':'REMOTE'}: ${row.Name}`,i===0?'→':'←',()=>takeTreeNode(id,i===0?'local':'remote')));
   line.append(slot);
  });
  line.addEventListener('click',()=>action(()=>focusNode(id)));line.addEventListener('dblclick',()=>action(()=>openTextNode(id)));line.addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();action(()=>openTextNode(id))}});wrapper.append(line);parent.append(wrapper);
  if(open)for(const child of row.Children||[])add(child,depth+1,wrapper);
 }add('root',0,$('tree'));requestAnimationFrame(()=>{syncScrollbarSizes();syncTreeSticky()});
}
function syncTreeSticky(){
 function visit(parent,top,depth){for(const entry of parent.children){
  if(!entry.classList.contains('treeEntry'))continue;
  const line=entry.firstElementChild;line.style.setProperty('--sticky-top',top+'px');line.style.setProperty('--sticky-z',Math.max(1,100-depth));
  visit(entry,top+line.offsetHeight,depth+1);
 }}visit($('tree'),0,0);
}
function leaveText(){if(draftDirty&&!confirm('Не применённый текст будет потерян. Перейти к дереву?'))return false;draftDirty=false;$('nodeEditor').hidden=true;$('compareView').classList.remove('editing-text');return true}
async function focusNode(id,conflict=false,conflictIndex=null){
 if(!reviewMap.has(id)||!leaveText())return;
 if(conflict){
  const c=state.Conflicts?.[conflictIndex??conflictCursor];
  if(c){const targets=conflictTargets(c);if(!targets.includes(id))id=targets[0];
   for(const target of targets)for(let r=reviewMap.get(target);r?.Parent;r=reviewMap.get(r.Parent))expanded.add(r.Parent);
  }
 }
 for(let r=reviewMap.get(id);r?.Parent;r=reviewMap.get(r.Parent))expanded.add(r.Parent);
 activeNode=id;const row=reviewMap.get(id);
 if(conflictIndex!==null)selectConflict(conflictIndex);
 else for(let r=row;r;r=reviewMap.get(r.Parent)){
  const index=(state.Conflicts||[]).findIndex(c=>(c.TargetID||'root')===r.ID);
  if(index>=0){if((state.Conflicts[conflictCursor]?.TargetID||'root')!==r.ID)selectConflict(index);break}
 }
 if(conflict||row.Conflict)expanded.add(id);
 drawTree();renderComparison();
 requestAnimationFrame(()=>{const entry=document.querySelector(`.treeEntry[data-id="${CSS.escape(id)}"]`);if(!entry)return;
  syncTreeSticky();const viewport=$('treeViewport');let pinned=0;
  for(let parent=entry.parentElement;parent&&parent!==$('tree');parent=parent.parentElement)pinned+=parent.firstElementChild.offsetHeight;
  const top=entry.getBoundingClientRect().top-viewport.getBoundingClientRect().top+viewport.scrollTop;
  viewport.scrollTop=Math.max(0,top-pinned-8);
 });
}
async function takeTreeNode(id,side){
 await preparing('Применяем изменение узла…',async()=>{
  state=await api('resolve',{ID:id,Choice:'take-'+side});activeNode=id;await loadTree();
 });
}
async function acceptTreeNode(){await preparing('Принимаем результат узла…',async()=>{const detail=await api('node?id='+encodeURIComponent(activeNode));state=await api('resolve',{ID:activeNode,Choice:'xml',XML:detail.ResultXML});await loadTree()})}
async function openTextNode(id){
 if(draftDirty&&!confirm('Не применённый текст будет потерян. Открыть другой узел?'))return;
 nodeDetail=await api('node?id='+encodeURIComponent(id));activeNode=id;draftDirty=false;
 $('reviewNodeName').textContent=id==='root'?'Весь документ':nodeDetail.Name;
 $('nodeEditor').hidden=false;$('compareView').classList.add('editing-text');$('nodeError').hidden=true;$('resetNode').disabled=!nodeDetail.Decision;
 initTextReview(nodeDetail);renderComparison();
}
function navigateReview(kind,direction){
 if(kind==='conflict'){
  const c=currentConflict();if(!c)return;
  const stops=state.Conflicts.flatMap((c,index)=>conflictTargets(c).map(id=>({id,index})));
  let at=stops.findIndex(s=>s.index===conflictCursor&&s.id===activeNode);
  if(at<0)at=stops.findIndex(s=>s.index===conflictCursor);
  const next=stops[(at+direction+stops.length)%stops.length];return focusNode(next.id,true,next.index);
 }
 const ids=reviewRows.filter(r=>r.Changed).map(r=>r.ID);if(!ids.length)return;
 let index=ids.indexOf(activeNode);if(index<0)index=direction>0?-1:0;
 return focusNode(ids[(index+direction+ids.length)%ids.length]);
}
for(const [id,kind,direction] of [['prevChange','change',-1],['nextChange','change',1],['prevConflict','conflict',-1],['nextConflict','conflict',1]])$(id).addEventListener('click',()=>action(()=>navigateReview(kind,direction)));
$('openNode').addEventListener('click',()=>action(()=>openTextNode(activeNode)));
$('acceptNode').addEventListener('click',()=>action(acceptTreeNode));
$('resetTreeNode').addEventListener('click',()=>action(()=>preparing('Отменяем решение узла…',async()=>{state=await api('resolve',{ID:activeNode,Choice:'reset'});await loadTree()})));
$('wholeDocument').addEventListener('click',()=>action(()=>openTextNode('root')));
$('closeNodeEditor').addEventListener('click',()=>{if(leaveText()){drawTree();renderComparison()}});
async function applyNodeDecision(reset){try{await preparing(reset?'Отменяем решение узла…':'Применяем текст узла…',async()=>{const id=activeNode;state=await api('resolve',{ID:id,Choice:reset?'reset':'xml',XML:reset?'':textResult()});draftDirty=false;await loadTree();await openTextNode(id)})}catch(e){$('nodeError').textContent=e.message;$('nodeError').hidden=false}}
$('applyNode').addEventListener('click',()=>action(()=>applyNodeDecision(false)));$('resetNode').addEventListener('click',()=>action(()=>applyNodeDecision(true)));
function dirtyResult(){draftDirty=true;renderComparison()}
function refreshUndo(){
 $('undoAction').disabled=$('nodeEditor').hidden?!state.CanUndo:!(textUndo.length||(!draftDirty&&state.CanUndo));
}
async function undoAction(){
 const editing=!$('nodeEditor').hidden;
 if(editing&&undoTextAction())return;
 if(draftDirty||!state.CanUndo)return;
 const id=activeNode,offset={...diffOffset},treeTop=$('treeViewport').scrollTop;
 await preparing('Отменяем последнее действие…',async()=>{
  state=await api('undo',{});await loadTree();
  if(editing){await openTextNode(id);syncDiffScroll(offset.left,offset.top)}
  else $('treeViewport').scrollTop=treeTop;
 });
 if(editing)diffPanes[1].querySelector('textarea')?.focus({preventScroll:true});
 else document.querySelector(`.treeEntry[data-id="${CSS.escape(id)}"] > .treeRow`)?.focus({preventScroll:true});
}
$('undoAction').addEventListener('click',()=>action(undoAction));
window.addEventListener('keydown',e=>{
 if(!(e.ctrlKey||e.metaKey)||e.shiftKey||e.altKey||e.isComposing||!(e.code==='KeyZ'||e.key.toLowerCase()==='z')||$('compareView').hidden)return;
 if(e.target.closest?.('dialog,input,[contenteditable="true"]'))return;
 e.preventDefault();action(undoAction);
});
function syncHorizontal(offset){horizontalOffset=offset;document.querySelectorAll('.valueCell,.paneScrollbar').forEach(el=>{if(el.scrollLeft!==offset)el.scrollLeft=offset})}
function syncScrollbarSizes(){
 const cells=[...document.querySelectorAll('.valueCell')];cells.forEach(c=>{c.firstElementChild.style.minWidth='0px'});
 const width=Math.max(0,...cells.map(c=>c.firstElementChild?.scrollWidth||0)),distance=Math.max(0,...cells.map(c=>width+35-c.clientWidth));
 cells.forEach(c=>{c.firstElementChild.style.minWidth=(distance+c.clientWidth-35)+'px'});document.querySelectorAll('.paneScrollbar').forEach(el=>{el.firstElementChild.style.width=(el.clientWidth+distance)+'px'});syncHorizontal(Math.min(horizontalOffset,distance));
}
document.querySelectorAll('.paneScrollbar').forEach(el=>el.addEventListener('scroll',()=>{if(el.scrollLeft!==horizontalOffset)syncHorizontal(el.scrollLeft)}));
window.addEventListener('resize',()=>requestAnimationFrame(()=>{syncScrollbarSizes();syncTreeSticky();sizeDiffPanes()}));
