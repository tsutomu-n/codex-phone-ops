package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type App struct {
	R        Remote
	Dir      string
	T        *Terminal
	Readonly bool
	External atomic.Bool
	Notice   string
	Fatal    error
}

func (a *App) message(title, text string) {
	w, h := a.T.Size()
	lines := Wrap(text, w)
	offset := 0
	for {
		bottom := offset + h - 4
		if bottom > len(lines) {
			bottom = len(lines)
		}
		a.T.Paint(title, lines[offset:bottom], "↑↓ スクロール  Enter / Esc 戻る")
		k, e := a.T.Key()
		if e != nil || k == "enter" || k == "esc" || k == "q" {
			return
		}
		if k == "down" && offset+h-4 < len(lines) {
			offset++
		}
		if k == "up" && offset > 0 {
			offset--
		}
	}
}
func (a *App) confirm(title, text string) bool {
	w, _ := a.T.Size()
	lines := Wrap(text, w)
	a.T.Paint(title, lines, "y 実行する   n / Esc やめる")
	for {
		k, e := a.T.Key()
		if e != nil {
			return false
		}
		if k == "y" {
			return true
		}
		if k == "n" || k == "esc" || k == "q" {
			return false
		}
	}
}
func (a *App) busy(s string) {
	a.T.Paint("Codex PhoneOps / Ubuntu", Wrap(s, 38), "確認中。変更操作はまだ行いません")
}
func (a *App) verified() (Info, error) {
	a.busy("接続先とCodexを確認しています…")
	ctx, cancel := CheckContext()
	defer cancel()
	return a.R.Verify(ctx)
}
func (a *App) Run() error {
	if e := a.T.Enter(); e != nil {
		return e
	}
	defer a.T.Leave()
	selected := 0
	for {
		if a.Fatal != nil {
			return a.Fatal
		}
		lines := []string{"Ubuntuで仕事を始める・続ける", "", "1  実行中の作業を純正一覧で選ぶ", "2  保存した作業を純正一覧で選ぶ", "3  プロジェクトを選んで新規開始", "", "4  Ubuntuへ直接SSH", "5  公式Codex Cloudを開く", "", "r  接続・共有サービスを確認", "d  Codexの診断を開く", "", "一覧の選択・承認は純正Codexで行います。"}
		if a.Readonly {
			lines[0] = "読取り専用：起動・画面移動は無効"
		}
		if a.Notice != "" {
			lines = append(lines, "", a.Notice)
		}
		for i, line := range lines {
			if strings.HasPrefix(line, fmt.Sprintf("%d  ", selected+1)) {
				lines[i] = "> " + line
			}
		}
		a.T.Paint("Codex PhoneOps / "+a.R.C.Label, lines, "↑↓ Enter 開く / ? ヘルプ / q 終了")
		k, e := a.T.Key()
		if e != nil {
			return e
		}
		if k == "up" || k == "k" {
			if selected > 0 {
				selected--
			}
			continue
		}
		if k == "down" || k == "j" {
			if selected < 4 {
				selected++
			}
			continue
		}
		if k == "enter" {
			k = fmt.Sprintf("%d", selected+1)
		}
		switch k {
		case "q", "esc":
			return nil
		case "?":
			a.message("使い方", helpText)
		case "r":
			a.inspect()
		case "1", "c":
			a.native("agents", "")
		case "2":
			a.native("resume", "")
		case "3", "n":
			a.projects()
		case "4", "s":
			a.shell()
		case "5":
			a.cloud()
		case "d":
			a.doctor()
		}
	}
}
func (a *App) inspect() {
	i, e := a.verified()
	if e != nil {
		a.Notice = "接続確認できません"
		a.message("確認できません", e.Error())
		return
	}
	ctx, cancel := CheckContext()
	defer cancel()
	d, e := a.R.Daemon(ctx)
	text := fmt.Sprintf("SSH接続先: %s\nユーザー: %s\nCodex: %s\n作業ルート: %s\n\n", a.R.C.Host, i.Identity.User, i.Version, a.R.C.ProjectsRoot)
	if e != nil {
		text += "共有サービス: 確認不能\n" + e.Error()
	} else if d.Status == "running" {
		text += "共有サービス: 起動中\n作業が実行中かどうかは純正一覧で確認します。"
	} else {
		text += "共有サービス: 未起動\n新規開始時に確認して起動できます。\n別のCodexまで停止中とは判断していません。"
	}
	a.Notice = "接続確認 " + time.Now().Format("15:04:05")
	a.message("Ubuntuの接続確認", text)
}
func (a *App) projects() {
	if a.Readonly {
		a.message("読取り専用", "新規開始は無効です。")
		return
	}
	if _, e := a.verified(); e != nil {
		a.message("接続できません", e.Error())
		return
	}
	ctx, cancel := CheckContext()
	p, e := a.R.Projects(ctx)
	cancel()
	if e != nil {
		a.message("フォルダーを読めません", e.Error())
		return
	}
	if len(p) == 0 {
		a.message("作業フォルダーがありません", a.R.C.ProjectsRoot+" の直下にフォルダーがありません。\nsetupで別のルートを登録できます。\nフォルダーやRepositoryは自動作成しません。")
		return
	}
	search := ""
	sel := 0
	for {
		filtered := []string{}
		for _, s := range p {
			if strings.Contains(strings.ToLower(filepath.Base(s)), strings.ToLower(search)) {
				filtered = append(filtered, s)
			}
		}
		if sel >= len(filtered) {
			sel = 0
		}
		_, h := a.T.Size()
		n := h - 7
		if n < 1 {
			n = 1
		}
		start := 0
		if sel >= n {
			start = sel - n + 1
		}
		lines := []string{"新規作業のフォルダーを選択", "検索: " + Safe(search), ""}
		for j := start; j < len(filtered) && j < start+n; j++ {
			prefix := "  "
			if j == sel {
				prefix = "> "
			}
			lines = append(lines, prefix+filepath.Base(filtered[j]))
		}
		if len(filtered) == 0 {
			lines = append(lines, "該当するフォルダーはありません")
		}
		a.T.Paint("新しい作業", lines, "↑↓ 選択  Enter 開く  / 検索  Esc 戻る")
		k, e := a.T.Key()
		if e != nil || k == "esc" || k == "q" {
			return
		}
		switch k {
		case "up", "k":
			if sel > 0 {
				sel--
			}
		case "down", "j":
			if sel+1 < len(filtered) {
				sel++
			}
		case "pageup":
			sel -= n
			if sel < 0 {
				sel = 0
			}
		case "pagedown":
			sel += n
			if sel >= len(filtered) {
				sel = len(filtered) - 1
			}
		case "/":
			a.T.Leave()
			fmt.Print("プロジェクト名の一部（空で解除）: ")
			s, inputErr := bufio.NewReader(os.Stdin).ReadString('\n')
			if inputErr != nil {
				a.Fatal = inputErr
				return
			}
			search = strings.TrimSpace(s)
			sel = 0
			a.restore()
			if a.Fatal != nil {
				return
			}
		case "enter":
			if len(filtered) > 0 {
				cwd := filtered[sel]
				if a.confirm("新しいCodex作業", "実行先: "+a.R.C.Label+"\nフォルダー:\n"+cwd+"\n\n新しい会話を開始します。既存作業の再開は『保存した作業』を使ってください。\n指示文・承認は純正Codexで入力します。") {
					a.native("new", cwd)
				}
				return
			}
		}
	}
}

