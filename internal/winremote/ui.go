package winremote

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/tsutomu-n/codex-phone-ops/internal/control"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const Card = `Codex PhoneOps
WindowsのChatGPT Remoteへ戻る：手動復旧カード
1. 保存済み接続先・Tailscaleの利用状態を確認。
2. RDPの保存済み接続へ。名前・証明書の変更警告は先に照合。
3. 対象ユーザーでログイン。他ユーザーを切断しない。
4. ChatGPT（Classicと区別）を確認。既に起動中なら閉じない。
5. 純正アプリでアカウント・workspace・接続許可を確認。
6. Android ChatGPT → Remote → 登録したWindows → 目的の作業。
7. RDPは切断。Windowsからサインアウトしない。
Termuxへ戻ったら r で再確認。前回の観測は現在の保証ではありません。
Remote接続済みで作業がない時は、純正側でプロジェクト・会話を選択。
再起動・kill・Git変更・再ペアリング・Tailscale再認証は自動実行しません。
パスワード・PIN・ペアリングコードは純正画面だけに入力してください。
SSH失敗でもRDP手順は使えます。ただし信頼警告を迂回しないでください。`

func show(s string) {
	for _, l := range control.Wrap(s, 40) {
		fmt.Println(l)
	}
}
func line(reader *bufio.Reader, prompt string) (string, error) {
	show(prompt)
	s, e := reader.ReadString('\n')
	if e != nil {
		return "", e
	}
	return strings.TrimSpace(s), nil
}
func ask(reader *bufio.Reader, prompt string) bool {
	s, e := line(reader, prompt+" [y/N]")
	return e == nil && strings.EqualFold(s, "y")
}
func Run(args []string) error {
	cmd := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}
	if cmd == "version" {
		fmt.Println("Codex PhoneOps", control.Version)
		return nil
	}
	if cmd == "card" {
		show(Card)
		return nil
	} // independent of config, lock and network
	if cmd != "" && cmd != "setup" && cmd != "check" {
		return errors.New("利用可能: setup / card / check / version")
	}
	fs := flag.NewFlagSet("phoneops win11", flag.ContinueOnError)
	dir := fs.String("config-dir", ConfigDir(), "Windows専用設定")
	state := fs.String("state-dir", StateDir(), "復旧メモ")
	ro := fs.Bool("readonly", false, "診断・案内のみ。保存・SSHシェルは禁止")
	asJSON := fs.Bool("json", false, "個人情報を含まない要約（check用）")
	cert := fs.Bool("certificate", false, "RDP証明書の照合情報も取得")
	replace := fs.Bool("replace", false, "旧Windows設定をbackupして再登録")
	host := fs.String("host", "", "既存SSH alias（setup）")
	sshFile := fs.String("ssh-config", "", "既存SSH設定の絶対パス（setup）")
	expected := fs.String("expected-host", "", "本人確認したWindowsホスト名（setup）")
	user := fs.String("user", "", "DOMAIN\\user（setup）")
	rdp := fs.String("rdp-name", "", "保存済みRDP接続名（setup）")
	address := fs.String("rdp-address", "", "本人確認したRDP接続先（setup）")
	purpose := fs.String("purpose", "", "復旧目的（setup）")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("不明な引数です")
	}
	*dir = control.Expand(*dir)
	*state = control.Expand(*state)
	if !control.ValidAbs(*dir) || !control.ValidAbs(*state) {
		return errors.New("設定・メモには絶対パスが必要です")
	}
	unlock, e := control.Lock(*dir)
	if e != nil {
		return e
	}
	defer unlock()
	reader := bufio.NewReader(os.Stdin)
	if cmd == "setup" {
		if *ro {
			return errors.New("読取り専用では登録しません")
		}
		c := Config{Schema: 1, Label: "Windows", Alias: *host, SSHConfig: control.Expand(*sshFile), Host: *expected, User: *user, RDPName: *rdp, RDPAddress: *address, Purpose: *purpose}
		// Prompt missing facts rather than silently reusing historical host/IP values.
		for _, field := range []struct {
			p     *string
			label string
		}{{&c.Alias, "現在のSSH alias"}, {&c.SSHConfig, "既存SSH設定の絶対パス"}, {&c.Host, "対象Windowsのホスト名"}, {&c.User, "対象ユーザー（DOMAIN\\user）"}, {&c.RDPName, "保存済みRDP接続名"}, {&c.RDPAddress, "本人確認したRDPの接続先"}} {
			if *field.p == "" {
				s, err := line(reader, field.label)
				if err != nil {
					return err
				}
				*field.p = control.Expand(s)
			}
		}
		if c.Purpose == "" {
			c.Purpose, e = line(reader, "復旧したい作業名")
			if e != nil {
				return e
			}
		}
		return setup(c, *dir, *replace, reader)
	}
	c, e := Load(*dir)
	if e != nil {
		show("設定を読めません。手動カードは利用可能です。\nphoneops card")
		return e
	}
	if cmd == "check" {
		s, observation, err := check(c, *cert)
		if *asJSON {
			return printJSON(s, err)
		}
		display(c, s)
		if *cert && observation.Certificate.Full() {
			show("RDP証明書 SHA-1: " + *observation.Certificate.Value + "\nクライアントに同種の指紋が表示される場合のみ本人が照合してください。")
		}
		if err != nil {
			return err
		}
		return nil
	}
	return menu(c, *dir, *state, *ro, reader)
}
func printJSON(s Summary, e error) error {
	v := struct {
		Summary Summary `json:"summary"`
		Error   string  `json:"error,omitempty"`
	}{Summary: s}
	if e != nil {
		v.Error = e.Error()
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
	return e
}
func check(c Config, cert bool) (Summary, Observation, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	o, e := Probe(ctx, c, cert)
	if o.Schema == 1 {
		s := Decide(c, o)
		if ve := Verify(c, o); ve != nil {
			return s, o, ve
		}
		return s, o, e
	}
	s := Summary{State: "unknown", Message: "Windowsの現在状態はUNKNOWNです。", Next: "保存済みRDP手順・手動カードを利用できます。", Checks: map[string]string{"SSH": "UNKNOWN"}}
	if e != nil {
		s.Message = e.Error()
		var re *control.RunError
		if errors.As(e, &re) && re.Kind == "ssh_trust" {
			s.Next = "接続先の信頼を先に照合してください。警告を迂回しないでください。"
		}
	}
	return s, o, e
}
func display(c Config, s Summary) {
	show("対象: " + c.Label + " / 目的: " + c.Purpose + "\n" + s.Message + "\n次: " + s.Next)
	if !s.ObservedAt.IsZero() {
		show("診断: " + s.ObservedAt.Local().Format("15:04"))
	}
	for _, key := range []string{"SSH", "identity", "sessions", "app", "candidates", "RDP", "machine", "Tailscale", "certificate"} {
		if v, ok := s.Checks[key]; ok {
			show(key + ": " + v)
		}
	}
}
func setup(c Config, dir string, replace bool, reader *bufio.Reader) error {
	path := filepath.Join(dir, "config.json")
	if _, e := os.Lstat(path); e == nil && !replace {
		return errors.New("設定は既にあります。置換は setup --replace を指定してください")
	}
	hash, e := control.HashFile(c.SSHConfig)
	if e != nil {
		return errors.New("SSH設定を読めません")
	}
	c.SSHHash = hash
	if e = c.Validate(false); e != nil {
		return e
	}
	show("登録候補: " + c.Host + " / " + c.User + "\nSSH: " + c.Alias + "\nRDP: " + c.RDPName + " / " + c.RDPAddress)
	if !ask(reader, "この対象へ固定の読取り診断を行いますか？") {
		return errors.New("登録を中止しました")
	}
	_, o, e := check(c, false)
	if e != nil {
		return e
	}
	if e = Verify(c, o); e != nil {
		return e
	}
	c.SID = o.Identity.Value.SID
	if o.Machine.Full() {
		c.Machine = o.Machine.Value.Hash
	}
	show("観測したユーザー: " + o.Identity.Value.User + "\nSID: " + c.SID + "\nホスト: " + o.Identity.Value.Host)
	show("対象アプリを明示選択します。0 は未選択（UNKNOWN）。")
	candidates := []Candidate{}
	if o.Candidates.Full() {
		candidates = *o.Candidates.Value
	}
	for i, v := range candidates {
		show(fmt.Sprintf("%d: %s\n%s", i+1, v.Name, v.ID))
	}
	if len(candidates) > 0 {
		value, e := line(reader, "番号（0で保留）:")
		if e != nil {
			return e
		}
		n, e := strconv.Atoi(value)
		if e != nil || n < 0 || n > len(candidates) {
			return errors.New("選択が不正です。保存しません")
		}
		if n > 0 {
			c.AppID = candidates[n-1].ID
		}
	}
	if !ask(reader, "この接続主体・RDP情報・選択アプリを保存しますか？") {
		return errors.New("保存を中止しました")
	}
	if e = c.Validate(true); e != nil {
		return e
	}
	if e = c.Trust(); e != nil {
		return e
	}
	if replace {
		if b, e := os.ReadFile(path); e == nil {
			if e = control.AtomicWrite(path+".backup-"+time.Now().Format("20060102-150405.000000000"), b); e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	if e = save(path, c); e != nil {
		return e
	}
	show("登録しました。Windowsの設定・アプリ起動は変更していません。")
	return nil
}
func menu(c Config, dir, state string, ro bool, reader *bufio.Reader) error {
	rec, e := LoadRecovery(state)
	if e != nil && !os.IsNotExist(e) {
		show("復旧メモを読めません。診断とカードは利用できます。")
	}
	if rec.Target != c.Alias || rec.Purpose != c.Purpose {
		rec = Recovery{Schema: 1, Target: c.Alias, Purpose: c.Purpose}
	}
	for {
		show("\nCodex PhoneOps / " + c.Label + "で" + c.Purpose + "を開く")
		if ro {
			show("読取り専用")
		}
		if !rec.ObservedAt.IsZero() {
			show("前回観測: " + rec.ObservedAt.Local().Format("2006-01-02 15:04") + " / " + rec.LastObservation + "\n現在は未確認。rで再確認。")
			if rec.Stale(time.Now()) {
				show("観測は古くなっています（STALE）。")
			}
		}
		if rec.Result != "" {
			show("前回の本人確認: " + rec.ConfirmedAt.Local().Format("2006-01-02 15:04") + "\n" + rec.Result)
		}
		k, e := line(reader, "1 準備状態を確認 / r 再確認\n2 RDP接続情報・手順\n3 直接SSH\n4 Android Remoteの本人確認\n? 手動カード / q 終了")
		if errors.Is(e, io.EOF) {
			return nil
		}
		if e != nil {
			return e
		}
		switch k {
		case "q", "":
			return nil
		case "?":
			show(Card)
		case "1", "r":
			show("固定診断中（最大12秒）。Ctrl+Cで待機取消。")
			s, _, e := check(c, false)
			display(c, s)
			rec.LastObservation = s.State
			rec.ObservedAt = time.Now().UTC()
			rec.Next = s.Next
			if e != nil {
				show("診断: " + e.Error())
			}
			if !ro {
				if e = SaveRecovery(state, rec); e != nil {
					show("復旧メモを保存できません。診断・手順は利用できます。")
				}
			}
		case "2":
			show("保存済みRDP: " + c.RDPName + "\n接続先: " + c.RDPAddress + "\nユーザー: " + c.User + "\nSSH・診断の信頼警告は解消してから接続してください。\n" + Card)
		case "3":
			if ro {
				show("読取り専用ではSSHシェルを開きません。")
				continue
			}
			if e = c.Trust(); e != nil {
				show(e.Error())
				continue
			}
			show("Windowsへ直接SSHします。以後の手入力は本ツールの制御外です。\nSSHだけ切断: Enter → ~ → .")
			t, e := control.NewTerminal()
			if e != nil {
				show(e.Error())
				continue
			}
			child := ssh(c, nil, true)
			child.Stdin = os.Stdin
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			e, restoreErr := interactive(child, t)
			if restoreErr != nil {
				return restoreErr
			}
			if e != nil {
				show("SSH画面の終了。Windows作業の完了は未確認です。")
			}
			reader.Reset(os.Stdin)
		case "4":
			if ro {
				show("読取り専用では本人確認を保存しません。")
				continue
			}
			k, e := line(reader, "1 Remoteで対象Windowsへ接続できた\n2 目的の作業を開けた\n3 対象Windowsに接続できるが作業がない\n4 まだ接続できない\n他: 戻る")
			if e != nil {
				return e
			}
			labels := map[string]string{"1": "Remoteで対象Windowsへ接続できた", "2": "目的の作業を開けた", "3": "対象Windowsに接続できるが目的の作業がない", "4": "まだ接続できない"}
			if label, ok := labels[k]; ok {
				rec.Result = label
				rec.ConfirmedAt = time.Now().UTC()
				if e = SaveRecovery(state, rec); e != nil {
					show("本人確認を保存できませんでした。")
				}
				if k == "3" {
					show("接続は回復。純正側でアカウント・workspace・プロジェクト・会話を確認。再起動しません。")
				}
			}
		}
	}
}
