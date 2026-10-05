const {test}=require('node:test');const assert=require('node:assert/strict');const diff=require('../internal/ui/assets/line-diff.js');
test('three versions stay lossless; padding never leaks into XML',()=>{
 const samples=[['a\nb\nc','a\nL\nb\nc','a\nb\nc','a\nb\nR\nc'],['a\nx\nz','a\nleft\nz','a\nx\nz','a\nright\nz'],['a\nb','a','a\nb','a\nnew\nb'],['','','','x'],['a\n','a\n\n','a\n','a\n'],['<r/>','<r/>','<r>\n<x/>\n</r>','<r/>']];
 for(const texts of samples){const cs=diff.align(...texts);for(let side=0;side<4;side++)assert.equal(cs.flatMap(c=>c.values[side]).join('\n'),texts[side]);assert.equal(diff.result(cs),texts[2]);}
});
test('independent left and right additions can both be taken',()=>{
 const base='<r>\n<a/>\n<b/>\n</r>',local='<r>\n<L/>\n<a/>\n<b/>\n</r>',remote='<r>\n<a/>\n<R/>\n<b/>\n</r>';let cs=diff.align(base,local,base,remote);
 assert.equal(cs.filter(c=>c.conflict).length,0);
 for(const c of cs){if(c.values[1].includes('<L/>'))c.values[2]=c.values[1].slice();if(c.values[3].includes('<R/>'))c.values[2]=c.values[3].slice()}
 assert.equal(diff.result(cs),'<r>\n<L/>\n<a/>\n<R/>\n<b/>\n</r>');
});
test('overlapping edits are conflicts until the center takes a side',()=>{
 let cs=diff.align('a\nx\nz','a\nleft\nz','a\nx\nz','a\nright\nz');assert.equal(cs.filter(c=>c.conflict).length,1);
 cs.find(c=>c.conflict).values[2]=['right'];cs=diff.align('a\nx\nz','a\nleft\nz',diff.result(cs),'a\nright\nz');assert.equal(cs.filter(c=>c.conflict).length,0);
});
test('matched suffix stays at the same visual row after an insertion',()=>{
 const cs=diff.align('a\nb','a\nnew\nb','a\nb','a\nb');let row=0;const positions=[];for(const c of cs){for(let s=1;s<4;s++){const at=c.values[s].indexOf('b');if(at>=0)positions[s]=row+at}row+=Math.max(1,...c.values.slice(1).map(v=>v.length))}assert.equal(positions[1],positions[2]);assert.equal(positions[2],positions[3]);
});
test('large input uses bounded alignment and roundtrips',()=>{const base=Array.from({length:3000},(_,i)=>'<i>'+i+'</i>').join('\n'),local=base.replace('<i>1500</i>','<new/>\n<i>1500</i>'),cs=diff.align(base,local,base,base);assert.equal(diff.result(cs),base);assert.equal(cs.flatMap(c=>c.values[1]).join('\n'),local)});

test('provenance marks automatic left, right, shared and deleted changes, never BASE or a mixture',()=>{
 const origin=(b,l,c,r)=>diff.origin({values:[b,l,c,r].map(diff.lines)});
 assert.deepEqual(origin('0','L','L','0'),[true,false]);
 assert.deepEqual(origin('0','0','R','R'),[false,true]);
 assert.deepEqual(origin('0','X','X','X'),[true,true]);
 assert.deepEqual(origin('0','L','0','R'),[false,false]);
 assert.deepEqual(origin('0','L','manual','R'),[false,false]);
 assert.deepEqual(origin('0','','','0'),[true,false]);
 const cs=diff.align('a\nb','L1\nL2','L1\nR2','R1\nR2');
 assert.equal(cs.length,2);assert.deepEqual(cs.map(diff.origin),[[true,false],[false,true]]);
});

test('insert above/below preserves the center and neighbouring blocks from either source',()=>{
 const chunks=diff.align('head\nbase\ntail','head\nleft\ntail','head\nmanual\ntail','head\nright\ntail');
 const index=chunks.findIndex(c=>c.changed),snapshot=JSON.stringify(chunks);
 for(const [side,source] of [[1,'left'],[3,'right']]){
  assert.equal(diff.transfer(chunks,index,side,'before'),`head\n${source}\nmanual\ntail`);
  assert.equal(diff.transfer(chunks,index,side,'after'),`head\nmanual\n${source}\ntail`);
  assert.equal(diff.transfer(chunks,index,side,'replace'),`head\n${source}\ntail`);
 }
 assert.equal(JSON.stringify(chunks),snapshot,'preview transfer must not mutate its input');
});
test('multiline insertion, empty source and empty result keep exact text without screen padding',()=>{
 const chunks=[{values:[['base'],['left1','left2'],['middle1','middle2'],[]]}];
 assert.equal(diff.transfer(chunks,0,1,'before'),'left1\nleft2\nmiddle1\nmiddle2');
 assert.equal(diff.transfer(chunks,0,1,'after'),'middle1\nmiddle2\nleft1\nleft2');
 assert.equal(diff.transfer(chunks,0,3,'before'),'middle1\nmiddle2');
 assert.equal(diff.transfer(chunks,0,3,'after'),'middle1\nmiddle2');
 assert.equal(diff.transfer(chunks,0,3,'replace'),'');
 chunks[0].values[2]=[];
 assert.equal(diff.transfer(chunks,0,1,'after'),'left1\nleft2');
 assert.equal(diff.transfer(chunks,0,1,'before'),'left1\nleft2');
});

test('source-line anchor survives changed chunk counts after insertion and deletion',()=>{
 const base='<r>\n<a>base</a>\n<b/>\n<c>base</c>\n</r>',local='<r>\n<a>left</a>\n<b/>\n<c>left</c>\n</r>',remote='<r>\n<a>right</a>\n<b/>\n<c>right</c>\n</r>';
 const old=diff.align(base,local,base,remote),index=old.findIndex(c=>c.changed);
 for(const mode of ['replace','before','after']){
  const next=diff.align(base,local,diff.transfer(old,index,1,mode),remote);
  for(const side of [1,3])for(let line=0;line<diff.lines(side===1?local:remote).length;line++){
   const at=diff.locateLine(next,side,line);
   assert.equal(next[at.index].values[side][at.row],diff.lines(side===1?local:remote)[line]);
  }
 }
 const next=diff.align('a\nb\nc','a\nb\nc','a\nc','a\nc');
 const at=diff.locateLine(next,3,1);assert.equal(next[at.index].values[3][at.row],'c');
 assert.deepEqual(diff.locateLine([],3,0),{index:0,row:0});
});
