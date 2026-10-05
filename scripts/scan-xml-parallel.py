"""Bounded process pool for the streaming XML inventory scanner."""
import argparse
import importlib.util
import json
import os
import time
from collections import Counter
from concurrent.futures import ProcessPoolExecutor, wait, FIRST_COMPLETED
from pathlib import Path

spec = importlib.util.spec_from_file_location('scanner', Path(__file__).with_name('scan-xml.py'))
scanner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(scanner)


def batch(source, files):
    return scanner.scan(source, None, files, quiet=True)


def scan(source, output, workers):
    start = time.monotonic()
    files, extensions = [], Counter()
    for folder, dirs, names in os.walk(source):
        dirs[:] = sorted(d for d in dirs if d not in ('.git','.svn','node_modules'))
        for name in sorted(names):
            path = Path(folder)/name
            extensions[path.suffix.lower()] += 1
            if path.suffix.lower() in ('.xml','.xsd','.svg'):
                files.append(str(path))
    batches = iter(files[i:i+192] for i in range(0,len(files),192))
    profiles, errors, counts = {}, [], Counter()
    print(f'Found {len(files)} XML-based files; workers={workers}',flush=True)
    last = 0
    with ProcessPoolExecutor(max_workers=workers) as pool:
        pending = set()
        def submit():
            group = next(batches,None)
            if group is not None:
                pending.add(pool.submit(batch,source,group))
        for _ in range(workers*2):
            submit()
        while pending:
            done,_ = wait(pending,return_when=FIRST_COMPLETED)
            for future in done:
                pending.remove(future)
                result = future.result()
                counts.update(result['Counts'])
                errors.extend(result['Errors'])
                for p in result['Profiles']:
                    key = (p['Namespace'],p['Root'])
                    dst = profiles.setdefault(key,dict(Root=p['Root'],Namespace=p['Namespace'],Files=0,Examples=[],Elements={}))
                    dst['Files'] += p['Files']
                    dst['Examples'] = sorted(dst['Examples']+p['Examples'])[:3]
                    for e in p['Elements']:
                        tag = (e['Namespace'],e['Name'])
                        target = dst['Elements'].setdefault(tag,dict(Name=e['Name'],Namespace=e['Namespace'],Count=0,Scalar=0,Attributes=set(),Elements=set()))
                        for key in ('Count','Scalar'):
                            target[key] += e[key]
                        for key in ('Attributes','Elements'):
                            target[key].update((a['Namespace'],a['Name']) for a in e[key])
                submit()
            if counts['candidates']-last >= 5000:
                print(f'Processed {counts["candidates"]}/{len(files)}; roots={len(profiles)}; elapsed={time.monotonic()-start:.0f}s',flush=True)
                last = counts['candidates']
    result = dict(Source=str(source),Workers=workers,Counts=dict(counts),Extensions=dict(extensions),Errors=sorted(errors,key=lambda e:e['File']),Profiles=[])
    for _,p in sorted(profiles.items()):
        elements = []
        for _,e in sorted(p['Elements'].items()):
            for key in ('Attributes','Elements'):
                e[key] = [dict(Namespace=uri,Name=name) for uri,name in sorted(e[key])]
            elements.append(e)
        p['Elements'] = elements
        result['Profiles'].append(p)
    output.write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8')
    print(f'Done: parsed={counts["parsed"]}, errors={len(errors)}, roots={len(profiles)}, elapsed={time.monotonic()-start:.1f}s',flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source',type=Path)
    parser.add_argument('output',type=Path)
    parser.add_argument('--workers',type=int,default=4)
    args = parser.parse_args()
    if not args.source.is_dir() or not 1 <= args.workers <= 16:
        raise SystemExit('Invalid source directory or worker count')
    scan(args.source,args.output,args.workers)
