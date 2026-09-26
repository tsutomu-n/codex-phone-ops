#!/usr/bin/env python3
"""Offline entrypoint and default namespace checks; SSH must never execute."""
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
BIN = ROOT / 'dist/phoneops-linux-amd64'
with tempfile.TemporaryDirectory() as td:
    home = Path(td)
    ssh = home / 'ssh'
    ssh.write_text('#!/bin/sh\ntouch "$HOME/unexpected-ssh"\nexit 99\n')
    ssh.chmod(0o700)
    env = dict(os.environ, HOME=td, PATH=td+':'+os.environ['PATH'])
    def run(*args):
        return subprocess.run([str(BIN), *args], env=env, text=True,
                              capture_output=True, timeout=5)
    for args in [('version',), ('ubuntu', 'version'), ('win11', 'version')]:
        p = run(*args)
        assert p.returncode == 0 and p.stdout.strip() == 'Codex PhoneOps 0.2.0', p
    for args in [('card',), ('win11', 'card'), ('help',), ('--help',)]:
        p = run(*args)
        assert p.returncode == 0 and 'Codex PhoneOps' in p.stdout, p
    assert not (home/'.config').exists(), 'offline commands must not create config/locks'
    for args in [('ubuntu',), ('win11',)]:
        p = run(*args)
        assert p.returncode != 0 and 'codex-phone-ops' in p.stderr, p
    for target in ['ubuntu', 'win11']:
        assert (home/'.config/codex-phone-ops'/target/'instance.lock').exists()
    p = run('ubuntu', '-h')
    assert str(home/'.local/state/codex-phone-ops/ubuntu') in p.stderr
    p = run('win11', '-h')
    assert str(home/'.local/state/codex-phone-ops/win11') in p.stderr
    assert run('unknown-command').returncode != 0
    assert not (home/'unexpected-ssh').exists()
    assert not (home/'.cache').exists()
    assert not (home/'.local/state').exists(), 'help must not create state'
    opened = home / 'opened-url'
    opener = home / 'termux-open-url'
    opener.write_text('#!/bin/sh\ntouch "$HOME/opened-url"\n')
    opener.chmod(0o700)
    for flag in ('--readonly', '--read-only', '--invalid'):
        p = run('ubuntu', 'cloud', flag)
        assert p.returncode != 0 and not opened.exists(), p
    assert run('ubuntu', 'cloud').returncode == 0 and opened.exists()
with tempfile.NamedTemporaryFile(prefix='SHOULD_NOT_PACKAGE-', suffix='.txt',
                                 dir=ROOT / 'internal') as secret, \
     tempfile.NamedTemporaryFile(prefix='z_should_not_build_', suffix='.go',
                                 dir=ROOT / 'cmd/phoneops') as extra_go:
    secret.write(b'local private test fixture')
    secret.flush()
    extra_go.write(b'package main\nimport "os"\nfunc init() { os.Stderr.WriteString("UNTRACKED_GO_MARKER\\n") }\n')
    extra_go.flush()
    p = subprocess.run(['bash', 'scripts/package.sh'], cwd=ROOT, text=True,
                       capture_output=True, timeout=90)
    assert p.returncode == 0, p.stdout + p.stderr
    source_archive = ROOT / 'dist/release/codex-phone-ops-0.2.0-source.tar.gz'
    with tarfile.open(source_archive, 'r:gz') as tar:
        names = tar.getnames()
        assert not any(Path(name).name in (Path(secret.name).name, Path(extra_go.name).name)
                       for name in names)
        assert any(name.endswith('/scripts/package.sh') for name in names)
        source_license = next(m for m in tar.getmembers() if m.name.endswith('/LICENSE'))
        assert tar.extractfile(source_license).read() == (ROOT / 'LICENSE').read_bytes()
    binary_archive = ROOT / 'dist/release/codex-phone-ops-0.2.0-linux.tar.gz'
    with tempfile.TemporaryDirectory() as td:
        with tarfile.open(source_archive, 'r:gz') as tar:
            tar.extractall(td, filter='data')
        source_dir = Path(td) / 'codex-phone-ops-0.2.0-source'
        subprocess.run(['bash', 'scripts/build.sh'], cwd=source_dir,
                       check=True, capture_output=True, timeout=90)
        with tarfile.open(binary_archive, 'r:gz') as tar:
            binary_license = next(m for m in tar.getmembers() if m.name.endswith('/LICENSE'))
            assert tar.extractfile(binary_license).read() == (ROOT / 'LICENSE').read_bytes()
            member = next(m for m in tar.getmembers() if m.name.endswith('/dist/phoneops-linux-amd64'))
            packaged_bin = Path(td) / 'phoneops'
            packaged_bin.write_bytes(tar.extractfile(member).read())
            assert packaged_bin.read_bytes() == (source_dir / 'dist/phoneops-linux-amd64').read_bytes()
            arm_member = next(m for m in tar.getmembers() if m.name.endswith('/dist/phoneops-linux-arm64'))
            assert tar.extractfile(arm_member).read() == (source_dir / 'dist/phoneops-linux-arm64').read_bytes()
        packaged_bin.chmod(0o700)
        p = subprocess.run([str(packaged_bin), 'version'], text=True,
                           capture_output=True, timeout=5)
        assert p.returncode == 0 and 'UNTRACKED_GO_MARKER' not in p.stderr, p
    with tempfile.NamedTemporaryFile(prefix='UNEXPECTED_ARTIFACT-',
                                     dir=ROOT / 'dist/release') as unexpected:
        p = subprocess.run(['bash', 'scripts/package.sh'], cwd=ROOT, text=True,
                           capture_output=True, timeout=5)
        assert p.returncode != 0 and 'Unexpected release artifact' in p.stderr, p
        assert Path(unexpected.name).exists()
print('PASS CLI routing / branding / offline commands / tracked-only packages')
