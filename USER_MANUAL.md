# Codex PhoneOps 0.2.0 配布候補の使い方

コード・Ubuntu上のローカル試験まで完了した配布候補です。Android実機での実行、Windows実機への診断、ChatGPT Remoteは未確認です。Windows復旧はAndroid端末から対象へ直接SSH接続し、Ubuntuを中継しません。

## Android端末へ渡すもの

`codex-phone-ops-0.2.0-linux.tar.gz` と `SHA256SUMS.txt` を同じ場所へコピーします。アーカイブにはLinux arm64／amd64実行ファイル、共通installer、手動カード、利用手順を含みます。秘密鍵・パスワード・診断出力は含みません。ソースは別の `codex-phone-ops-0.2.0-source.tar.gz` です。

Termuxの既存bash／OpenSSH／tar／sha256sumを利用します。Go・Python・Nodeは利用時に不要です。Termuxのストレージ共有が未設定なら、利用者が共有設定を済ませてください。以下は共有Downloadsへコピーした場合です。`$HOME` は実際のTermux HOMEから解決します。

```bash
cd "$HOME/storage/downloads"
grep '  codex-phone-ops-0.2.0-linux.tar.gz$' SHA256SUMS.txt | sha256sum -c -
mkdir -p "$HOME/.local/share/codex-phone-ops/deliveries"
test ! -e "$HOME/.local/share/codex-phone-ops/deliveries/codex-phone-ops-0.2.0-linux" && \
  tar -xzf codex-phone-ops-0.2.0-linux.tar.gz -C "$HOME/.local/share/codex-phone-ops/deliveries"
cd "$HOME/.local/share/codex-phone-ops/deliveries/codex-phone-ops-0.2.0-linux"
bash install.sh
"$HOME/.local/bin/phoneops" version
"$HOME/.local/bin/phoneops" card
"$HOME/.local/bin/phoneops" win11 setup
"$HOME/.local/bin/phoneops" win11
```

同じ展開先が既にある場合は上書きせず、既存内容を確認してください。installerは個人領域に一時コピーしてhash・versionを確認し、実行不能なら入口を切り替えません。共有Downloads上のnoexecにも対応します。Linux arm64のビルド成功だけでAndroid動作保証はしません。

共通installerで単一CLI `phoneops` とWidget「Codex PhoneOps」を導入します。引数なしではUbuntu / Windows / 手動カードを選ぶトップメニューを開き、Windows復旧へ直接入る場合は `phoneops win11` です。Ubuntuの初回登録は `phoneops ubuntu setup`。UbuntuのSSH設定は既定で `$HOME/.ssh/config` を読み、別の場所なら `--ssh-config` で指定します。SSH agentを使う場合は起動環境で `SSH_AUTH_SOCK` を設定してください。旧版を利用していた場合は、末尾の移行手順を先に確認してください。

## 初回登録

`phoneops win11 setup` が現在のSSH alias、既存SSH設定の絶対パス、対象Windows名、`DOMAIN\user`、保存済みRDP名と接続先、復旧したい作業名を尋ねます。過去資料のIPやaliasを自動登録しません。既存鍵・known_hostsは読み取り利用し、鍵の登録・解除・ホスト鍵承認は代行しません。

対象を確認して読取り診断を許可すると、観測したユーザー・SID・ホストを表示します。ChatGPTとChatGPT Classicの候補はAppID付きで表示し、番号で明示選択します。0は保留です。候補が見つからなくても未インストールとは断定せず、アプリUNKNOWNの登録にできます。最後の本人確認でWindows専用設定を保存します。

アプリ更新やSSH設定の変更後は、内容を確認して `phoneops win11 setup --replace` で再登録します。旧設定は同じ専用ディレクトリへ時刻付きbackupとして保存します。設定保存前にもSSH設定のhashを再確認します。SSH接続ユーザーと対象GUIユーザーは同一を初期版の登録条件とします。

## 通常の復旧

各選択は数字を入力してEnter。40桁を基準に折り返すテキスト画面です。起動時の自動接続はありません。

1. 準備状態を確認。固定診断は1回、接続上限6秒・全体上限12秒。Ctrl+Cは待機取消です。遠隔処理が完全に取消された意味ではありません。
2. 次の一手を読む。GUIなしが完全な照会で確認できた時だけRDPログインを案内します。権限不足・解析失敗・照合不能はUNKNOWNです。
3. 必要な場合だけ保存済みRDPへ切り替え、対象ユーザーでログインして対象ChatGPTを確認。既存アプリ・作業を停止しません。
4. Android ChatGPT → Remote → 登録したWindows → 目的の作業へ戻ります。
5. RDPは切断し、Windowsからサインアウトしません。Remoteへ再接続できることを確認します。
6. Termuxへ戻ったら `r` で再確認。本人確認メニューで「対象Windowsへ接続」「目的の作業を開いた」「接続できるが作業がない」「まだ接続できない」を区別して記録します。

ChatGPTプロセスを確認してもRemote利用可能とは表示しません。接続できるが作業がない場合は、アカウント・workspace・プロジェクト・会話を純正側で確認します。

## 診断と逃げ道

