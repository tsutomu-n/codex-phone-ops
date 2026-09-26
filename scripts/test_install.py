#!/usr/bin/env python3
"""Temporary HOME installer tests. No device, user config or network writes."""
from pathlib import Path
import os, subprocess, tempfile, shutil
ROOT = Path(__file__).resolve().parents[1]
def run(home, path=None):
    env = dict(os.environ, HOME=str(home))
    if path: env['PATH'] = str(path)+':'+env['PATH']
    return subprocess.run(['bash',str(ROOT/'install.sh')], input='y\n',text=True,capture_output=True,env=env,timeout=15)
with tempfile.TemporaryDirectory() as td:
    home=Path(td)/'home'; home.mkdir()
    config=home/'.config/codex-phone-ops/win11/config.json'; config.parent.mkdir(parents=True); config.write_text('preserve me')
    state=home/'.local/state/codex-phone-ops/win11/recovery.json'
    state.parent.mkdir(parents=True); state.write_text('preserve recovery')
    operation=home/'.local/state/codex-phone-ops/ubuntu/operations/pending.json'
    operation.parent.mkdir(parents=True); operation.write_text('{"outcome":"unknown"}')
    legacy=home/'.local/bin/phoneops'; legacy.parent.mkdir(parents=True)
    legacy.write_text('#!/bin/sh\n# CODEX_PHONE_OPS_MANAGED\nexit 0\n')
    old_launcher=legacy.read_bytes()
    old_widget=home/'.shortcuts/Codex PhoneOps'; old_widget.parent.mkdir(parents=True)
    old_widget.write_text('#!/bin/sh\n# CODEX_PHONE_OPS_MANAGED\nexec "$HOME/.local/bin/phoneops"\n')
    old_widget_bytes=old_widget.read_bytes()
    for name,widget in [('cpo','Codex PhoneOps')]:
        p=run(home); assert p.returncode==0,p.stdout+p.stderr
        launcher=home/'.local/bin'/name; before=launcher.read_bytes()
        assert not legacy.exists() and not legacy.is_symlink(), 'managed old launcher must be removed'
        backup=Path(p.stdout.split('配置しました。backup: ',1)[1].splitlines()[0])
        assert (backup/'legacy-launcher').read_bytes()==old_launcher
        p=run(home); assert p.returncode==0,p.stdout+p.stderr
        assert launcher.read_bytes()==before
        assert config.read_text()=='preserve me'
        assert state.read_text()=='preserve recovery'
        assert operation.read_text()=='{"outcome":"unknown"}'
        for command in ['version', 'card']:
            smoke=subprocess.run([str(launcher),command],env=dict(os.environ,HOME=str(home)),text=True,capture_output=True,timeout=5)
            assert smoke.returncode==0 and 'Codex PhoneOps' in smoke.stdout,smoke
        assert b'/.local/bin/cpo' in (home/'.shortcuts/Codex PhoneOps').read_bytes()
        assert list((home/'.shortcuts').iterdir())==[home/'.shortcuts/Codex PhoneOps']
        print('PASS install/update',name)
    launcher=home/'.local/bin/cpo'; before=launcher.read_bytes()
    widget=home/'.shortcuts/Codex PhoneOps'; widget_before=widget.read_bytes()
    fake=Path(td)/'fake'; fake.mkdir()
    (fake/'uname').write_text('#!/bin/sh\necho unsupported\n');(fake/'uname').chmod(0o700)
    assert run(home,path=fake).returncode!=0
    assert launcher.read_bytes()==before
    (fake/'uname').unlink()
    (fake/'mv').write_text('#!/bin/sh\ncase "$*" in *PhoneOps*) exit 1;; esac\nexec /usr/bin/mv "$@"\n');(fake/'mv').chmod(0o700)
    assert run(home,path=fake).returncode!=0
    assert launcher.read_bytes()==before and widget.read_bytes()==widget_before
    print('PASS unsupported CPU / failed switch rollback')
    rollback_home=Path(td)/'rollback-home'; rollback_home.mkdir()
    rollback_legacy=rollback_home/'.local/bin/phoneops'; rollback_legacy.parent.mkdir(parents=True)
    rollback_legacy.write_bytes(old_launcher)
    rollback_widget=rollback_home/'.shortcuts/Codex PhoneOps'; rollback_widget.parent.mkdir(parents=True)
    rollback_widget.write_bytes(old_widget_bytes)
    previous_widget=rollback_widget.read_bytes()
    assert run(rollback_home,path=fake).returncode!=0
    assert rollback_legacy.read_bytes()==old_launcher
    assert rollback_widget.read_bytes()==previous_widget
    assert not (rollback_home/'.local/bin/cpo').exists()
    print('PASS legacy launcher restored after failed switch')
    (fake/'mv').unlink()
    (fake/'rm').write_text('#!/bin/sh\ncase "$*" in *bin/phoneops*) exit 1;; esac\nexec /usr/bin/rm "$@"\n');(fake/'rm').chmod(0o700)
    assert run(rollback_home,path=fake).returncode!=0
    assert rollback_legacy.read_bytes()==old_launcher
    assert rollback_widget.read_bytes()==previous_widget
    assert not (rollback_home/'.local/bin/cpo').exists()
    (fake/'rm').unlink()
    print('PASS legacy removal failure rollback')
    legacy.write_text('unrelated old command')
    p=run(home); assert p.returncode==0,p.stdout+p.stderr
    assert legacy.read_text()=='unrelated old command' and '管理外の旧入口' in p.stderr
    legacy.unlink(); legacy.symlink_to(launcher)
    p=run(home); assert p.returncode==0,p.stdout+p.stderr
    assert legacy.is_symlink()
    legacy.unlink()
    print('PASS foreign legacy command and symlink protected')
    widget.write_text('unrelated widget')
    assert run(home).returncode!=0
    assert widget.read_text()=='unrelated widget' and launcher.read_bytes()==before
    widget.unlink(); widget.symlink_to(launcher)
    assert run(home).returncode!=0
    print('PASS foreign file / symlink protected')
    # A tampered payload fails its checksum before switching any entry.
    copy=Path(td)/'bundle'; (copy/'scripts').mkdir(parents=True); (copy/'dist').mkdir()
    shutil.copy(ROOT/'scripts/install-product.sh',copy/'scripts/install-product.sh')
    shutil.copy(ROOT/'dist/SHA256SUMS',copy/'dist/SHA256SUMS')
    (copy/'dist/cpo-linux-amd64').write_text('broken')
    env=dict(os.environ,HOME=str(home))
    p=subprocess.run(['bash',str(copy/'scripts/install-product.sh')],input='y\n',text=True,capture_output=True,env=env)
    assert p.returncode!=0 and launcher.read_bytes()==before
    print('PASS corrupt payload protected')
