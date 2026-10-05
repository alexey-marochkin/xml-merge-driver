'use strict';
const $=id=>document.getElementById(id);
const token=location.hash.slice(1)||sessionStorage.getItem('xmlmerge-token');sessionStorage.setItem('xmlmerge-token',token);history.replaceState(null,'',location.pathname);
let state,selected,mode='attribute',checked=new Set(),valid=false,busy=false,sampleRequest=0,previewRequest=0,pendingChoice;
const node=(tag,text,cls)=>{const n=document.createElement(tag);if(text!==undefined)n.textContent=text;if(cls)n.className=cls;return n};
function error(message){$('error').textContent=message;$('error').hidden=!message}
async function api(path,body){const res=await fetch('/api/'+path,{method:body===undefined?'GET':'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});const data=await res.json();if(!res.ok)throw Error(data.Error||'Ошибка запроса');return data}
async function action(fn){if(busy)return;busy=true;error('');try{await fn()}catch(e){error(e.message)}finally{busy=false}}
async function preparing(message,fn){
 const panel=$('preparation'),wasHidden=panel.hidden,previous=$('preparationMessage').textContent;
 const title=panel.querySelector('h2'),hint=panel.querySelector('small'),previousTitle=title.textContent,previousHint=hint.textContent;
 const updating=state?.Phase==='compared';
 title.textContent=updating?'Обновление результата':'Подготовка сравнения';
 hint.textContent=updating?'Дождитесь завершения операции':'Сравнение откроется автоматически';
 const regions=[$('rulesView'),$('compareView'),document.querySelector('.context'),document.querySelector('footer')];
 const inert=regions.map(n=>n.inert);
 panel.hidden=false;$('preparationMessage').textContent=message;regions.forEach(n=>n.inert=true);document.body.setAttribute('aria-busy','true');
 try{
  // Paint the indicator before starting work that may also occupy the UI thread.
  await new Promise(resolve=>requestAnimationFrame(()=>setTimeout(resolve,0)));
  return await fn();
 }finally{
  panel.hidden=wasHidden;$('preparationMessage').textContent=previous;
  title.textContent=previousTitle;hint.textContent=previousHint;
  regions.forEach((n,i)=>n.inert=inert[i]);document.body.setAttribute('aria-busy',String(!wasHidden));
 }
}
function fieldKey(f){return JSON.stringify([f.Namespace||'',f.Name])}
function basename(path){return path.split(/[\\/]/).pop()}
function render(){
 if(state.Phase!=='compared'&&typeof resetReview==='function')resetReview();
 $('root').textContent=state.Root.replace(/^\{\}/,'');$('rulesFile').textContent=state.Options.Rules;
 $('localFile').textContent=basename(state.Options.Local);$('localFile').title=state.Options.Local;
 $('resultFile').textContent=basename(state.Options.Base);$('resultFile').title='Основа: '+state.Options.Base+'; результат: '+state.Options.Output;
 $('remoteFile').textContent=basename(state.Options.Remote);$('remoteFile').title=state.Options.Remote;
 $('comparisonControls').hidden=state.Phase!=='compared';document.body.classList.toggle('comparing',state.Phase==='compared');
 $('readyBadge').textContent=state.Report.Ready?'Правила готовы':'Требуется настройка';$('readyBadge').className='badge'+(state.Report.Ready?'':' amber');
 $('count').textContent=state.Report.Missing;$('status').textContent=state.Report.Ready?'Все группы узлов подготовлены к сравнению':`До сравнения нужно настроить групп: ${state.Report.Missing}`;
 $('compare').disabled=!state.Report.Ready||state.Options.Mode==='rules';$('compare').hidden=state.Phase==='compared';
 $('rulesView').hidden=state.Phase==='compared';$('compareView').hidden=state.Phase!=='compared';
 $('stepRules').classList.toggle('active',state.Phase!=='compared');$('stepCompare').classList.toggle('active',state.Phase==='compared');
 renderGroups();
}
function visibleGroups(){return (state.Report.Groups||[]).filter(g=>(!$('missingOnly').checked||g.Status==='missing')&&(g.Path+' '+g.Name).toLowerCase().includes($('search').value.toLowerCase()))}
function renderGroups(){const list=visibleGroups();$('groups').replaceChildren();for(const g of list){const b=node('button',undefined,'group'+(g.Status==='missing'?' missing':'')+(selected?.Path===g.Path?' selected':''));b.append(node('span','','dot'));const label=node('span');label.append(node('strong',g.Name),node('small',g.Status==='missing'?'Нужно правило':g.Status==='structural'?'По структуре':g.InheritedFrom?'Унаследовано':'Правило задано'));b.append(label);b.addEventListener('click',()=>select(g));$('groups').append(b)}if(!selected||!list.some(g=>g.Path===selected.Path)){selected=null;if(list.length)select(list[0]);else{$('editor').hidden=true;$('empty').hidden=false;$('empty').querySelector('h2').textContent=state.Report.Ready?'Правил достаточно':'Нет узлов по этому фильтру'}}}
function select(g){selected=g;valid=false;previewRequest++;checked=new Set((g.Rule?.Fields||[]).map(fieldKey));mode=g.Rule?.Mode==='auto'?'attribute':g.Rule?.Mode||'attribute';$('editor').hidden=false;$('empty').hidden=true;$('nodeName').textContent=g.Name;$('nodePath').textContent=g.Path;$('nodeBadge').textContent=g.Status==='missing'?'Нужна настройка':g.Status==='structural'?'Однозначная структура':'Правило готово';$('nodeBadge').className='badge'+(g.Status==='missing'?' amber':'');$('explanation').textContent=(g.Message||'Правило действует для этого имени элемента на любой глубине.')+(g.InheritedFrom?' Настройка унаследована от '+g.InheritedFrom+'. Сохранение создаст собственное правило этого узла.':'');$('order').value=g.Rule?.ConfiguredOrder||g.Rule?.Order||'significant';$('previewTable').replaceChildren();$('previewHint').textContent='Проверка выполняется для всех элементов этого имени на любой глубине, включая не показанные в примерах.';$('saveRule').disabled=true;renderModes();renderGroups();loadSamples(g.Path)}
function renderModes(){document.querySelector('.modes').hidden=selected.IsRoot;$('identityTitle').textContent=selected.IsRoot?'Порядок элементов корня':'Как определить один и тот же узел?';document.querySelectorAll('[data-mode]').forEach(b=>b.classList.toggle('active',b.dataset.mode===mode));$('fields').replaceChildren();if(selected.IsRoot){$('fieldHelp').textContent='Корень уже определен именем и пространством имен. Здесь можно задать значимость порядка его дочерних элементов.';return}const fields=mode==='attribute'?selected.Attributes:selected.Elements;$('fieldHelp').textContent=mode==='name'?'Ключ — имя элемента и пространство имён. Подходит для одиночного свойства.':mode==='text'?'Используется собственное текстовое значение без текста потомков.':mode==='attribute'?'Выберите один или несколько атрибутов. Их значения образуют составной ключ.':'Выберите дочерние элементы с текстовыми значениями. Их значения образуют составной ключ.';if(!['text','name'].includes(mode)){for(const f of fields||[]){const l=node('label',undefined,'field'),cb=node('input');cb.type='checkbox';cb.checked=checked.has(fieldKey(f));cb.addEventListener('change',()=>{cb.checked?checked.add(fieldKey(f)):checked.delete(fieldKey(f));invalidate()});l.append(cb,node('span',f.Name));if(f.Namespace)l.append(node('small',f.Namespace));$('fields').append(l)}if(!fields?.length)$('fields').append(node('span','Подходящих полей в примерах нет.','muted'))} }
function invalidate(){valid=false;previewRequest++;$('saveRule').disabled=true;$('previewHint').textContent='Правило изменено. Проверьте ключи перед сохранением.';$('previewTable').replaceChildren()}
function draft(){if(selected.IsRoot)return{Path:selected.Path,Selector:selected.Selector,TrimSpace:!!selected.Rule?.TrimSpace,Mode:selected.Rule?.Mode==='auto'?'text':selected.Rule?.Mode||'text',Order:$('order').value,Origin:'manual',Fields:selected.Rule?.Fields||[]};const fields=mode==='attribute'?selected.Attributes:selected.Elements;return{Path:selected.Path,Selector:selected.Selector,TrimSpace:!!selected.Rule?.TrimSpace,Mode:mode,AllowDeleteAdd:!!selected.Rule?.AllowDeleteAdd,Order:$('order').value,Origin:'manual',Fields:['text','name'].includes(mode)?[]:(fields||[]).filter(f=>checked.has(fieldKey(f))).map(f=>({...f,TrimSpace:!!selected.Rule?.Fields?.find(old=>fieldKey(old)===fieldKey(f))?.TrimSpace}))}}
async function loadSamples(path){const req=++sampleRequest;$('samples').replaceChildren(node('span','Загрузка примеров…','muted'));try{const samples=await api('samples?path='+encodeURIComponent(path));if(req!==sampleRequest)return;$('samples').replaceChildren();for(const side of ['base','local','remote']){const col=node('div',undefined,'sampleColumn');col.append(node('h4',side.toUpperCase()));for(const row of (samples||[]).filter(s=>s.Side===side).slice(0,4)){const attrs=Object.entries(row.Attributes||{}).map(([k,v])=>'@'+k.replace(/^\{\}/,'')+' = '+JSON.stringify(v));const elems=Object.entries(row.Elements||{}).map(([k,v])=>k.replace(/^\{\}/,'')+' = '+JSON.stringify(v));col.append(node('div',[...attrs,...elems,...(row.Text?['text = '+JSON.stringify(row.Text)]:[])].join('\n')||'(пустой узел)','sampleItem'))}$('samples').append(col)}}catch(e){error(e.message)}}
document.querySelectorAll('[data-mode]').forEach(b=>b.addEventListener('click',()=>{mode=b.dataset.mode;checked.clear();invalidate();renderModes()}));
$('order').addEventListener('change',invalidate);$('search').addEventListener('input',renderGroups);$('missingOnly').addEventListener('change',renderGroups);
$('preview').addEventListener('click',()=>action(async()=>{const req=++previewRequest;const p=await api('preview',draft());if(req!==previewRequest)return;valid=p.Valid;$('saveRule').disabled=!valid;$('previewHint').textContent=valid?`Ключи подходят. Проверено узлов: ${p.Total}.`:p.Error;$('previewHint').className=valid?'good':'muted';const table=node('table'),head=node('tr');['Версия / группа','Узел','Ключ','Проверка'].forEach(t=>head.append(node('th',t)));table.append(head);for(const row of p.Rows){const tr=node('tr',undefined,row.Error||row.Duplicate?'bad':'');tr.append(node('td',row.Side+' / '+row.Group),node('td',String(row.Index)),node('td',row.Key||row.Error,'key'),node('td',row.Error?'Нет значения':row.Duplicate?'Повторяется':'Уникален'));table.append(tr)}$('previewTable').replaceChildren(table)}));
$('saveRule').addEventListener('click',()=>action(async()=>{if(!valid)return;const path=selected.Path;state=await api('rule',{Rule:draft(),Revision:state.Revision});selected=null;render();if(state.Report.Ready)await compareWhenReady();else $('status').textContent='Правило сохранено. Осталось настроить групп: '+state.Report.Missing}));
$('reload').addEventListener('click',()=>action(async()=>{state=await api('reload',{});selected=null;render();await compareWhenReady()}));
$('compare').addEventListener('click',()=>action(compareWhenReady));
$('back').addEventListener('click',()=>action(async()=>{if((state.Decisions||draftDirty)&&!confirm('Возврат к правилам сбросит решения узлов и неприменённый текст. Продолжить?'))return;state=await api('reload',{});selected=null;render()}));
$('saveResult').addEventListener('click',()=>action(async()=>{if(draftDirty)throw Error('Сначала примените или отмените изменения в среднем окне.');await api('save',{Choice:'merged'});$('status').textContent='Результат сохранен. Окно закрывается.'}));
$('close').addEventListener('click',()=>action(()=>api('cancel',{})));
async function compareWhenReady(){
 if(!state.Report?.Ready||state.Options.Mode==='rules')return;
 await preparing('Сопоставляем узлы и объединяем изменения…',async()=>{
  if(state.Phase!=='compared')state=await api('compare',{});
  $('preparationMessage').textContent='Готовим дерево сравнения…';render();await loadTree();
 });
}
action(()=>preparing('Читаем документы и проверяем правила…',async()=>{state=await api('state');render();await compareWhenReady()}));
