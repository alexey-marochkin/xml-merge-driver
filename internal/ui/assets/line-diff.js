/* Pure line alignment. Screen padding is never part of a document. */
(function(root){
'use strict';
const lines=text=>text===''?[]:text.replace(/\r\n/g,'\n').split('\n');
const key=line=>line.trim();
function matches(a,b){
 const out=[];
 function solve(a0,a1,b0,b1){
  while(a0<a1&&b0<b1&&key(a[a0])===key(b[b0]))out.push([a0++,b0++]);
  const tail=[];while(a0<a1&&b0<b1&&key(a[a1-1])===key(b[b1-1]))tail.push([--a1,--b1]);
  const n=a1-a0,m=b1-b0;
  if(n&&m&&n*m<=2000000){
   const table=new Uint32Array((n+1)*(m+1)),w=m+1;
   for(let i=n-1;i>=0;i--)for(let j=m-1;j>=0;j--)table[i*w+j]=key(a[a0+i])===key(b[b0+j])?1+table[(i+1)*w+j+1]:Math.max(table[(i+1)*w+j],table[i*w+j+1]);
   let i=0,j=0;while(i<n&&j<m){if(key(a[a0+i])===key(b[b0+j]))out.push([a0+i++,b0+j++]);else if(table[(i+1)*w+j]>=table[i*w+j+1])i++;else j++}
  }else if(n&&m){
   // Patience anchors bound memory on large modules with mostly unchanged text.
   const am=new Map(),bm=new Map();for(let i=a0;i<a1;i++){const k=key(a[i]);am.set(k,am.has(k)?-1:i)}for(let j=b0;j<b1;j++){const k=key(b[j]);bm.set(k,bm.has(k)?-1:j)}
   const candidates=[];for(const [k,i]of am)if(i>=0&&bm.get(k)>=0)candidates.push([i,bm.get(k)]);
   const tails=[],prev=[];for(let i=0;i<candidates.length;i++){let lo=0,hi=tails.length;while(lo<hi){const mid=(lo+hi)>>1;if(candidates[tails[mid]][1]<candidates[i][1])lo=mid+1;else hi=mid}prev[i]=lo?tails[lo-1]:-1;tails[lo]=i}
   const chain=[];for(let i=tails.at(-1);i!==undefined&&i>=0;i=prev[i])chain.push(candidates[i]);chain.reverse();
   for(const [i,j]of chain){solve(a0,i,b0,j);out.push([i,j]);a0=i+1;b0=j+1}if(chain.length)solve(a0,a1,b0,b1);
  }
  out.push(...tail.reverse());
 }
 solve(0,a.length,0,b.length);return out;
}
function against(base,side){
 const cells=Array(base.length).fill(null),before=Array.from({length:base.length+1},()=>[]);
 let ai=0,bi=0;
 for(const [a,b]of [...matches(base,side),[base.length,side.length]]){
  const count=Math.min(a-ai,b-bi);for(let k=0;k<count;k++)cells[ai+k]=side[bi+k];
  for(let k=bi+count;k<b;k++)before[a].push(side[k]);
  if(a<base.length)cells[a]=side[b];ai=a+1;bi=b+1;
 }
 return{cells,before};
}
function align(baseText,localText,centerText,remoteText){
 const base=lines(baseText),sources=[lines(localText),lines(centerText),lines(remoteText)],maps=sources.map(s=>against(base,s)),rows=[];
 for(let i=0;i<=base.length;i++){
  const count=Math.max(...maps.map(m=>m.before[i].length));
  for(let j=0;j<count;j++)rows.push([null,...maps.map(m=>m.before[i][j]??null)]);
  if(i<base.length)rows.push([base[i],...maps.map(m=>m.cells[i])]);
 }
 const chunks=[];const offsets=[0,0,0,0];
 for(const row of rows){
  const [b,l,c,r]=row,changed=l!==b||r!==b||c!==b,conflict=l!==b&&r!==b&&l!==r&&c!==l&&c!==r;
  // Split adjacent independent edits and gaps, so an arrow never consumes its neighbour.
  const signature=changed?[l!==b,r!==b,c!==b,conflict,c===l,c===r,...row.map(v=>v===null)].join(':'):'same';
  let part=chunks.at(-1);if(!part||part.signature!==signature){part={signature,changed,conflict,values:[[],[],[],[]],starts:offsets.slice(),rows:0};chunks.push(part)}
  part.rows++;row.forEach((v,s)=>{if(v!==null){part.values[s].push(v);offsets[s]++}});
 }
 return chunks;
}
function result(chunks){return chunks.flatMap(c=>c.values[2]).join('\n')}
function transfer(chunks,index,side,mode='replace'){
 if(side!==1&&side!==3)throw Error('Invalid source side');
 if(!['replace','before','after'].includes(mode)||!chunks[index])throw Error('Invalid transfer');
 const source=chunks[index].values[side],center=chunks[index].values[2];
 const replacement=mode==='before'?[...source,...center]:mode==='after'?[...center,...source]:source;
 return chunks.flatMap((c,i)=>i===index?replacement:c.values[2]).join('\n');
}
// Locate a stable source line after the result has been realigned.
function locateLine(chunks,side,line){
 for(let index=0;index<chunks.length;index++){
  const c=chunks[index],count=c.values[side].length;
  if(count&&line<c.starts[side]+count)return{index,row:Math.max(0,line-c.starts[side])};
 }
 const index=Math.max(0,chunks.length-1);
 return{index,row:Math.max(0,(chunks[index]?.values[side].length||1)-1)};
}
function origin(chunk){
 const values=chunk.values.map(v=>JSON.stringify(v));
 return values[2]===values[0]?[false,false]:[values[2]===values[1],values[2]===values[3]];
}
const api={lines,matches,align,result,origin,transfer,locateLine};if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.LineDiff=api;
})(typeof window!=='undefined'?window:this);
