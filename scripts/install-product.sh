#!/usr/bin/env bash
# Shared local installer. No network, SSH or settings migration.
set -euo pipefail
here=$(cd -- "$(dirname -- "$0")/.." && pwd -P)
binary=phoneops
name=phoneops
package=codex-phone-ops
widget='Codex PhoneOps'
marker=CODEX_PHONE_OPS_MANAGED
case "$(uname -m)" in aarch64|arm64) arch=arm64;; x86_64|amd64) arch=amd64;; *) echo '未対応CPU。変更せず終了。' >&2; exit 1;; esac
for cmd in sha256sum mktemp cp chmod mv mkdir bash; do command -v "$cmd" >/dev/null; done
file="$binary-linux-$arch"
(cd "$here/dist" && grep "  $file\$" SHA256SUMS | sha256sum -c -)
# Shared storage may be noexec on Android: test a private staged copy.
umask 077
base="$HOME/.local/lib/$package"
launcher="$HOME/.local/bin/$name"
shortcut="$HOME/.shortcuts/$widget"
for target in "$launcher" "$shortcut"; do
 if [[ -e "$target" || -L "$target" ]]; then
  [[ -f "$target" && ! -L "$target" ]] && grep -qx "# $marker" "$target" || { echo "既存ファイルを保護して中止: $target" >&2; exit 1; }
 fi
done
mkdir -p -- "$base"
stage=$(mktemp -d "$base/.staging.XXXXXXXX")
backup=''; changed=0; had_launcher=0; had_widget=0
cleanup() {
 code=$?
 if [[ $code != 0 && $changed == 1 ]]; then
  if [[ $had_launcher == 1 ]]; then cp -p -- "$backup/launcher" "$launcher"; else rm -f -- "$launcher"; fi
  if [[ $had_widget == 1 ]]; then cp -p -- "$backup/widget" "$shortcut"; else rm -f -- "$shortcut"; fi
  echo '切替失敗。前の入口へ復元しました。' >&2
 fi
 rm -rf -- "$stage"
 exit "$code"
}
trap cleanup EXIT
cp -- "$here/dist/$file" "$stage/$binary"
chmod 700 "$stage/$binary"
hash=$(sha256sum "$here/dist/$file"); hash=${hash%% *}
actual=$(sha256sum "$stage/$binary"); actual=${actual%% *}
[[ "$actual" == "$hash" ]]
"$stage/$binary" version
root="$base/0.2.0-${hash:0:16}"
if [[ -e "$root" || -L "$root" ]]; then
 [[ -d "$root" && ! -L "$root" && -f "$root/$binary" && ! -L "$root/$binary" ]] || exit 1
 actual=$(sha256sum "$root/$binary"); [[ "${actual%% *}" == "$hash" ]] || { echo '既存版ディレクトリの衝突。中止。' >&2; exit 1; }
fi
printf 'Codex PhoneOps 追加・更新先:\n%s\n%s\n%s\n' "$root" "$launcher" "$shortcut"
printf '設定・SSH鍵を変更せず、旧入口をbackupします。続けますか？ [y/N]: '
read -r answer
[[ "$answer" == y || "$answer" == Y ]] || exit 0
mkdir -p -- "$base/backups" "$(dirname "$launcher")" "$(dirname "$shortcut")"
backup=$(mktemp -d "$base/backups/update.XXXXXXXX")
if [[ -f "$launcher" ]]; then cp -p -- "$launcher" "$backup/launcher"; had_launcher=1; fi
if [[ -f "$shortcut" ]]; then cp -p -- "$shortcut" "$backup/widget"; had_widget=1; fi
# Settings are not overwritten. Preserve snapshots for a manual rollback audit.
for target in ubuntu win11; do
 if [[ -d "$HOME/.config/$package/$target" ]]; then cp -Rp -- "$HOME/.config/$package/$target" "$backup/config-$target"; fi
 if [[ -d "$HOME/.local/state/$package/$target" ]]; then cp -Rp -- "$HOME/.local/state/$package/$target" "$backup/state-$target"; fi
done
if [[ ! -d "$root" ]]; then mkdir -- "$root"; cp -p -- "$stage/$binary" "$root/$binary"; fi
shell=$(command -v bash)
printf '#!%s\n# %s\nexec "$HOME/.local/lib/%s/%s/%s" "$@"\n' "$shell" "$marker" "$package" "$(basename "$root")" "$binary" > "$stage/launcher"
printf '#!%s\n# %s\nexec "$HOME/.local/bin/%s"\n' "$shell" "$marker" "$name" > "$stage/widget"
chmod 700 "$stage/launcher" "$stage/widget"
# Same filesystem under HOME. All preparation precedes the two atomic switches.
changed=1
mv -f -- "$stage/launcher" "$launcher"
mv -f -- "$stage/widget" "$shortcut"
changed=0
printf '配置しました。backup: %s\n' "$backup"
printf '戻す場合はbackupのlauncher/widgetのみ元の表示先へコピーします。設定・復旧メモは巻き戻しません。\n'
printf '起動: %s\nTermux:WidgetをREFRESHしてください。\n' "$launcher"
