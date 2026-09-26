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
    for name,widget in [('phoneops','Codex PhoneOps')]:
        p=run(home); assert p.returncode==0,p.stdout+p.stderr
        launcher=home/'.local/bin'/name; before=launcher.read_bytes()
        p=run(home); assert p.returncode==0,p.stdout+p.stderr
        assert launcher.read_bytes()==before
        assert config.read_text()=='preserve me'
        assert state.read_text()=='preserve recovery'
        assert operation.read_text()=='{"outcome":"unknown"}'
        for command in ['version', 'card']:
            smoke=subprocess.run([str(launcher),command],env=dict(os.environ,HOME=str(home)),text=True,capture_output=True,timeout=5)
            assert smoke.returncode==0 and 'Codex PhoneOps' in smoke.stdout,smoke
        assert list((home/'.shortcuts').iterdir())==[home/'.shortcuts/Codex PhoneOps']
        print('PASS install/update',name)
    launcher=home/'.local/bin/phoneops'; before=launcher.read_bytes()
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
    (copy/'dist/phoneops-linux-amd64').write_text('broken')
    env=dict(os.environ,HOME=str(home))
    p=subprocess.run(['bash',str(copy/'scripts/install-product.sh')],input='y\n',text=True,capture_output=True,env=env)
    assert p.returncode!=0 and launcher.read_bytes()==before
    print('PASS corrupt payload protected')
