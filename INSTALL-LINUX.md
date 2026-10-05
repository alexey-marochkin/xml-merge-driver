# XML Merge: установка в Linux (x86-64)

Скачайте `XmlMerge-Linux-x64-<версия>.tar.gz` из [Releases](https://github.com/alexey-marochkin/xml-merge-driver/releases), распакуйте и запустите установщик:

```sh
tar -xzf XmlMerge-Linux-x64-<версия>.tar.gz -C /пустая/папка
cd /пустая/папка
sh ./Install-XmlMerge.sh
```

Он поместит `xmlmerge`, `xmlmerge-ui`, `rules.xml` и `merge-policy.xml` в `~/.local/share/XmlMerge` (или в `$XDG_DATA_HOME/XmlMerge`) и настроит драйвер `xmlmerge` в пользовательском Git config. Можно передать установщику другой каталог первым аргументом. При повторной установке существующие правила и политика сохраняются; прежняя команда Git-драйвера, если была, записывается в `git-driver.previous.txt`.

В репозитории укажите `merge=xmlmerge` только для нужных XML-файлов в `.gitattributes`. Не назначайте этот драйвер для `*.txt`. `ConfigDumpInfo.xml` должен оставаться под управлением драйвера: политика сохраняет локальную версию этого файла. Проверка:

```sh
git config --global --get merge.xmlmerge.driver
git -C /путь/к/репозиторию check-attr merge -- Config/путь/к/файлу.xml
"${XDG_DATA_HOME:-$HOME/.local/share}/XmlMerge/xmlmerge" rules validate "${XDG_DATA_HOME:-$HOME/.local/share}/XmlMerge/rules.xml"
```

Редактор правил открывается командой `xmlmerge-ui` из каталога установки. Linux-версия использует браузер и локальный веб-сервер на автоматически выбранном порту. Нужны графическая сессия, браузер и `xdg-open`; если браузер не открылся автоматически, скопируйте адрес из терминала. Закрывайте редактор его кнопкой **«Закрыть»**, чтобы остановить сервер. При закрытии вкладки браузера процесс остаётся запущенным; его можно остановить Ctrl+C в терминале.

Для ручного слияния вызовите `xmlmerge mergetool --rules ... --base ... --local ... --remote ... --output ...` с полными путями к файлам. Автоматический CLI-драйвер работает и без графического окружения, например в CI. XML-файлы политики и правил изменяйте осторожно: общее описание поведения находится в [MERGE-DRIVER-POLICY.md](https://github.com/alexey-marochkin/xml-merge-driver/blob/main/MERGE-DRIVER-POLICY.md).
