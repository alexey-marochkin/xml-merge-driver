#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Usage: package-linux.sh MAJOR.MINOR.PATCH' >&2
  exit 2
fi

project="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stage="$(mktemp -d)"
trap 'rm -rf -- "$stage"' EXIT
mkdir -p "$project/dist"

(cd "$project" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$stage/xmlmerge" ./cmd/xmlmerge)
(cd "$project" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$stage/xmlmerge-ui" ./cmd/xmlmerge-ui)
cp "$project/rules.xml" "$project/merge-policy.xml" "$project/INSTALL-LINUX.md" \
  "$project/scripts/Install-XmlMerge.sh" "$stage/"
chmod 755 "$stage/xmlmerge" "$stage/xmlmerge-ui" "$stage/Install-XmlMerge.sh"

(cd "$stage" && sha256sum xmlmerge xmlmerge-ui rules.xml merge-policy.xml INSTALL-LINUX.md Install-XmlMerge.sh > SHA256SUMS.txt)
archive="$project/dist/XmlMerge-Linux-x64-$version.tar.gz"
tar -C "$stage" -czf "$archive" .
echo "Готов комплект: $archive"
