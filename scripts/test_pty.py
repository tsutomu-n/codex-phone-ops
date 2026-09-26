#!/usr/bin/env python3
"""Repeatable Linux terminal integration tests.
The shell, binary, PTY, signals and termios are real. Codex is a harmless
fixture CLI, NOT a claim of validation against an actual Codex runtime.
No SSH/network/user projects are accessed.
"""
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time

BIN = Path(sys.argv[1] if len(sys.argv) > 1 else "dist/phoneops-linux-amd64").resolve()
FIXTURE = r'''#!/bin/sh
if [ "$1" = --version ]; then echo 'codex-cli 0.155.1'; exit 0; fi
if [ "$1 $2 $3" = 'app-server daemon version' ]; then
 if [ -f "$CC_TEST_DIR/running" ]; then state=running; else state=notRunning; fi
 printf '{"status":"%s","socketPath":"/tmp/cc-fixture.sock","appServerVersion":"0.155.1"}\n' "$state"
 exit 0
fi
if [ "$1 $2 $3" = 'app-server daemon start' ]; then
 echo start >> "$CC_TEST_DIR/starts"
 touch "$CC_TEST_DIR/running"
 if [ "$CC_TEST_MODE" = lost ]; then exit 255; fi
 echo '{"status":"started","socketPath":"/tmp/cc-fixture.sock"}'
 exit 0
fi
printf '%s\000' "$@" >> "$CC_TEST_DIR/args"
stty -a > "$CC_TEST_DIR/child-termios"
echo CC_NATIVE_STARTED
echo $$ > "$CC_TEST_DIR/child-pid"
read -r line
if [ "$line" = fail ]; then stty -echo -icanon; exit 7; fi
exit 0
'''


def scenario(mode: str):
    with tempfile.TemporaryDirectory(prefix="cc-pty-") as td:
        d = Path(td)
        fixture = d / "codex fixture'"
        fixture.write_text(FIXTURE)
        fixture.chmod(0o700)
        (d / "projects" / "日本語 project'").mkdir(parents=True)
        env = dict(os.environ, TERM="xterm-256color", CODEX_HOME=str(d / "codex-home"),
                   CC_TEST_DIR=td, CC_TEST_MODE=mode)
        base = [str(BIN), "ubuntu", "--state-dir", str(d / "state"), "--config-dir", str(d / "config"), "--local"]
        setup = subprocess.run([str(BIN), "ubuntu", "setup", "--local", "--config-dir", str(d / "config"),
                                "--codex-path", str(fixture), "--projects-root", str(d / "projects"),
                                "--ssh-config", "/tmp/unused-ssh-config"],
                               input="y\n", env=env, text=True, capture_output=True, timeout=8)
        assert setup.returncode == 0, setup.stdout + setup.stderr
        if mode in {"normal", "abnormal", "readonly", "signal", "signal-handoff", "resume", "eof"}:
            (d / "running").touch()
        if mode == "journal-denied":
            (d / "state").mkdir()
            (d / "state/operations").write_text("not a directory")
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 40, 0, 0))
        before = termios.tcgetattr(slave)
        def prepare():
            os.setsid()
            fcntl.ioctl(slave, termios.TIOCSCTTY, 0)
        argv = base + (["--readonly"] if mode == "readonly" else [])
        proc = subprocess.Popen(argv, env=env, stdin=slave, stdout=slave, stderr=slave,
                                preexec_fn=prepare)
        log = b""
        def until(s, timeout=8):
            nonlocal log
            needle = s.encode()
            end = time.monotonic() + timeout
            new = b""
            while time.monotonic() < end:
                if select.select([master], [], [], .1)[0]:
                    try:
                        b = os.read(master, 65536)
                    except OSError:
                        break
                    new += b
                    log += b
                    if needle in new:
                        return new
            raise AssertionError(f"missing {s}: {log[-3000:].decode(errors='replace')}")
        def send(s):
            os.write(master, s.encode())
        try:
            until("Codex PhoneOps")
            if mode == "readonly":
                send("1"); until("読取り専用")
                assert not (d / "args").exists()
                send("\x1b"); time.sleep(.15); send("q")
            elif mode == "signal":
                os.kill(proc.pid, signal.SIGTERM)
            else:
                if mode == "new":
                    send("3"); until("新規作業のフォルダーを選択"); send("\r")
                    until("新しいCodex作業"); send("y")
                else:
                    send("2" if mode == "resume" else "1")
                if mode in {"cold", "lost", "new", "journal-denied"}:
                    until("起動しますか"); send("y")
                if mode == "journal-denied":
                    until("起動しません"); send("\r"); time.sleep(.15); send("q")
                    assert not (d / "starts").exists()
                elif mode == "lost":
                    until("起動結果を確認できません")
                    send("\r"); until("Codex PhoneOps"); send("1")
                    until("CC_NATIVE_STARTED"); send("done\n")
                    until("Enterで戻る"); send("\n"); until("Codex PhoneOps"); send("q")
                else:
                    until("CC_NATIVE_STARTED")
                    if mode == "signal-handoff":
                        child_pid = int((d / "child-pid").read_text())
                        os.kill(proc.pid, signal.SIGTERM)
                        proc.wait(timeout=8)
                        try:
                            os.kill(child_pid, 0)
                        except ProcessLookupError:
                            pass
                        else:
                            raise AssertionError(f"orphan child PID {child_pid}")
                    else:
                        flags = (d / "child-termios").read_text()
                        assert "-icanon" not in flags and "-echo " not in flags, flags
                        # Resize during native handoff. The entry must restore itself afterwards.
                        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 28, 60, 0, 0))
                        send("fail\n" if mode == "abnormal" else "done\n")
                        until("Enterで戻る")
                        if mode == "eof":
                            send("\x04")
                        else:
                            send("\n"); until("Codex PhoneOps"); send("q")
            proc.wait(timeout=5)
            after = termios.tcgetattr(slave)
            assert before == after, (before, after)
            if mode in {"cold", "new"}:
                assert (d / "starts").read_text().splitlines() == ["start"]
                records = [json.loads(p.read_text()) for p in (d / "state/operations").glob("*.json")]
                assert records[0]["outcome"] == "accepted"
            if mode == "lost":
                assert (d / "starts").read_text().splitlines() == ["start"]
                records = [json.loads(p.read_text()) for p in (d / "state/operations").glob("*.json")]
                assert records[0]["outcome"] == "unknown"
                assert (d / "args").exists(), "explicit second selection may open observed running daemon"
            if mode in {"normal", "abnormal", "cold", "new", "resume"}:
                args = (d / "args").read_bytes().split(b"\0")
                if mode == "new":
                    assert args == [b"--remote", b"unix:///tmp/cc-fixture.sock", b"--no-alt-screen", b"-C", str(d / "projects" / "日本語 project'").encode(), b""], args
                elif mode == "resume":
                    assert args[:4] == [b"resume", b"--all", b"--remote", b"unix:///tmp/cc-fixture.sock"], args
                else:
                    assert args[:3] == [b"agents", b"--remote", b"unix:///tmp/cc-fixture.sock"], args
            print("PASS", mode)
        finally:
            if proc.poll() is None:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.wait()
            os.close(master); os.close(slave)


