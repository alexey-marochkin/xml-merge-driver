#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Usage: package-windows-cross.sh MAJOR.MINOR.PATCH' >&2
  exit 2
fi

project="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for script in Install-XmlMerge.ps1 Configure-GitExtensions.ps1; do
  if [[ "$(od -An -tx1 -N3 "$project/scripts/$script" | tr -d ' \n')" != efbbbf ]]; then
    echo "PowerShell 5.1 requires a UTF-8 BOM: $script" >&2
    exit 1
  fi
done
stage="$(mktemp -d)"
trap 'rm -rf -- "$stage"' EXIT
mkdir -p "$project/dist"

(cd "$project" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -o "$stage/xmlmerge.exe" ./cmd/xmlmerge)
(cd "$project" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -ldflags '-H windowsgui' -o "$stage/xmlmerge-ui.exe" ./cmd/xmlmerge-ui)
(cd "$project" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -ldflags '-H windowsgui' -o "$stage/xmlmerge-select.exe" ./cmd/xmlmerge-select)

cp "$project/rules.xml" "$project/merge-policy.xml" "$project/INSTALL-RU.md" \
  "$project/scripts/Install-XmlMerge.ps1" \
  "$project/scripts/Configure-GitExtensions.ps1" "$stage/"

files=(
  Configure-GitExtensions.ps1 INSTALL-RU.md Install-XmlMerge.ps1
  merge-policy.xml rules.xml xmlmerge-select.exe xmlmerge-ui.exe xmlmerge.exe
)
(cd "$stage" && sha256sum "${files[@]}" > SHA256SUMS.txt)

archive="$project/dist/XmlMerge-Windows-x64-$version.zip"
(cd "$stage" && "${PYTHON:-python3}" -m zipfile -c "$archive" "${files[@]}" SHA256SUMS.txt)
echo "Готов комплект: $archive"
