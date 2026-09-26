# Codex PhoneOps — WindowsのChatGPT Remoteへ戻る：オフライン手動カード

このカードは実行スクリプトではありません。本人の最新の保存済み接続情報を使用してください。パスワード・PIN・ペアリングコードを書き込まないでください。

1. Android端末で既存Tailscaleの利用状態と対象Windowsを確認する。
2. 保存済みRDP接続を開く。接続先名・証明書に変更警告があれば先に照合する。
3. 対象Windowsユーザーでログインする。他ユーザーの切断警告が出たら、無断で先へ進めない。
4. 対象のChatGPTを確認する。ChatGPT Classicと取り違えない。起動中なら終了させない。
5. 純正画面でアカウント・workspace・Remote接続許可を本人が確認する。
6. Android ChatGPT → Remote → 登録したWindows → 目的の作業を開く。
7. RDPは接続を切断する。Windowsからサインアウトしない。その後、Remoteへ再接続できることを確認する。
8. Termuxへ戻ったら再確認し、本人による確認結果を記録する。

SSHが使えなくてもRDP手順は残ります。ただし、ホスト鍵・証明書・接続先の不一致を迂回して無視しないでください。照会の権限不足は「ユーザーなし」「未インストール」「SSH認証失敗」を意味しません。

SSHで手動確認する場合、既定shellがcmdなら先に `powershell.exe -NoLogo -NoProfile` を開いてからPowerShellコマンドを入力します。

接続できるが作業がない時は、純正側でアカウント・workspace・プロジェクト・会話の選択を確認します。新しいCodexを開始しても元の会話への復帰にはなりません。

無断再起動・サインアウト・一括kill・Git reset/pull・Tailscale再認証・Firewall/NLA変更・Remote再ペアリングはしません。目的の作業が開けたら終了です。