`phoneops win11 check` は登録対象を一度だけ診断します。`phoneops win11 check --json` はSID、ユーザー、アプリID、IP、パス、生stderrを含めない要約です。`phoneops win11 check --certificate` は任意のRDP証明書SHA-1照合情報を表示します。証明書の読み取りやクライアント側での同種指紋表示ができなければUNKNOWNです。

RDP待受を確認してもAndroid端末側からの経路・認証成功ではありません。SSH到達失敗からWindows停止とは断定しません。Tailscaleはサービス観測のみで、tailnetの疎通・認証成功を保証しません。

直接SSHは変更可能な純正シェルです。ツール内の安全制御の外に移ることを表示します。Enter → `~` → `.` でSSH接続だけ切断できます。`--readonly` は診断・案内だけを許可し、設定・復旧メモ更新やSSHシェル起動を禁止します。

`phoneops card` は設定・lock・ネットワークなしで利用できます。本体も起動不能なら同梱の手動カードを読んでください。信頼異常はRDPへ迂回して無視してよい警告ではありません。

## 保存と更新・復元

Windows設定は `$HOME/.config/codex-phone-ops/win11/config.json`、復旧メモは `$HOME/.local/state/codex-phone-ops/win11/recovery.json`。新規ディレクトリ0700、ファイル0600で保存します。保存するのは対象・目的・最後の観測分類と時刻・次の一手・本人確認です。生の診断結果、会話、認証情報は保存しません。

観測は常に前回情報として表示し、2分以上経過・未来時刻・時刻なしならSTALEです。起動し直した時や他アプリから戻った時は `r` で再確認します。WindowsへのWRITE処理自体を実装していません。

installerは版とbinary hashごとの専用ディレクトリを作り、旧launcher／WidgetをWidget外のbackupへ保存して切り替えます。既存設定・復旧メモは書き換えず、更新時のsnapshotだけbackupに保存します。別製品のファイルやsymlinkが追加先にあれば中止します。

戻す時はinstallerが表示したbackupディレクトリの `launcher` を `$HOME/.local/bin/phoneops` へ、`widget` を `$HOME/.shortcuts/Codex PhoneOps` へコピーします。設定・復旧メモは巻き戻しません。初回追加でbackupの入口がない場合は旧版への復元対象がありません。削除は別途本人判断です。

Ubuntu設定は `$HOME/.config/codex-phone-ops/ubuntu/config.json`、操作記録は `$HOME/.local/state/codex-phone-ops/ubuntu/operations/` です。両入口の `--config-dir`、`--state-dir` で保存先を明示できます。従来どおりHOME基準で、XDG環境変数による上書きは実装していません。cacheは作成しません。

Windowsへの恒久配置・タスク登録・サービス変更はありません。再起動、sign-out、kill、daemon restart、Tailscale再認証、Firewall/RDP/Startup変更、Autologon、Proxmox、Git変更、新会話、Remote再ペアリングは自動実行しません。


## 旧名称からの移行（必要な場合だけ）

旧名称 `codex-control` / `codex-ubuntu` / `codex-win-remote` のaliasは作りません。旧installer・旧Widget・旧binaryを自動削除せず、新しい入口だけを導入します。まず旧入口を閉じ、実行中の操作がないことを確認してください。新旧の同時起動は別のlockになるため避けます。

設定ファイルの内容・SSH鍵・接続先は変更不要です。旧データをそのまま保持したまま使う場合は、次の明示指定ができます。Ubuntuでは旧操作記録も同じディレクトリにあるため、両方の引数が必要です。

```bash
phoneops ubuntu --config-dir "$HOME/.config/codex-control-ubuntu" --state-dir "$HOME/.config/codex-control-ubuntu"
phoneops win11 --config-dir "$HOME/.config/codex-win-remote" --state-dir "$HOME/.local/state/codex-win-remote"
```

新しい既定先へ移す場合は、旧ディレクトリをbackupし、新しい移行先が未作成であることを確認してから、以下の対応で**コピー**します。既存の新設定へ上書き・混合しません。個人領域内で `umask 077` を使用し、ディレクトリ0700・ファイル0600を維持します。

| 旧名称の保存場所（実行端末のHOME基準） | 新しい保存場所（同じHOME基準） |
|---|---|
| `$HOME/.config/codex-control-ubuntu/config.json` と設定backup | `$HOME/.config/codex-phone-ops/ubuntu/` |
| `$HOME/.config/codex-control-ubuntu/operations/` 全体 | `$HOME/.local/state/codex-phone-ops/ubuntu/operations/` |
| `$HOME/.config/codex-win-remote/config.json` と設定backup | `$HOME/.config/codex-phone-ops/win11/` |
| `$HOME/.local/state/codex-win-remote/recovery.json` | `$HOME/.local/state/codex-phone-ops/win11/recovery.json` |

`instance.lock` はコピー不要です。Ubuntuの `dispatching` / `unknown` 記録を省略しないでください。過去の起動要求の不確実性を新しい入口にも引き継ぐ必要があります。移行確認までは旧データ・入口を残します。新しいWidgetは既定先を使うため、明示指定での利用中はTermuxから上記コマンドを実行してください。
