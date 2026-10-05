# XML Merge: инструкция для сотрудников (Windows)

XML Merge автоматически объединяет XML по структуре, а при конфликте открывает окно с тремя версиями документа. Git вызывает консольную программу `xmlmerge.exe`; в Git Extensions для ручного решения конфликтов используется `xmlmerge-select.exe`, который направляет XML в `xmlmerge-ui.exe`, а остальные файлы — в Araxis Merge. Рабочие правила сопоставления узлов находятся в `rules.xml`, политика выбора версии поставщика — в `merge-policy.xml`.

## 1. Установка готового комплекта

Скачайте `XmlMerge-Windows-x64-<версия>.zip` из [раздела Releases](https://github.com/alexey-marochkin/xml-merge-driver/releases), распакуйте и запустите `Install-XmlMerge.ps1` в PowerShell:

```powershell
cd <папка_с_распакованным_архивом>
.\Install-XmlMerge.ps1
```

Скрипт положит следующие файлы в `%LOCALAPPDATA%\Programs\XmlMerge` текущего пользователя Windows:

```text
xmlmerge.exe          автоматический драйвер Git
xmlmerge-ui.exe       окно правил и трёхстороннего сравнения
xmlmerge-select.exe   выбор XML Merge или Araxis в Git Extensions
rules.xml            готовые правила для XML 1С
merge-policy.xml     политика веток и файлов поставщика
```

Если вы получили файлы отдельно, положите **все пять файлов в одну папку с этими именами**, затем запустите тот же скрипт. Правила и политику не следует класть в репозиторий выгрузки ERP: это настройки инструмента. Программа не требует установленного Go или Python. Для окна нужен Microsoft Edge WebView2 Runtime, для слияния — Git for Windows. На общем терминальном сервере каждый сотрудник устанавливает комплект в свой профиль и запускает своё окно; свободный локальный порт выбирается автоматически.

Установщик сохраняет существующие `rules.xml` и `merge-policy.xml` в папке установки при повторном запуске. Чтобы обновить их вручную, сохраните старые копии и замените файлы из нового комплекта. Файл `settings.xml` с темой и шириной панели хранится отдельно в `%LOCALAPPDATA%\XmlMerge`.

## 2. Git: автоматическое слияние

`Install-XmlMerge.ps1` прописывает в пользовательском Git config драйвер `xmlmerge` и пути к установленным `rules.xml` и `merge-policy.xml`. Существующее значение `merge.xmlmerge.driver` перед изменением сохраняется в `git-driver.previous.txt` рядом с программой. Настройка действует для всех репозиториев этого пользователя, **но Git запускает драйвер только для файлов, назначенных через `.gitattributes` конкретного репозитория**.

В репозитории ERP проверьте его `.gitattributes`. Для обрабатываемых XML там должно быть `merge=xmlmerge`, например:

```gitattributes
*.xml merge=xmlmerge
```

Не добавляйте `*.txt`: содержимое TXT надо проверять отдельно, и оно не всегда XML. `ConfigDumpInfo.xml` должен оставаться направленным в драйвер: политика сохраняет нашу версию этого служебного файла. Если правила или политика всё-таки хранятся внутри отдельного репозитория, исключите их из структурного слияния явной строкой `merge=text`.

Проверка после установки:

```powershell
git config --global --get merge.xmlmerge.driver
git config --global --get merge.xmlmerge.recursive
git -C <путь_к_репозиторию_ERP> check-attr merge -- Config/путь/к/файлу.xml
& "$env:LOCALAPPDATA\Programs\XmlMerge\xmlmerge.exe" rules validate "$env:LOCALAPPDATA\Programs\XmlMerge\rules.xml"
```

Первые две команды должны показать команду `git-driver` и `binary`, проверка атрибута — `xmlmerge`, а последняя команда — число профилей правил. В настройках политики перечислены допустимые имена веток поставщика; автоматически выбираемая сторона определяется по текущей ветке или по основной цепочке коммитов поставщика. Сообщение о выборе local/remote и причине Git показывает для каждого файла. Подробный журнал можно включить, добавив `--verbose` в команду драйвера.

## 3. Git Extensions: открытие конфликтов

Если для остальных форматов вы используете Araxis Merge, укажите его `Compare.exe`:

```powershell
.\Configure-GitExtensions.ps1 -RepositoryPath <путь_к_репозиторию_ERP> -AraxisExe "C:\Program Files\Araxis\Araxis Merge\Compare.exe"
```

Скрипт добавит **локально для этого репозитория** `merge.guitool=xmlmerge-select` и команду `mergetool.xmlmerge-select.cmd`. Путь к Araxis замените на установленный у вас; скрипт проверяет его наличие. Конфигурация других репозиториев не меняется. В Git Extensions заново откройте окно разрешения конфликтов. Для XML выберите файл и нажмите **«Открыть в»** — откроется XML Merge. Для остальных файлов останется Araxis.

Проверка:

```powershell
git -C <путь_к_репозиторию_ERP> config --local --get merge.guitool
git -C <путь_к_репозиторию_ERP> config --local --get mergetool.xmlmerge-select.cmd
```

Если Araxis у вас нет, автоматический XML-драйвер из шага 2 уже работает. Ручное окно XML можно запустить из PowerShell командой ниже, подставив пути к трём версиям и итоговому файлу. При этом не назначайте XML Merge общим инструментом для не-XML файлов.

```powershell
& "$env:LOCALAPPDATA\Programs\XmlMerge\xmlmerge.exe" mergetool --rules "$env:LOCALAPPDATA\Programs\XmlMerge\rules.xml" --base <BASE.xml> --local <LOCAL.xml> --remote <REMOTE.xml> --output <RESULT.xml>
```

После конфликта окно сначала предложит настроить недостающие правила идентификации узлов. Когда правил хватит, появится трёхстороннее сравнение: **LOCAL слева, BASE и собираемый РЕЗУЛЬТАТ по центру, REMOTE справа**. Выберите нужные изменения, затем нажмите **«Сохранить результат»**. Закрытие без сохранения оставляет конфликт нерешённым. В Git Extensions пометьте сохранённый файл как разрешённый (или выполните `git add`) и продолжите слияние.

## 4. Правила и обновления

Для просмотра или изменения базы откройте `xmlmerge-ui.exe` без параметров: он прочитает `rules.xml` из папки установки. Значок сохранения или Ctrl+S сохраняет всю базу. При работе нескольких людей согласуйте изменённый `rules.xml` и распространите его новым комплектом; личная копия сама не синхронизируется. `merge-policy.xml` редактируется как XML и перечитывается при каждом вызове драйвера.

При обновлении замените три EXE новым комплектом и повторно запустите установщик. Он не перезапишет локально изменённые правила и политику. Для возврата к прежнему Git-драйверу используйте сохранённую команду из `git-driver.previous.txt`. Подробности алгоритма выбора поставщика приведены в [MERGE-DRIVER-POLICY.md](MERGE-DRIVER-POLICY.md).

Источник правил Git: [gitattributes](https://git-scm.com/docs/gitattributes#_defining_a_custom_merge_driver) и [git-mergetool](https://git-scm.com/docs/git-mergetool). Параметры Git Extensions 6.0 описаны в [официальной документации](https://git-extensions-documentation.readthedocs.io/en/release-6.0/settings.html#config).
