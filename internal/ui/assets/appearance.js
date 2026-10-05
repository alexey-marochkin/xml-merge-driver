'use strict';
(() => {
 const root=document.documentElement,button=document.getElementById('themeToggle'),splitter=document.getElementById('sidebarSplitter'),layout=document.getElementById('rulesView');
 const secret=location.hash.slice(1)||sessionStorage.getItem('xmlmerge-token');
 let preferredWidth=320,revision=0,queue=Promise.resolve(),drag=null;
 const notice=document.createElement('div');notice.className='settingsNotice';notice.hidden=true;notice.setAttribute('role','alert');document.body.append(notice);
 function applyTheme(theme){root.dataset.theme=theme;button.setAttribute('aria-pressed',String(theme==='dark'));const label=theme==='dark'?'Включить светлую тему':'Включить тёмную тему';button.setAttribute('aria-label',label);button.title=label;}
 function maximum(){return Math.max(220,Math.min(720,(layout.clientWidth||window.innerWidth)-448));}
 function applyWidth(width){const bounded=Math.round(Math.max(220,Math.min(maximum(),width)));root.style.setProperty('--sidebar-width',bounded+'px');splitter.setAttribute('aria-valuenow',String(bounded));splitter.setAttribute('aria-valuemax',String(maximum()));return bounded;}
 async function request(patch){const response=await fetch('/api/settings',{method:patch?'POST':'GET',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:patch?JSON.stringify(patch):undefined});const data=await response.json();if(!response.ok)throw Error(data.Error||'Не удалось сохранить настройки интерфейса');return data;}
 function persist(patch){revision++;queue=queue.then(()=>request(patch)).then(()=>{notice.hidden=true}).catch(e=>{notice.textContent=e.message;notice.hidden=false});}
 applyTheme(root.dataset.theme||'light');applyWidth(preferredWidth);
 const initial=revision;request().then(p=>{if(revision!==initial)return;applyTheme(p.Theme);preferredWidth=p.SidebarWidth;applyWidth(preferredWidth)}).catch(e=>{notice.textContent=e.message;notice.hidden=false});
 button.addEventListener('click',()=>{const theme=root.dataset.theme==='dark'?'light':'dark';applyTheme(theme);persist({Theme:theme})});
 splitter.addEventListener('pointerdown',e=>{if(e.button!==0)return;revision++;drag={id:e.pointerId,x:e.clientX,width:Number(splitter.getAttribute('aria-valuenow')),preferred:preferredWidth};splitter.setPointerCapture(e.pointerId);splitter.focus();document.body.classList.add('resizing');e.preventDefault()});
 splitter.addEventListener('pointermove',e=>{if(!drag||e.pointerId!==drag.id)return;preferredWidth=applyWidth(drag.width+e.clientX-drag.x)});
 function finish(e){if(!drag||e.pointerId!==drag.id)return;const previous=drag;drag=null;document.body.classList.remove('resizing');if(e.type==='pointercancel'){preferredWidth=previous.preferred;applyWidth(preferredWidth)}else persist({SidebarWidth:preferredWidth});if(splitter.hasPointerCapture(e.pointerId))splitter.releasePointerCapture(e.pointerId);}
 splitter.addEventListener('pointerup',finish);splitter.addEventListener('pointercancel',finish);
 splitter.addEventListener('lostpointercapture',e=>{if(drag){drag=null;document.body.classList.remove('resizing');persist({SidebarWidth:preferredWidth})}});
 splitter.addEventListener('keydown',e=>{let next=Number(splitter.getAttribute('aria-valuenow'));const step=e.shiftKey?64:16;if(e.key==='ArrowLeft')next-=step;else if(e.key==='ArrowRight')next+=step;else if(e.key==='Home')next=220;else if(e.key==='End')next=maximum();else return;e.preventDefault();preferredWidth=applyWidth(next);persist({SidebarWidth:preferredWidth})});
 splitter.addEventListener('dblclick',()=>{preferredWidth=applyWidth(320);persist({SidebarWidth:preferredWidth})});
 window.addEventListener('resize',()=>applyWidth(preferredWidth));
})();
