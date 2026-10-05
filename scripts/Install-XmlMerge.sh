#!/usr/bin/env sh
set -eu

source_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
destination=${1:-"${XDG_DATA_HOME:-$HOME/.local/share}/XmlMerge"}
mkdir -p -- "$destination"
destination=$(CDPATH= cd -- "$destination" && pwd)

for name in xmlmerge xmlmerge-ui rules.xml merge-policy.xml; do
  if [ ! -f "$source_dir/$name" ]; then
    printf 'Не найден файл комплекта: %s\n' "$name" >&2
    exit 2
  fi
done

for name in xmlmerge xmlmerge-ui; do
  if [ "$source_dir/$name" != "$destination/$name" ]; then
    cp -- "$source_dir/$name" "$destination/$name"
  fi
  chmod 755 -- "$destination/$name"
done
for name in rules.xml merge-policy.xml; do
  if [ ! -e "$destination/$name" ]; then
    cp -- "$source_dir/$name" "$destination/$name"
  else
    printf 'Сохранены действующие настройки: %s\n' "$destination/$name"
  fi
done

shell_quote() {
  printf "'"
  printf '%s' "$1" | sed "s/'/'\\\\''/g"
  printf "'"
}
binary=$(shell_quote "$destination/xmlmerge")
rules=$(shell_quote "$destination/rules.xml")
policy=$(shell_quote "$destination/merge-policy.xml")
driver="$binary git-driver --base \"%O\" --local \"%A\" --remote \"%B\" --path \"%P\" --remote-label \"%Y\" --rules $rules --policy $policy"
previous=$(git config --global --get merge.xmlmerge.driver || true)
if [ -n "$previous" ] && [ "$previous" != "$driver" ] && [ ! -e "$destination/git-driver.previous.txt" ]; then
  printf '%s\n' "$previous" > "$destination/git-driver.previous.txt"
fi
git config --global merge.xmlmerge.name 'Structural XML merge'
git config --global merge.xmlmerge.driver "$driver"
git config --global merge.xmlmerge.recursive binary
printf 'XML Merge установлен: %s\nGit-драйвер xmlmerge настроен для текущего пользователя.\n' "$destination"