// Native never automatically switches from an existing work to a new work.
// A missing daemon is only started after an explicit user confirmation.
func (a *App) native(mode, cwd string) {
	if a.Readonly {
		a.message("読取り専用", "純正Codexは変更操作が可能なため、このモードでは開きません。")
		return
	}
	if _, e := a.verified(); e != nil {
		a.message("開けません", e.Error())
		return
	}
	ctx, cancel := CheckContext()
	d, e := a.R.Daemon(ctx)
	cancel()
	if e != nil {
		a.message("共有状態を確認できません", e.Error()+"\n\n自動起動や別環境への切替は行いません。必要ならメニューの『直接SSH』を使ってください。")
		return
	}
	if d.Status == "notRunning" {
		n, e := UnknownOperations(a.Dir, a.R.C.Host)
		if e != nil {
			a.message("操作記録を確認してください", e.Error())
			return
		}
		text := "UbuntuのCodex共有サービスを起動します。\n\n起動すると、このSSH接続とは別のサービスが動きます。\n保存済みのRemote・自動更新設定も使われます。更新処理が後で作業を中断する可能性があります。設定は変更しません。"
		if n > 0 {
			text = fmt.Sprintf("過去の起動要求 %d 件の結果が不明です。現在は未起動と観測しました。\nこれは再送ではなく、新しい起動要求になります。\n\n", n) + text
		}
		if !a.confirm("共有サービスを起動しますか", text) {
			return
		}
		o, e := BeginOperation(a.Dir, a.R.C.Host)
		if e != nil {
			a.message("起動しません", "操作記録を保存できないため送信しません。\n"+e.Error())
			return
		}
		a.T.Paint("Codex PhoneOps / Ubuntu", []string{"共有サービスへ起動を要求中…"}, "送信中。応答喪失時は結果不明・自動再送なし")
		ctx, cancel = context.WithTimeout(context.Background(), 25*time.Second)
		b, sendErr := a.R.CodexRead(ctx, "app-server", "daemon", "start")
		cancel()
		var reply Daemon
		parseErr := json.Unmarshal(b, &reply)
		accepted := sendErr == nil && parseErr == nil && (reply.Status == "started" || reply.Status == "alreadyRunning")
		outcome := "unknown"
		if accepted {
			outcome = "accepted"
		}
		responseStatus := ""
		if accepted {
			responseStatus = reply.Status
		}
		if e = FinishOperation(a.Dir, o, outcome, responseStatus); e != nil {
			a.message("起動要求の結果を記録できません", "再送せず終了します。次に接続状態を確認してください。")
			return
		}
		if !accepted {
			a.message("起動結果を確認できません", "同じ要求は再送していません。\nメニューの接続確認、または直接SSHで現在の状態を確認してください。")
			return
		}
		ctx, cancel = CheckContext()
		d, e = a.R.Daemon(ctx)
		cancel()
		if e != nil || d.Status != "running" {
			a.message("起動要求は受理されました", "ただし、現在利用できることは確認できません。再起動せずメニューへ戻ります。")
			return
		}
	}
	if d.AppVersion != nil && !VersionOK(*d.AppVersion) {
		a.message("共有サービスの版が古いか不明です", "稼働中: "+Safe(*d.AppVersion)+"\n自動更新・再起動はしません。既存の作業が終了してから環境を整えてください。")
		return
	}
	endpoint := "unix://" + d.Socket
	var args []string
	switch mode {
	case "agents":
		args = []string{"agents", "--remote", endpoint, "--no-alt-screen"}
	case "resume":
		args = []string{"resume", "--all", "--remote", endpoint, "--no-alt-screen"}
	case "new":
		args = []string{"--remote", endpoint, "--no-alt-screen", "-C", cwd}
	default:
		a.message("操作未対応", mode)
		return
	}
	a.handoff(a.R.Native(cwd, args...), "純正Codexへ移ります。作業・会話は純正側で選んでください。\n接続だけ切る場合は Enter → ~ → . を順に押します（SSHの切断）。\nCtrl+CはCodexの処理を中断する場合があります。\nホスト・共有サービスが止まれば継続できません。")
}
func (a *App) handoff(cmd *exec.Cmd, note string) {
	a.T.Leave()
	fmt.Println("\n" + note + "\n")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	a.External.Store(true)
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGHUP)
	e := cmd.Start()
	interrupted := false
	if e == nil {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case e = <-done:
		case s := <-signals:
			interrupted = true
			_ = cmd.Process.Signal(s)
			select {
			case e = <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				e = <-done
			}
		}
	}
	signal.Stop(signals)
	a.External.Store(false)
	if restoreErr := a.T.Restore(); restoreErr != nil {
		a.Fatal = restoreErr
		return
	}
	if interrupted {
		a.Fatal = fmt.Errorf("終了シグナルを受けたため、子画面を閉じました")
		return
	}
	fmt.Println("\nCodex作業の入口へ戻ります。子画面の終了は作業完了を意味しません。")
	if e != nil {
		fmt.Println("接続・子画面の終了:", e)
	}
	fmt.Print("Enterで戻る: ")
	_, inputErr := bufio.NewReader(os.Stdin).ReadString('\n')
	if inputErr != nil {
		a.Fatal = inputErr
		return
	}
	a.restore()
	a.Notice = "純正画面から戻りました（作業状態未確認）"
}
func (a *App) restore() {
	if e := a.T.Enter(); e != nil {
		a.T.Leave()
		a.Fatal = fmt.Errorf("端末の再取得に失敗しました。再送せず終了します: %w", e)
	}
}

