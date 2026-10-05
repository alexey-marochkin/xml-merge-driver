"""Convert the supplied Oso XML Merge specifications to XML Merge rules v2.

Only explicit identity settings become keys. Observed attributes are not keys.
The input has QName selectors but neither document paths nor namespace bindings.
"""
import argparse
import hashlib
from pathlib import Path
import xml.etree.ElementTree as ET


def convert(source):
    database = ET.Element('xmlmerge', version='2')
    report = [
        'Импорт спецификаций .pmfmt в XML Merge, формат правил версии 2',
        'Источник описания: https://osocorporation.com/doc.php?page=formatspecs&product=xmlmerge',
        '',
        'Соответствия:',
        '  file/@root -> profile/@root; namespace="*" означает любой URI корня.',
        '  element/@name -> rule/@selector (имя элемента на любой глубине).',

        '  Вложенное правило дочернего ключа -> короткий контекст parent/child, также на любой глубине.',
        '  no-inherit=true сохраняет исходную модель: именованные правила не становятся ключами потомков.',
        '  element name="*" -> настройка по умолчанию для профиля.',
        '  Sequenced -> significant; Random -> insignificant; UseDefault -> default (ссылка на *).',
        '  Вложенные attribute -> attribute; вложенные element -> element; content -> text.',
        '  Пустое правило * без ключа -> auto: подбор ключей при загрузке XML.',
        '  content -> trim-space="true": обрезка краевых XML-пробелов только в ключе.',
        '',
        'Ограничения и сохранность:',
        '  URI и полные пути отсутствуют в исходниках: они не подставлялись по догадке.',
        '  selector и lexical="true" сопоставляют исходные имена с префиксами буквально.',
        '  Смена префикса может потребовать нового правила; ключ узла по-прежнему включает URI.',
        '  Конкретные правила по расширенным путям имеют приоритет над шаблоном этого узла.',
        '  атрибут attributes — каталог наблюдавшихся полей, а не выбор ключа; не импортируется.',
        '  Записи с UseDefault без дочернего ключа не создают отдельное правило.',
        '  filenames не используется: все профили имеют разные корни.',
        '  preprocess=DoNothing сохранен поведением движка: XML не нормализуется.',
        '  Комментарии DoNothing остаются конфликтами; политика ResolveRight не поддерживается.',
        '  text в нашем движке допустим только для скалярных узлов; контейнеры требуют другого ключа.',
        '  Уникальность/наличие ключей проверяется на реальных тройках XML перед сравнением.',
        '  Для явного импортированного ключа ошибка не запускает молчаливую замену ключа.',
        '  Это перенос настроек, а не утверждение о полноте правил для всех документов 1С.',
        '',
    ]
    roots, total, known = set(), 0, 0
    files = sorted(source.glob('*.pmfmt'), key=lambda p:p.name.casefold())
    if not files:
        raise ValueError('В каталоге нет .pmfmt')
    for path in files:
        data = path.read_bytes()
        if b'<!DOCTYPE' in data.upper():
            raise ValueError(f'{path.name}: DTD не поддерживается')
        doc = ET.fromstring(data)
        root = doc.get('root')
        if doc.tag != 'file' or not root or root in roots:
            raise ValueError(f'{path.name}: некорректный или повторяющийся корень')
        roots.add(root)
        if doc.get('filenames', '*.*') != '*.*':
            raise ValueError(f'{path.name}: специфичная маска файлов требует ручного переноса')
        elements = list(doc)
        defaults = [e for e in elements if e.get('name') == '*']
        if len(defaults) != 1:
            raise ValueError(f'{path.name}: требуется одна настройка *')
        default = defaults[0]
        order = {'Sequenced':'significant', 'Random':'insignificant'}[default.get('order')]
        profile = ET.SubElement(database, 'profile', root=root, namespace='*')
        selectors, count = set(), 0
        for element in elements:
            name = element.get('name')
            if element.tag != 'element' or not name or name in selectors:
                raise ValueError(f'{path.name}: некорректное или повторяющееся имя {name}')
            selectors.add(name)
            if element.get('preprocess') not in ('DoNothing', 'UseDefault'):
                raise ValueError(f'{path.name}: неизвестная обработка текста')
            if element.get('comments') not in ('DoNothing', 'UseDefault', 'ResolveRight'):
                raise ValueError(f'{path.name}: неизвестная политика комментариев')
            children = list(element)
            kinds = {c.tag for c in children}
            if len(kinds) > 1 or kinds - {'content','attribute','element'}:
                raise ValueError(f'{path.name}: смешанные ключи требуют ручного переноса')
            if not children and name != '*' and element.get('order') == 'UseDefault':
                continue
            selected_order = 'default' if element.get('order') == 'UseDefault' else {'Sequenced':'significant','Random':'insignificant'}[element.get('order')]
            mode = {'content':'text','attribute':'attribute','element':'element'}.get(next(iter(kinds), ''), 'auto')
            if mode == 'auto' and name != '*':
                raise ValueError(f'{path.name}: отдельная настройка порядка без ключа не поддерживается')
            rule = ET.SubElement(profile, 'rule', selector=name, mode=mode, order=selected_order, origin='imported')
            if name != '*': rule.set('no-inherit', 'true')
            if mode == 'text':
                if len(children) != 1:
                    raise ValueError(f'{path.name}: несколько content')
                rule.set('trim-space', 'true')
            if mode in ('attribute','element'):
                for child in children:
                    child_content = list(child)
                    if not child.get('name') or any(c.tag != 'content' or list(c) for c in child_content) or len(child_content) > 1:
                        raise ValueError(f'{path.name}: вложенная схема ключа не поддерживается')
                    field = ET.SubElement(rule, 'field', name=child.get('name'), lexical='true')
                    if mode == 'element' and (child_content or default.find('content') is not None):
                        field.set('trim-space', 'true')
                    if child_content:
                        ET.SubElement(profile, 'rule', selector=name+'/'+child.get('name'), mode='text', order=selected_order, origin='imported', **{'trim-space':'true', 'no-inherit':'true'})
                        count += 1
            count += 1
        total += count
        known += len(elements)
        report.append(f'{path.name}: root={root}; описаний элементов={len(elements)}; правил={count}; порядок={order}')
        report.append(f'  SHA-256: {hashlib.sha256(data).hexdigest()}')
        for rule in profile:
            fields = ', '.join(f.get('name') for f in rule)
            report.append(f'  {rule.get("selector")}: {rule.get("mode")}' + (f' [{fields}]' if fields else ''))
        if default.get('comments') == 'ResolveRight':
            report.append('  НЕ ПЕРЕНЕСЕНО: ResolveRight — конфликтные комментарии не будут автоматически взяты из remote.')
        report.append('')
    report.append(f'Итого: файлов={len(files)}; описаний элементов={known}; профилей={len(roots)}; правил={total}.')
    ET.indent(database, space='  ')
    return ET.tostring(database, encoding='utf-8', xml_declaration=True)+b'\n', '\n'.join(report)+'\n'


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, default=Path('.formats'))
    parser.add_argument('--output', type=Path, default=Path('rules.xml'))
    parser.add_argument('--report', type=Path, default=Path('rules-import-report.txt'))
    args = parser.parse_args()
    xml, report = convert(args.source)
    # Never replace a user's existing rules or a previous report silently.
    if args.output.exists() or args.report.exists():
        raise SystemExit('Выходной файл уже существует; выберите новые --output и --report.')
    args.output.write_bytes(xml)
    args.report.write_text(report, encoding='utf-8')
    print(report.splitlines()[-1])
