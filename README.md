# Codex PhoneOps

Android-first operations and recovery toolkit
for Codex CLI and ChatGPT Remote workflows.

Unofficial project. Not affiliated with or endorsed by OpenAI.

Android / TermuxからUbuntuのCodex CLIを操作し、Windows 11のChatGPT Remoteへ戻るための個人向けツールです。SSH接続、純正CLIへの移動、RDPによる手動復旧の手順を一つの入口にまとめています。スマートフォンで次に確認することが分かるように作りました。

製品名はCodex PhoneOps、コマンド名はその頭文字の `cpo` です。

既存のSSH・RDP接続を自分で管理できる利用者が対象です。Codex CLIやChatGPTの代替クライアントではありません。

- **Ubuntu Codex CLI**：既存作業の一覧・resume・明示した新規開始、確認付きdaemon start、直接SSH、純正doctor、Cloudへの入口。
- **Win11 ChatGPT Remote recovery**：対象登録、固定の読取り診断、RDP接続情報・手順、Android Remoteでの本人確認結果の保存。
- **SSH / RDP / Remote**：SSHは診断とCLI操作、RDPはWindowsのGUI確認・手動準備、Remoteは純正ChatGPTで目的の作業を開く経路です。WindowsへはAndroidから直接SSHし、Ubuntuを中継しません。

## 現在の状態

バージョンは **0.2.0・正式Release前の配布候補**です。Linuxでのローカル試験と静的ビルドが対象です。Android実機、Windows実機、実ChatGPT Remoteの受入は完了していません。GitHub Actionsは定義済みですが、ローカル検証とGitHub上の実行成功は別です。

Repository / Go module: `github.com/tsutomu-n/codex-phone-ops`

ライセンス: [MIT](LICENSE)。source archiveとbinary archiveの両方に同梱します。

この文書の `<repo-root>` は、利用者が配置したリポジトリのルートを表します。配置先は任意です。

## Installation — 導入

現在はソースから配布候補を作成します。公開済みreleaseとダウンロードURLはまだありません。ビルドにはGo 1.23以上が必要です。Ubuntu連携には、接続先にCodex CLI 0.155以上が必要です。

```bash
# cloneしたrepositoryのrootで実行
bash scripts/package.sh
```

生成物は `dist/release/codex-phone-ops-0.2.0-linux.tar.gz` と `dist/release/codex-phone-ops-0.2.0-source.tar.gz`、検証用の `SHA256SUMS.txt` です。Git管理ファイルの現在の内容からsource archiveを作り、そのsourceからbinaryをbuildします。配布フォルダーに別のファイルがあればpackageは停止します。Linux用archiveにはarm64 / amd64のbinaryを同梱します。Termux向けの候補であり、Android実機での動作を保証するものではありません。配布前に、信頼できる経路で得たchecksumと照合してください。

Termuxには、既存のbash・OpenSSH・tar・sha256sumが必要です。配布候補を検証・展開した後、展開先で以下を実行します。installerはネットワークを使わず、配置先を表示して確認を求めます。

```bash
bash install.sh
"$HOME/.local/bin/cpo" version
"$HOME/.local/bin/cpo" card
```

`$HOME/.local/bin` がPATHに入っていれば `cpo` で起動できます。installerはTermux:Widget用の「Codex PhoneOps」を作成します。REFRESH後に利用してください。初回登録では接続先を確認し、`cpo ubuntu setup` または `cpo win11 setup` を実行します。Windows登録では復旧したい作業名も指定します。setupでは対象へのSSH診断を行います。

旧版の管理下にある `phoneops` launcherは、installerがbackup後に取り外します。`phoneops` の互換コマンドは提供しません。設定と操作記録の保存先は変わりません。

詳しい導入・更新・旧設定移行は [利用手順](USER_MANUAL.md)、実行不能時は [手動復旧カード](RECOVERY_CARD.md) を参照してください。

## CLI

| コマンド | 動作 |
|---|---|
| `cpo` | Ubuntu / Windows / 手動カードを選ぶトップメニュー |
| `cpo ubuntu` | Ubuntu TUI。doctorは `d` キー |
| `cpo ubuntu setup` | Ubuntu登録 |
| `cpo ubuntu cloud` | Termuxで公式Cloud画面を開く |
| `cpo win11` | Windows復旧メニュー |
| `cpo win11 setup` | Windows登録 |
| `cpo win11 check` | 登録先への読取り診断。`--json` / `--certificate` を選択可能 |
| `cpo card` | 設定・通信不要の手動カード |
| `cpo version` / `cpo help` | 製品版・使い方 |