func (a *App) shell() {
	if a.Readonly {
		a.message("読取り専用", "SSHシェルは変更可能なので開きません。")
		return
	}
	if !a.R.Local {
		if e := a.R.C.TrustConfig(); e != nil {
			a.message("設定確認が必要", e.Error())
			return
		}
	}
	cmd := a.R.Command(nil, true, "")
	if a.R.Local {
		cmd = exec.Command("sh")
	}
	a.handoff(cmd, "Ubuntuへ直接入ります。ここからの操作は本ツールの安全制御の外です。")
}
func (a *App) doctor() {
	if a.Readonly {
		a.message("読取り専用", "純正doctorはread-mostlyで、内部記録を行う場合があります。今回は起動しません。")
		return
	}
	if _, e := a.verified(); e != nil {
		a.message("診断できません", e.Error())
		return
	}
	a.handoff(a.R.Native("", "doctor", "--summary"), "純正Codex doctorを実行します。問題を自動修復する機能ではありません。")
}
func (a *App) cloud() {
	if a.Readonly {
		a.message("読取り専用", "外部画面への移動は無効です。")
		return
	}
	cmd := exec.Command("termux-open-url", "https://chatgpt.com/codex/")
	if _, e := exec.LookPath("termux-open-url"); e != nil {
		a.message("公式Cloud", "ブラウザーで https://chatgpt.com/codex/ を開いてください。\nこのツールからタスクを送信してはいません。")
		return
	}
	if e := cmd.Run(); e != nil {
		a.message("ブラウザーを開けません", e.Error())
		return
	}
	a.Notice = "Cloudを開きました。送信・Repo選択は公式画面で。"
}

