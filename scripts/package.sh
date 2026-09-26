#!/usr/bin/env bash
# Build and package only tracked working-tree files, including tracked edits.
set -euo pipefail
cd "$(dirname "$0")/.."
release="$PWD/dist/release"
mkdir -p "$release"
name=codex-phone-ops-0.2.0-linux
source_name=codex-phone-ops-0.2.0-source
unexpected=$(find "$release" -mindepth 1 -maxdepth 1 \
  ! -name "$name.tar.gz" ! -name "$source_name.tar.gz" ! -name SHA256SUMS.txt -print -quit)
if [[ -n $unexpected ]]; then
  printf 'Unexpected release artifact: %s\n' "$unexpected" >&2
  exit 1
fi
stage=$(mktemp -d "$PWD/dist/.package.XXXXXXXX")
trap 'rm -rf -- "$stage"' EXIT
mkdir -p "$stage/$source_name"
git ls-files -z -- go.mod .gitignore LICENSE install.sh README.md USER_MANUAL.md RECOVERY_CARD.md cmd internal scripts .github/workflows/check.yml > "$stage/source-files"
while IFS= read -r -d '' file; do
  if [[ ! -f $file || -L $file ]]; then
    printf 'source file missing or symlink: %s\n' "$file" >&2
    exit 1
  fi
  mkdir -p "$stage/$source_name/$(dirname "$file")"
  cp -- "$file" "$stage/$source_name/$file"
done < "$stage/source-files"
if [[ ! -f "$stage/$source_name/LICENSE" ]]; then
  printf 'LICENSE must be Git-tracked before packaging\n' >&2
  exit 1
fi
(cd "$stage/$source_name" && find . -type f ! -name CONTENTS.sha256 -print0 | sort -z | xargs -0 sha256sum > CONTENTS.sha256)
tar -czf "$stage/$source_name.tar.gz" -C "$stage" "$source_name"
bash "$stage/$source_name/scripts/build.sh"
mkdir -p "$stage/$name/dist" "$stage/$name/scripts"
cp "$stage/$source_name/"{LICENSE,install.sh,README.md,USER_MANUAL.md,RECOVERY_CARD.md} "$stage/$name/"
cp "$stage/$source_name/scripts/install-product.sh" "$stage/$name/scripts/"
cp "$stage/$source_name/dist/"{cpo-linux-amd64,cpo-linux-arm64,SHA256SUMS} "$stage/$name/dist/"
(cd "$stage/$name" && find . -type f ! -name CONTENTS.sha256 -print0 | sort -z | xargs -0 sha256sum > CONTENTS.sha256)
tar -czf "$stage/$name.tar.gz" -C "$stage" "$name"
(cd "$stage" && sha256sum "$name.tar.gz" "$source_name.tar.gz" > SHA256SUMS.txt)
mv -- "$stage/$name.tar.gz" "$stage/$source_name.tar.gz" "$stage/SHA256SUMS.txt" "$release/"
printf '配布候補: %s\n' "$release"
