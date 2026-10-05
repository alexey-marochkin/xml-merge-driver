"""Read-only streaming XML inventory. No document content is copied into the report."""
import argparse
import json
import os
import time
from collections import Counter
from pathlib import Path
from xml.parsers import expat


def name(value):
    if '}' in value:
        uri, local = value.rsplit('}', 1)
        return (uri, local)
    return ('', value)


def scan(directory, output, file_list=None, quiet=False):
    profiles = {}
    errors = []
    counts = Counter()
    extensions = Counter()
    start = time.monotonic()
    folders = os.walk(directory) if file_list is None else [('', [], file_list)]
    for folder, dirs, files in folders:
        dirs[:] = sorted(d for d in dirs if d not in ('.git', '.svn', 'node_modules'))
        for filename in sorted(files):
            path = Path(folder) / filename
            ext = path.suffix.lower()
            extensions[ext] += 1
            if ext not in ('.xml', '.xsd', '.svg'):
                continue
            counts['candidates'] += 1
            # Keep changes file-local until parsing succeeds: malformed files
            # must not contaminate the inventory with partial observations.
            local = {}
            root = None
            stack = []
            def begin(tag, attrs):
                nonlocal root
                tag = name(tag)
                if root is None:
                    root = tag
                info = local.setdefault(tag, {'Count':0, 'Scalar':0, 'Attributes':set(), 'Elements':set()})
                info['Count'] += 1
                info['Attributes'].update(name(a) for a in attrs)
                if stack:
                    stack[-1][1] += 1
                    local[stack[-1][0]]['Elements'].add(tag)
                stack.append([tag, 0])
            def end(tag):
                tag, children = stack.pop()
                if children == 0:
                    local[tag]['Scalar'] += 1
            def reject_doctype(*args):
                raise ValueError('DTD: external entities are not loaded; document skipped')
            parser = expat.ParserCreate(namespace_separator='}')
            parser.StartElementHandler = begin
            parser.EndElementHandler = end
            parser.StartDoctypeDeclHandler = reject_doctype
            try:
                with path.open('rb') as stream:
                    parser.ParseFile(stream)
                if root is None:
                    raise ValueError('empty XML document')
            except (OSError, ValueError, expat.ExpatError) as exc:
                errors.append({'File':str(path.relative_to(directory)), 'Error':str(exc)})
                continue
            counts['parsed'] += 1
            profile = profiles.setdefault(root, {'Files':0,'Examples':[], 'Elements':{}})
            profile['Files'] += 1
            if len(profile['Examples']) < 3:
                profile['Examples'].append(str(path.relative_to(directory)))
            for tag, info in local.items():
                dest = profile['Elements'].setdefault(tag, {'Count':0,'Scalar':0,'Attributes':set(),'Elements':set()})
                for k in ('Count','Scalar'):
                    dest[k] += info[k]
                for k in ('Attributes','Elements'):
                    dest[k].update(info[k])
            if not quiet and counts['candidates'] % 2500 == 0:
                print(f'Parsed {counts["parsed"]} files; root types {len(profiles)}; elapsed {time.monotonic()-start:.0f}s', flush=True)
    def field(pair):
        return {'Namespace':pair[0], 'Name':pair[1]}
    result = {'Source':str(directory), 'Counts':dict(counts), 'Extensions':dict(extensions), 'Errors':errors, 'Profiles':[]}
    for root, profile in sorted(profiles.items()):
        result['Profiles'].append({'Root':root[1],'Namespace':root[0], 'Files':profile['Files'], 'Examples':profile['Examples'], 'Elements':[
            dict(field(tag), Count=info['Count'], Scalar=info['Scalar'], Attributes=[field(a) for a in sorted(info['Attributes'])], Elements=[field(a) for a in sorted(info['Elements'])]) for tag,info in sorted(profile['Elements'].items())]})
    if output is not None:
        output.write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8')
    if not quiet:
        print(f'Done: {counts["parsed"]} parsed, {len(errors)} errors, {len(profiles)} root types; {time.monotonic()-start:.1f}s',flush=True)
    return result


if __name__ == '__main__':
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument('source',type=Path)
    args.add_argument('output',type=Path)
    options = args.parse_args()
    if not options.source.is_dir():
        raise SystemExit('Source directory not found')
    scan(options.source, options.output)