const helpText = `これはUbuntu用の「入口版」です。

1: 純正agentsで、共有環境の作業を選びます。
2: 純正resumeで保存された会話を選びます。最新の会話へ勝手に入りません。
3: 登録ルート直下のフォルダーを選び、新しい会話を開始します。
4: Remoteアプリや本ツールの状態確認が使えない時の直接SSHです。
5: PC不要の仕事は公式Cloudで開始できます。

Codexが未起動なら、確認して共有サービスを起動します。停止・再起動・自動更新設定変更は実装していません。

SSH切断とCodex停止は別です。純正画面中はEnter→~→.でSSH接続だけ閉じられます。共有サービス・Ubuntu・回線まで止まる場合の継続は保証できません。

Ctrl+Cは純正画面内ではCodexを中断する場合があります。q/Escは本ツール内でだけ戻る・終了です。

実行中の作業をこのツールから自動再開・複製・承認することはありません。独自thread一覧はこの版にはありません。`

func Setup(dir, host, sshConfig, root, codex string, replace, local bool) error {
	if !aliasRE.MatchString(host) {
		return errors.New("SSH aliasが不正です")
	}
	path := filepath.Join(dir, "config.json")
	if _, e := os.Lstat(path); e == nil && !replace {
		return errors.New("設定は既にあります。変更する場合は setup --replace を明示してください")
	}
	sshConfig = Expand(sshConfig)
	hash := "local-test-only"
	if !local {
		var e error
		hash, e = HashFile(sshConfig)
		if e != nil {
			return fmt.Errorf("SSH設定を読めません: %w", e)
		}
	}
	c := Config{Schema: 1, Label: "Ubuntu VM", Host: host, SSHConfig: sshConfig, SSHConfigHash: hash}
	fmt.Printf("%s へ読み取り接続します。既存のホスト鍵とSSH設定を使います。\n", host)
	ctx, cancel := CheckContext()
	i, e := (Remote{C: c, Local: local}).Discover(ctx, codex)
	cancel()
	if e != nil {
		return e
	}
	if root == "" {
		root = filepath.Join(i.Identity.Home, "projects")
	}
	if !ValidAbs(root) {
		return errors.New("projects-rootにはUbuntu側の絶対パスが必要です")
	}
	c.Identity = i.Identity
	c.Codex = i.Codex
	c.ProjectsRoot = root
	fmt.Printf("\n登録先: %s\nユーザー: %s (uid %s)\nCodex: %s\nCodex HOME: %s\n作業ルート: %s\nCLI: %s\n\n", Safe(host), Safe(i.Identity.User), Safe(i.Identity.UID), Safe(c.Codex), Safe(i.Identity.CodexHome), Safe(root), Safe(i.Version))
	fmt.Print("この接続先をAndroid端末側に登録しますか？ [y/N]: ")
	input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(strings.ToLower(input)) != "y" {
		return errors.New("登録を中止しました。設定は変更していません")
	}
	if replace {
		if b, e := os.ReadFile(path); e == nil {
			if e = AtomicWrite(path+".backup-"+time.Now().Format("20060102-150405"), b); e != nil {
				return e
			}
		}
	}
	if e = SaveConfig(dir, c); e != nil {
		return e
	}
	fmt.Println("登録しました。サービス起動・Codexの作業開始はまだ行っていません。")
	return nil
}