各入口の `-h` でオプションを確認できます。Ubuntu / Win11には `--readonly` があります。単独の `cpo doctor` は追加していません。

Windows診断は最大12秒で1回実行します。対象ChatGPTの起動は、登録したSID・セッション・AppIDとプロセスの生成時刻を照合できた場合だけ表示します。取得不能はUNKNOWNとし、最終応答を受け取っても終了待ちでtimeout・取消・実行失敗した場合は、応答時点の観測と実行エラーを併記して終了コードを失敗にします。診断時刻・前回観測・本人確認時刻は地域名とUTC offset付きで表示し、保存値はUTCです。Androidでは端末の地域設定を読み、取得できなければUTC fallbackを明示します。

未登録なら `cpo win11 setup` を実行してください。SSH認証失敗時は鍵未ロードと決めつけず、実行端末で `ssh -G <登録したSSH alias>` の `identityagent` を確認し、そのagentと鍵・接続ユーザーを調べてください。鍵・SSH設定を自動変更しません。

## 保存先

実行端末のHOMEを基準にします。従来の方式を保ち、XDG環境変数による変更は行いません。設定・状態は `--config-dir` / `--state-dir` で指定できます。

- 設定：`$HOME/.config/codex-phone-ops/ubuntu/`、`$HOME/.config/codex-phone-ops/win11/`
- 操作記録・復旧メモ：`$HOME/.local/state/codex-phone-ops/ubuntu/`、`$HOME/.local/state/codex-phone-ops/win11/`
- 実行ファイル・更新backup：`$HOME/.local/lib/codex-phone-ops/`
- launcher：`$HOME/.local/bin/cpo`、Widget：`$HOME/.shortcuts/Codex PhoneOps`
- cache：現在は使用・作成しません。

旧データは自動移動・削除しません。旧設定を利用していた場合は利用手順の移行節を先に読んでください。

## Security model — 安全性

既存SSH設定と厳格なホスト鍵確認を使い、登録した設定のhashと対象identityを照合します。鍵・パスワード・PIN・ペアリングコードは生成・保存しません。権限不足や解析失敗はUNKNOWNとして扱い、プロセス存在からRemote利用可能とは断定しません。

Ubuntuのdaemon startは確認後に操作記録を保存して実行し、応答喪失時に自動再送しません。Windowsの診断は固定PowerShellスクリプトで、再起動・kill・再ペアリング・サービス修復を自動実行しません。直接SSHや純正CLIへ移動した後の手入力は、本ツールの制御外です。

installerはchecksum・version・既存ファイルの所有markerを確認し、通常の切替失敗をrollbackします。配布物は無署名であり、同梱checksumだけで配布者の真正性を証明するものではありません。

## Limitations — 制約

- Windows設定はSSHユーザーと対象GUIユーザーが同じことを前提とします。
- WTS / AUMID等を照合できない環境はUNKNOWNです。RDP待受は経路・認証成功の保証ではありません。
- Remote内部の接続状態や会話は取得しません。アカウント・workspace・目的の作業は純正画面で本人確認します。
- 手動カードは登録済みの接続先と目的を自動表示しません。本人が保存済み情報と照合してください。
- Linux / Termux向けCLIです。WindowsネイティブCLIは配布しません。
- 電源断を含むinstaller全体の完全なtransaction、端末間の排他は保証しません。

## 検証

```bash
go test ./...
go test -race ./...
go vet ./...
bash scripts/build.sh
python3 scripts/test_pty.py
python3 scripts/test_install.py
python3 scripts/test_win_ui.py
python3 scripts/test_cli.py
python3 scripts/test_probe_contract.py  # Linux pwshが必要
git diff --check
```

PTY試験はfixture Codex、Windows画面試験はfake SSHを使います。実端末受入の代替ではありません。

## Roadmap — 今後の確認

未実施の受入として、Android / Termux / Widget実機、隔離WindowsでのPowerShell / OpenSSH、本人によるRemote復帰とRDP切断後の再接続確認を予定しています。公開配布は、これらの受入と公開内容を確認してから判断します。自動修復とRemote代替クライアントは実装済み機能に含まれません。
