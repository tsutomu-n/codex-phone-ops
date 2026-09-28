package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/tsutomu-n/codex-phone-ops/internal/control"
	"github.com/tsutomu-n/codex-phone-ops/internal/winremote"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	_ "time/tzdata"
)

func main() {
	if e := run(); e != nil {
		var printed *winremote.PrintedError
		if !errors.As(e, &printed) { fmt.Fprintln(os.Stderr, "\n", e) }
		os.Exit(1)
	}
}
func run() error {
	args := os.Args[1:]
	if len(args) == 0 {
		return topMenu()
	}
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			fmt.Println(`Codex PhoneOps — Android / Termuxからの操作・復旧入口
使い方: cpo [ubuntu|win11|card|version|help]
  引数なし                     Ubuntu / Windows / 手動カードを選択
  ubuntu [setup|cloud] [flags]  Ubuntu TUI
  win11 [setup|check] [flags]   Windows Remote復旧
  card                         オフライン手動復旧カード
  version                      製品バージョン
UbuntuのdoctorはTUIの d キー。各入口の -h で設定オプションを表示。
Unofficial project. Not affiliated with or endorsed by OpenAI.`)
			return nil
		case "win11":
			return winremote.Run(args[1:])
		case "card":
			return winremote.Run(args)
		case "version":
			fmt.Println("Codex PhoneOps", control.Version)
			return nil
		case "ubuntu":
			args = args[1:]
		}
	}
	return runUbuntu(args)
}

func topMenu() error {
	t, e := control.NewTerminal()
	if e != nil {
		return e
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	if e = t.Enter(); e != nil {
		signal.Stop(signals)
		return e
	}
	done := make(chan struct{})
	go func() {
		select {
		case s := <-signals:
			t.Leave()
			os.Exit(128 + int(s.(syscall.Signal)))
		case <-done:
		}
	}()
	leave := func() {
		signal.Stop(signals)
		close(done)
		t.Leave()
	}
	for {
		t.Paint("Codex PhoneOps", []string{"接続先を選んでください", "", "1  Ubuntu / Codex CLI", "2  Windows / Remote Recovery", "3  Manual Recovery Card"}, "1–3 選択   q 終了")
		key, keyErr := t.Key()
		if keyErr != nil {
			leave()
			return keyErr
		}
		switch key {
		case "1", "2", "3":
			leave()
			switch key {
			case "1":
				return runUbuntu(nil)
			case "2":
				return winremote.Run(nil)
			default:
				return winremote.Run([]string{"card"})
			}
		case "q", "esc":
			leave()
			return nil
		}
	}
}

func runUbuntu(args []string) error {
	cmd := ""
	if len(args) > 0 && (args[0] == "setup" || args[0] == "version" || args[0] == "cloud") {
		cmd = args[0]
		args = args[1:]
	}
	if cmd == "cloud" {
		if len(args) > 0 {
			if len(args) == 1 && args[0] == "--readonly" {
				return fmt.Errorf("読取り専用では外部画面を開きません")
			}
			return fmt.Errorf("不明な引数です: %v", args)
		}
		return exec.Command("termux-open-url", "https://chatgpt.com/codex/").Run()
	}
	if cmd == "version" {
		fmt.Println("Codex PhoneOps", control.Version)
		return nil
	}
	fs := flag.NewFlagSet("cpo ubuntu", flag.ContinueOnError)
	dir := fs.String("config-dir", control.DefaultDir(), "本ツール専用の設定フォルダー")
	state := fs.String("state-dir", control.StateDir(), "操作記録フォルダー")
	ro := fs.Bool("readonly", false, "読取り専用（純正への移動・サービス起動を禁止）")
	local := fs.Bool("local", false, "Ubuntu上でSSHを経由せず使う（開発・自己検証用）")
	host := fs.String("host", "ubuntu", "既存SSH alias。setup用")
	h, _ := os.UserHomeDir()
	sshConfig := filepath.Join(h, ".ssh", "config")
	sshFile := fs.String("ssh-config", sshConfig, "既存のSSH設定。setup用")
	root := fs.String("projects-root", "", "Ubuntu側の作業ルート。既定は確認したHOME/projects")
	codex := fs.String("codex-path", "", "Ubuntu側Codexの絶対パス。setup時の自動検出を上書き")
	replace := fs.Bool("replace", false, "setupで登録情報を置き換える（旧設定は保存）")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("不明な引数です: %v", fs.Args())
	}
	unlock, e := control.Lock(control.Expand(*dir))
	if e != nil {
		return e
	}
	defer unlock()
	if cmd == "setup" {
		if *ro {
			return fmt.Errorf("読取り専用ではsetupしません")
		}
		return control.Setup(control.Expand(*dir), *host, *sshFile, *root, *codex, *replace, *local)
	}
	c, e := control.Load(control.Expand(*dir))
	if e != nil {
		return fmt.Errorf("登録情報を開けません: %w\n最初に cpo ubuntu setup を実行してください。", e)
	}
	t, e := control.NewTerminal()
	if e != nil {
		return e
	}
	a := &control.App{R: control.Remote{C: c, Local: *local}, Dir: control.Expand(*state), T: t, Readonly: *ro}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	go func() {
		for range signals {
			if a.External.Load() {
				continue
			}
			t.Leave()
			os.Exit(130)
		}
	}()
	return a.Run()
}
