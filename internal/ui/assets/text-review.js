 'use strict';
let textChunks=[],textActive=-1,textBase='',textLocal='',textRemote='',textConflictLines=[[],[],[]];
const diffPanes=[...document.querySelectorAll('.diffPane')],expectedTextScroll=new WeakMap();
let diffOffset={left:0,top:0},textLayoutChanging=false;
let textTransferMode='replace',textTransferPreview=null;
let textUndo=[];
function rememberTextAction(selection=null){
 textUndo.push({chunks:structuredClone(textChunks),active:textActive,offset:{...diffOffset},dirty:draftDirty,selection});
 if(textUndo.length>100)textUndo.shift();
}
function undoTextAction(){
 const previous=textUndo.pop();if(!previous)return false;
 textChunks=previous.chunks;textActive=previous.active;diffOffset=previous.offset;draftDirty=previous.dirty;
 renderTextDiff();renderComparison();$('nodeError').hidden=true;
 const selection=previous.selection,block=diffPanes[1].children[selection?.index??Math.max(0,textActive)],area=block?.querySelector('textarea');
 area?.focus({preventScroll:true});if(selection)area?.setSelectionRange(selection.start,selection.end);
 return true;
}
function transferMode(event){return event.shiftKey?'before':event.ctrlKey?'after':'replace'}
function updateTransferMode(event){
 const mode=transferMode(event);if(mode===textTransferMode)return;textTransferMode=mode;
 refreshTransferButtons();if(textTransferPreview)previewTextTransfer(...textTransferPreview);
}
function refreshTransferButtons(){
 for(const p of diffPanes)p.querySelectorAll('.takeArrow').forEach(b=>{
  const insert=textTransferMode!=='replace',left=b.dataset.source==='1';
  b.hidden=insert?b.dataset.empty==='true':b.dataset.same==='true';
  const verb=textTransferMode==='before'?'Вставить выше':textTransferMode==='after'?'Вставить ниже':'Заменить';
  b.title=`${verb}: ${b.dataset.range}`;b.setAttribute('aria-label',b.title);
  const path=textTransferMode==='before'?(left?'M3 13L13 3M6 3h7v7':'M13 13L3 3h7M3 3v7'):
   textTransferMode==='after'?(left?'M3 3l10 10M6 13h7V6':'M13 3L3 13h7M3 13V6'):
   left?'M3 8h10M8 3l5 5-5 5':'M13 8H3M8 3L3 8l5 5';
  b.querySelector('path').setAttribute('d',path);
 });
}
window.addEventListener('keydown',updateTransferMode);window.addEventListener('keyup',updateTransferMode);
window.addEventListener('blur',()=>{clearTransferPreview();updateTransferMode({})});
document.addEventListener('visibilitychange',()=>{if(document.hidden){clearTransferPreview();updateTransferMode({})}});
function textResult(){return LineDiff.result(textChunks)}
function initTextReview(detail){textUndo=[];textConflictLines=detail.ConflictLines||[[],[],[]];textBase=detail.XML[0];textLocal=detail.XML[1];textRemote=detail.XML[2];textActive=-1;textChunks=LineDiff.align(textBase,textLocal,detail.ResultXML,textRemote);diffOffset={left:0,top:0};renderTextDiff()}
function refreshTextCounts(){
 const changes=textChunks.filter(c=>c.changed).length,conflicts=textChunks.filter(c=>c.conflict||c.structuralConflict).length;
 $('textCounts').textContent=`Изменений: ${changes} · Конфликтных блоков: ${conflicts}`;
 for(const id of ['prevTextChange','nextTextChange'])$(id).disabled=!changes;
 for(const id of ['prevTextConflict','nextTextConflict'])$(id).disabled=!conflicts;
}
function structuralTextConflict(chunk){return chunk.changed&&[0,1,3].some((side,i)=>(textConflictLines[i]||[]).some(([start,end])=>chunk.starts[side]<end&&chunk.starts[side]+Math.max(1,chunk.values[side].length)>start))}
function textOriginMarkers(chunk){
 const column=node('span',undefined,'originLines'),sides=LineDiff.origin(chunk);
 if(sides.some(Boolean))for(let i=0;i<Math.max(1,...chunk.values.slice(1).map(v=>v.length));i++){
  const markers=originMarkers(sides);markers.style.top=(i*18+4)+'px';column.append(markers);
 }
 return column;
}
function renderTextDiff(anchor=null){
 const position={...diffOffset};textLayoutChanging=true;
 try{
 textTransferPreview=null;
 textChunks.forEach(c=>c.structuralConflict=structuralTextConflict(c));
 const fragments=diffPanes.map(()=>document.createDocumentFragment());
 textChunks.forEach((chunk,index)=>{
  for(let side=1;side<=3;side++){
   const block=node('div',undefined,'diffBlock'+(chunk.changed?' changed':'')+(chunk.conflict?' conflictLines':'')+(chunk.structuralConflict?' structuralConflict':'')+(textActive===index?' activeHunk':''));block.dataset.hunk=index;block.dataset.side=side;block.tabIndex=-1;
   const gutter=node('div',undefined,'lineGutter');
   const numbers=node('pre',chunk.values[side].map((_,i)=>chunk.starts[side]+i+1).join('\n'),'lineNumbers');gutter.append(numbers);block.append(gutter);
   const area=node('textarea',undefined,'diffText');area.value=chunk.values[side].join('\n');area.readOnly=side!==2;area.spellcheck=false;area.wrap='off';area.setAttribute('aria-label',`${['','LOCAL','Результат','REMOTE'][side]} · строки ${chunk.starts[side]+1}–${chunk.starts[side]+Math.max(1,chunk.values[side].length)}`);
   if(!chunk.values[side].length)block.classList.add('gap');
   let selection=null;
   if(side===2)area.addEventListener('beforeinput',()=>{selection={index,start:area.selectionStart,end:area.selectionEnd}});
   if(side===2)area.addEventListener('input',()=>{
    rememberTextAction(selection);selection=null;
    chunk.values[2]=LineDiff.lines(area.value);chunk.changed=true;chunk.conflict=chunk.values[1].join('\n')!==area.value&&chunk.values[3].join('\n')!==area.value&&chunk.conflict;
    textActive=index;dirtyResult();let offset=0;textChunks.forEach((c,i)=>{c.starts[2]=offset;offset+=c.values[2].length;updateHunk(i)});refreshTransferButtons();sizeDiffPanes();refreshTextCounts();
   });
   area.addEventListener('focus',()=>{textActive=index;markTextHunk()});block.append(area);
   if(side===2)gutter.append(textOriginMarkers(chunk));
   if(side!==2&&(chunk.changed||JSON.stringify(chunk.values[side])!==JSON.stringify(chunk.values[2]))){
    const start=chunk.starts[side]+1,end=chunk.starts[side]+chunk.values[side].length;
    const range=chunk.values[side].length?`строки ${start}–${end}`:'удаление блока';
    const button=arrow('',side===1?'→':'←',e=>takeTextHunk(index,side,transferMode(e)));
    button.addEventListener('pointerdown',e=>{if(e.button===0){e.preventDefault();button.focus({preventScroll:true})}});
    button.dataset.source=side;button.dataset.range=`${side===1?'LOCAL':'REMOTE'}, блок ${index+1}, ${range}`;
    button.dataset.empty=String(!chunk.values[side].length);button.dataset.same=String(JSON.stringify(chunk.values[side])===JSON.stringify(chunk.values[2]));
    button.addEventListener('mouseenter',e=>{updateTransferMode(e);previewTextTransfer(index,side)});button.addEventListener('mouseleave',clearTransferPreview);
    button.addEventListener('focus',()=>previewTextTransfer(index,side));button.addEventListener('blur',clearTransferPreview);(side===3?gutter:block).append(button);
   }
   fragments[side-1].append(block);
  }
 });
 diffPanes.forEach((p,i)=>p.replaceChildren(fragments[i]));
 refreshTransferButtons();refreshTextCounts();sizeDiffPanes(anchor,position);
 if(anchor){
  const block=diffPanes[anchor.side-1].children[textActive];
  (block?.querySelector('.takeArrow:not([hidden])')||block)?.focus({preventScroll:true});
 }
 }finally{textLayoutChanging=false}
}
function updateHunk(index){
 const c=textChunks[index];document.querySelectorAll(`.diffBlock[data-hunk="${index}"]`).forEach(block=>{
  const side=Number(block.dataset.side);block.classList.toggle('gap',!c.values[side].length);block.classList.toggle('changed',c.changed);block.classList.toggle('conflictLines',c.conflict);
  const button=block.querySelector('.takeArrow');if(button)button.dataset.same=String(JSON.stringify(c.values[side])===JSON.stringify(c.values[2]));
  if(side===2){block.querySelector('.lineNumbers').textContent=c.values[2].map((_,i)=>c.starts[2]+i+1).join('\n');block.querySelector('.originLines')?.replaceWith(textOriginMarkers(c))}
 });
}
function clearTransferPreview(){textTransferPreview=null;document.querySelectorAll('.transferSource,.transferTarget').forEach(b=>b.classList.remove('transferSource','transferTarget','insertBefore','insertAfter'))}
function previewTextTransfer(index,side){
 clearTransferPreview();if(textTransferMode!=='replace'&&!textChunks[index].values[side].length)return;textTransferPreview=[index,side];
 diffPanes[side-1].children[index]?.classList.add('transferSource');diffPanes[1].children[index]?.classList.add('transferTarget');
 if(textTransferMode!=='replace')diffPanes[1].children[index]?.classList.add(textTransferMode==='before'?'insertBefore':'insertAfter');
}
function takeTextHunk(index,side,mode='replace'){
 const current=LineDiff.transfer(textChunks,index,side,mode);if(current===textResult())return;rememberTextAction();dirtyResult();
 const chunk=textChunks[index],line=Math.max(0,chunk.starts[side]-(chunk.values[side].length?0:1));
 const located=LineDiff.locateLine(textChunks,side,line),pane=diffPanes[side-1];
 const anchor={side,line,viewportY:pane.children[located.index].offsetTop+located.row*18-pane.scrollTop};
 textChunks=LineDiff.align(textBase,textLocal,current,textRemote);textActive=LineDiff.locateLine(textChunks,side,line).index;renderTextDiff(anchor);
}
function markTextHunk(){document.querySelectorAll('.diffBlock').forEach(b=>b.classList.toggle('activeHunk',Number(b.dataset.hunk)===textActive))}
function navigateText(kind,direction){
 const ids=textChunks.flatMap((c,i)=>(kind==='conflict'?(c.conflict||c.structuralConflict):c.changed)?[i]:[]);if(!ids.length)return;
 const candidates=direction>0?ids.filter(i=>i>textActive):ids.filter(i=>i<textActive).reverse();textActive=candidates[0]??(direction>0?ids[0]:ids.at(-1));markTextHunk();
 const block=diffPanes[1].querySelector(`[data-hunk="${textActive}"]`);if(block)syncDiffScroll(diffOffset.left,Math.max(0,block.offsetTop-24));
}
for(const [id,kind,dir]of [['prevTextChange','change',-1],['nextTextChange','change',1],['prevTextConflict','conflict',-1],['nextTextConflict','conflict',1]])$(id).addEventListener('click',()=>navigateText(kind,dir));
function syncDiffScroll(left,top){
 left=Math.max(0,Math.min(left,...diffPanes.map(p=>p.scrollWidth-p.clientWidth)));
 top=Math.max(0,Math.min(top,...diffPanes.map(p=>p.scrollHeight-p.clientHeight)));
 diffOffset={left,top};for(const p of diffPanes){p.scrollLeft=left;p.scrollTop=top;expectedTextScroll.set(p,{left:p.scrollLeft,top:p.scrollTop})}
}
for(const p of diffPanes)p.addEventListener('scroll',()=>{if(textLayoutChanging)return;const e=expectedTextScroll.get(p);if(e&&e.left===p.scrollLeft&&e.top===p.scrollTop)return;syncDiffScroll(p.scrollLeft,p.scrollTop)});
for(const p of diffPanes)p.addEventListener('pointermove',updateTransferMode);
function sizeDiffPanes(anchor=null,position={...diffOffset}){
 if($('nodeEditor').hidden)return;
 const measure=node('pre',undefined,'textMeasure');document.body.append(measure);let width=0;
 for(const c of textChunks)for(let s=1;s<=3;s++){measure.textContent=c.values[s].join('\n');width=Math.max(width,measure.scrollWidth)}measure.remove();
 const extra=Math.max(0,width+70-Math.min(...diffPanes.map(p=>p.clientWidth)));
 textChunks.forEach((c,index)=>{const height=Math.max(1,...c.values.slice(1).map(v=>v.length))*18;
  diffPanes.forEach(p=>{const block=p.children[index];block.style.height=height+'px';block.style.width=(p.clientWidth+extra)+'px';block.querySelector('textarea').style.height=height+'px'})
 });
 if(anchor){const located=LineDiff.locateLine(textChunks,anchor.side,anchor.line);position.top=diffPanes[anchor.side-1].children[located.index].offsetTop+located.row*18-anchor.viewportY}
 syncDiffScroll(position.left,position.top);
}