for case in ("normal", "abnormal", "cold", "new", "resume", "lost", "readonly", "signal", "signal-handoff", "eof", "journal-denied"):
    scenario(case)

for choice, expected in (("3", "WindowsのChatGPT Remoteへ戻る"), ("2", "codex-phone-ops"), ("q", "")):
    with tempfile.TemporaryDirectory(prefix="phoneops-top-") as td:
        master, slave = pty.openpty()
        before = termios.tcgetattr(slave)
        proc = subprocess.Popen([str(BIN)], env=dict(os.environ, HOME=td, TERM="dumb"),
                                stdin=slave, stdout=slave, stderr=slave)
        output = b""
        try:
            end = time.monotonic() + 5
            while b"Manual Recovery Card" not in output and time.monotonic() < end:
                if select.select([master], [], [], .1)[0]:
                    output += os.read(master, 65536)
            assert b"Manual Recovery Card" in output, output
            os.write(master, choice.encode())
            proc.wait(timeout=5)
            while select.select([master], [], [], .1)[0]:
                try:
                    output += os.read(master, 65536)
                except OSError:
                    break
            assert expected.encode() in output, output.decode(errors="replace")
            assert before == termios.tcgetattr(slave)
            if choice == "2":
                assert proc.returncode != 0
                assert (Path(td) / '.config/codex-phone-ops/win11/instance.lock').exists()
            else:
                assert proc.returncode == 0
        finally:
            if proc.poll() is None:
                proc.kill(); proc.wait()
            os.close(master); os.close(slave)
    print("PASS top menu", choice)

for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
    master, slave = pty.openpty()
    before = termios.tcgetattr(slave)
    def prepare_top():
        os.setsid()
        fcntl.ioctl(slave, termios.TIOCSCTTY, 0)
    proc = subprocess.Popen([str(BIN)], stdin=slave, stdout=slave, stderr=slave,
                            preexec_fn=prepare_top)
    output = b""
    try:
        end = time.monotonic() + 5
        while b"Manual Recovery Card" not in output and time.monotonic() < end:
            if select.select([master], [], [], .1)[0]:
                output += os.read(master, 65536)
        assert b"Manual Recovery Card" in output, output
        os.kill(proc.pid, sig)
        assert proc.wait(timeout=5) == 128 + sig.value
        assert before == termios.tcgetattr(slave)
    finally:
        if proc.poll() is None:
            proc.kill(); proc.wait()
        os.close(master); os.close(slave)
    print("PASS top menu signal", sig.name)
print("PTY integration passed. Actual Codex / SSH / Android not covered by this suite.")
