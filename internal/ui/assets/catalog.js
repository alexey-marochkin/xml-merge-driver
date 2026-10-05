'use strict';
const $=id=>document.getElementById(id);
const token=location.hash.slice(1)||sessionStorage.getItem('xmlmerge-token');
sessionStorage.setItem('xmlmerge-token',token);history.replaceState(null,'',location.pathname);
let state,profileIndex=0,ruleIndex=-1,activeKnown=null,busy=false,dirty=false,staged=false,closing=false,pendingLeave=null,lastStatus='Загружено';
const node=(tag,text,cls)=>{const n=document.createElement(tag);if(text!==undefined)n.textContent=text;if(cls)n.className=cls;return n};
const profile=()=>state?.Database.Profiles?.[profileIndex];
const commonRule=()=>profile()?.Rules?.find(r=>r.Selector==='*')||{Selector:'*',Mode:'auto',Order:'significant',Fields:[]};
const expanded=e=>`{${e.Namespace||''}}${e.Name}`;
function knownRule(e){return profile()?.Rules?.findIndex(r=>r.Selector===expanded(e)||r.Selector===e.Name)??-1}
function editKnown(e){const i=knownRule(e);if(i>=0){editRule(i);return}editRule(-1,{...commonRule(),Selector:expanded(e),Path:'',Origin:'',NoInherit:true,Order:'default'});activeKnown=e;renderGroups();fillSuggestions();$('ruleTitle').textContent=e.Name;$('origin').textContent='Общая настройка';$('ruleNote').textContent='У этого элемента нет собственной настройки. Сейчас действует «Все элементы». Изменение настройки создаст отдельное правило при сохранении файла.'}
function error(message){$('error').textContent=message;$('error').hidden=!message}
async function api(path,body){const res=await fetch('/api/'+path,{method:body===undefined?'GET':'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});const data=await res.json();if(!res.ok)throw Error(data.Error||'Ошибка запроса');return data}
async function action(fn){if(busy)return;busy=true;error('');document.querySelectorAll('button,input,select').forEach(n=>n.disabled=true);try{await fn();return true}catch(e){error(e.message);return false}finally{busy=false;document.querySelectorAll('button,input,select').forEach(n=>n.disabled=false);$('newRule').disabled=!profile();updateStatus()}}
function withUnsaved(fn){if(busy||pendingLeave)return;if(!(dirty||staged)){fn();return}pendingLeave=fn;$('unsavedDialog').showModal()}
function updateStatus(){const changed=dirty||staged;$('status').textContent=(changed?'Не сохранено':lastStatus)+' · Профилей: '+(state?.Database.Profiles?.length||0);$('status').classList.toggle('unsaved',changed);$('saveDatabase').disabled=busy||!changed}
function changed(){dirty=true;updateStatus()}
function stageEditor(){
 if(!dirty)return true;
 const form=$('profileEditor').hidden?$('editor'):$('profileEditor');
 if(!form.reportValidity())return false;
 const db=state.Database;db.Version=2;
 if(form.id==='profileEditor'){
  const root=$('rootName').value.trim(),namespace=$('rootNamespace').value.trim();
  if(!root){error('Укажите имя корневого элемента.');return false}
  db.Profiles??=[];
  if(db.Profiles.some(p=>p.Root===root&&p.Namespace===namespace)){error('Такой профиль уже существует.');return false}
  db.Profiles.push({Root:root,Namespace:namespace,Rules:[{Selector:'*',Mode:'auto',Order:'significant',Origin:'manual',Fields:[]}]});profileIndex=db.Profiles.length-1;ruleIndex=0;
 }else{
  const mode=$('mode').value,name=$('path').value.trim();
  const fields=['text','auto','name'].includes(mode)?[]:[...$('fields').children].map(row=>({Name:row.querySelector('[data-field="Name"]').value.trim(),Namespace:row.querySelector('[data-field="Namespace"]').value.trim(),Lexical:row.querySelector('[data-flag="Lexical"]').checked,TrimSpace:row.querySelector('[data-flag="TrimSpace"]').checked}));
  if(!name||(!['text','auto','name'].includes(mode)&&(!fields.length||fields.some(f=>!f.Name)))){error('Укажите имя элемента и хотя бы одно поле ключа.');return false}
  if(mode==='auto'&&name!=='*'){error('Автоматический способ доступен только для «Все элементы».');return false}
  const rule={Path:name.startsWith('/')?name:'',Selector:name.startsWith('/')?'':name,TrimSpace:$('trim').checked,NoInherit:true,AllowDeleteAdd:!!profile()?.Rules?.[ruleIndex]?.AllowDeleteAdd,Mode:mode,Order:$('order').value,Origin:'manual',Fields:fields};
  const p=profile();p.Rules??=[];
  if(p.Rules.some((r,i)=>i!==ruleIndex&&(r.Selector||r.Path)===name)){error('Правило для этого элемента уже существует.');return false}
  if(ruleIndex<0){p.Rules.push(rule);ruleIndex=p.Rules.length-1}else p.Rules[ruleIndex]=rule;
 }
 dirty=false;staged=true;activeKnown=null;error('');updateStatus();return true;
}
function navigate(fn){if(busy||!stageEditor())return;fn()}
async function saveFile(){if(busy||!(dirty||staged)||!stageEditor())return false;return action(async()=>{state=await api('database',{Database:state.Database,Revision:state.Revision});dirty=false;staged=false;lastStatus='Сохранено';render();if(profile()?.Rules?.[ruleIndex])editRule(ruleIndex);updateStatus()})}
function requestClose(){if(busy||closing)return;withUnsaved(()=>{closing=true;action(async()=>{try{await api('cancel',{})}catch(e){closing=false;throw e}})})}
window.xmlmergeRequestClose=requestClose;
window.addEventListener('beforeunload',e=>{if(!closing&&(dirty||staged)){e.preventDefault();e.returnValue=''}});
function render(){
 $('rulesFile').textContent=state.Options.Rules;$('rulesFile').title=state.Options.Rules;$('profile').replaceChildren();
 const profiles=state.Database.Profiles||[];if(profileIndex>=profiles.length)profileIndex=0;
 profiles.forEach((p,i)=>{const option=node('option',p.Root+(p.Namespace?' · '+p.Namespace:''));option.value=i;$('profile').append(option)});$('profile').value=String(profileIndex);
 $('profile').title=profile()?.Namespace==='*'?'Любое пространство имён':profile()?.Namespace||'Без пространства имён';$('groups').replaceChildren();
 renderGroups();
 $('empty').hidden=!!profile();$('profileEditor').hidden=true;$('editor').hidden=true;$('newRule').disabled=!profile();
 if(profile()&&!profile().Rules?.length){$('empty').hidden=false;$('empty').querySelector('h2').textContent='Добавьте первое правило';$('empty').querySelector('p').textContent='Укажите имя элемента: правило будет действовать на любой глубине в этом профиле.'}
 else if(!profile()){$('empty').querySelector('h2').textContent='Создайте профиль XML'}
 updateStatus();
}
function renderGroups(){
 $('groups').replaceChildren();
 const query=$('search').value.toLowerCase();
 const add=(title,subtitle,selected,click)=>{const b=node('button',undefined,'group'+(selected?' selected':'')),label=node('span');label.append(node('strong',title),node('small',subtitle));b.append(label);b.addEventListener('click',()=>{navigate(click)});$('groups').append(b)};
 if(profile()){const i=(profile().Rules||[]).findIndex(r=>r.Selector==='*');add('Все элементы','Общая настройка профиля',ruleIndex===i&&i>=0||ruleIndex===-2,()=>editRule(i>=0?i:-2));}
 (profile()?.Rules||[]).forEach((r,i)=>{if(r.Selector==='*'||!(r.Selector||r.Path).toLowerCase().includes(query))return;add((r.Selector||r.Path).replace(/^\{[^}]*\}/,''),'Собственное правило',ruleIndex===i,()=>editRule(i))});
 (profile()?.KnownElements||[]).forEach(e=>{if(knownRule(e)>=0||!(e.Name+' '+e.Namespace).toLowerCase().includes(query))return;add(e.Name,'Используется «Все элементы»',activeKnown&&expanded(activeKnown)===expanded(e),()=>editKnown(e))});
}
function addField(field={}){const row=node('div',undefined,'keyField');for(const [key,title,placeholder] of [['Name','Имя','Например, uuid'],['Namespace','Пространство имен','Необязательно']]){const label=node('label',title),input=node('input');input.value=field[key]||'';input.placeholder=placeholder;input.dataset.field=key;if(key==='Name')input.setAttribute('list','knownFields');input.required=key==='Name';label.append(input);row.append(label)}for(const [key,title] of [['Lexical','Имя с префиксом'],['TrimSpace','Убрать краевые пробелы в ключе']]){const l=node('label',title),cb=node('input');cb.type='checkbox';cb.dataset.flag=key;cb.checked=!!field[key];l.append(cb);row.append(l)}const remove=node('button','×','quiet');remove.type='button';remove.setAttribute('aria-label','Удалить поле');remove.addEventListener('click',()=>{row.remove();changed()});row.append(remove);$('fields').append(row)}
function fillSuggestions(){const list=activeKnown?[activeKnown]:(profile()?.KnownElements||[]);const key=$('mode').value==='element'?'Elements':'Attributes';const names=[...new Set(list.flatMap(e=>(e[key]||[]).map(f=>f.Name)))].sort();$('knownFields').replaceChildren(...names.map(n=>{const o=node('option');o.value=n;return o}))}
function modeChanged(){$('textHint').textContent=$('mode').value==='name'?'Ключ — имя элемента и пространство имён. Для повторяющихся элементов этого имени правило не подходит.':'Используется собственное текстовое значение без текста потомков.';fillSuggestions();const text=['text','auto','name'].includes($('mode').value);$('fieldSection').hidden=text;$('textHint').hidden=!text;$('fields').querySelectorAll('input[data-field="Name"]').forEach(n=>n.required=!text)}
function editRule(index,draft){ruleIndex=index;activeKnown=null;render();const existing=profile()?.Rules?.[index];const r=draft||existing||(index===-2?commonRule():null);const isDefault=r?.Selector==='*';$('empty').hidden=true;$('editor').hidden=false;$('ruleTitle').textContent=isDefault?'Все элементы':existing?'Редактирование правила':'Новое правило';$('ruleNote').textContent=isDefault?'Эта настройка применяется ко всем тегам данного корня, для которых не задано собственное правило. Изменение применяется ко всем таким элементам после сохранения файла.':'Собственное правило применяется к этому имени на любой глубине. Без него действует общая настройка «Все элементы».';$('path').value=r?.Selector||r?.Path||'';$('nameEntry').hidden=isDefault;$('mode').value=r?.Mode||'attribute';$('orderDefault').hidden=isDefault;$('order').value=r?.Order||(profile()?.Rules?.some(r=>r.Selector==='*')?'default':'significant');$('trim').checked=!!r?.TrimSpace;$('inherit').checked=!r?.NoInherit;$('origin').textContent=r?.Origin==='imported'?'Импортировано':r?.Origin==='automatic'?'Создано автоматически':r?'Задано вручную':'';$('deleteRule').hidden=!existing||isDefault;$('fields').replaceChildren();if(r?.Fields?.length)r.Fields.forEach(addField);else addField();modeChanged();dirty=false}
$('newProfile').addEventListener('click',()=>navigate(()=>{render();$('empty').hidden=true;$('profileEditor').hidden=false;$('rootName').value='';$('rootNamespace').value='';dirty=false;$('rootName').focus()}));
$('newRule').addEventListener('click',()=>navigate(()=>editRule(-1)));
$('profile').addEventListener('change',()=>{const next=Number($('profile').value);if(!stageEditor()){$('profile').value=String(profileIndex);return}profileIndex=next;ruleIndex=-1;render();if(profile()?.Rules?.length)editRule(0)});
for(const id of ['editor','profileEditor'])$(id).addEventListener('input',changed);
$('mode').addEventListener('change',modeChanged);$('addField').addEventListener('click',()=>{addField();modeChanged();changed()});
$('profileEditor').addEventListener('submit',e=>{e.preventDefault();dirty=true;if(stageEditor())editRule(0)});
$('editor').addEventListener('submit',e=>{e.preventDefault();saveFile()});
$('saveDatabase').addEventListener('click',saveFile);
document.addEventListener('keydown',e=>{if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='s'){e.preventDefault();saveFile()}});
$('deleteRule').addEventListener('click',()=>$('confirm').showModal());$('reject').addEventListener('click',()=>$('confirm').close());$('accept').addEventListener('click',()=>{profile().Rules.splice(ruleIndex,1);dirty=false;staged=true;ruleIndex=-1;render();if(profile()?.Rules?.length)editRule(0);$('confirm').close()});
$('search').addEventListener('input',renderGroups);
$('reload').addEventListener('click',()=>withUnsaved(()=>action(async()=>{state=await api('reload',{});dirty=false;staged=false;lastStatus='Загружено';ruleIndex=-1;render();if(profile()?.Rules?.length)editRule(0)})));
$('close').addEventListener('click',requestClose);
$('leaveCancel').addEventListener('click',()=>{$('unsavedDialog').close();pendingLeave=null});
$('unsavedDialog').addEventListener('cancel',()=>pendingLeave=null);
$('leaveDiscard').addEventListener('click',()=>{const fn=pendingLeave;pendingLeave=null;$('unsavedDialog').close();fn?.()});
$('leaveSave').addEventListener('click',async()=>{const fn=pendingLeave;pendingLeave=null;$('unsavedDialog').close();if(await saveFile())fn?.()});
if(window.xmlmergeEditorReady)window.xmlmergeEditorReady();
action(async()=>{state=await api('state');render();if(profile()?.Rules?.length)editRule(0)});
