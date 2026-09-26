#!/usr/bin/env python3
"""Network-free Windows entry setup/menu/readonly/card tests using fake ssh."""
import json, os, subprocess, tempfile, hashlib
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
BIN=ROOT/'dist/cpo-linux-amd64'
with tempfile.TemporaryDirectory() as td:
    d=Path(td); config=d/'config';state=d/'state'; config.mkdir()
    env=dict(os.environ, HOME=td, PATH=td+':'+os.environ['PATH'])
    def run(args, data=''):
        return subprocess.run([str(BIN),"win11",*args],input=data,text=True,capture_output=True,env=env,timeout=15)
    (config/'config.json').write_text('broken')
    assert run(['card','--config-dir',str(config)]).returncode==0
    (config/'config.json').unlink()
    ssh=d/'ssh'; fixture=ROOT/'internal/winremote/testdata/denied.json'
    ssh.write_text('#!/usr/bin/env python3\nimport sys,json\nr=json.load(sys.stdin)\no=json.load(open('+repr(str(fixture))+'))\no["requestId"]=r["data"]["requestId"]\nprint(json.dumps(o))\n')
    ssh.chmod(0o700)
    sshconfig=d/'ssh_config';sshconfig.write_text('Host test\n')
    p=run(['setup','--config-dir',str(config),'--host','test','--ssh-config',str(sshconfig),'--expected-host','TEST','--user','TEST\\dev','--rdp-name','saved','--rdp-address','example.invalid'],'example work\ny\n1\ny\n')
    assert p.returncode==0,p.stdout+p.stderr
    c=json.loads((config/'config.json').read_text());assert c['appId']=='OpenAI.Test!App' and c['purpose']=='example work'
    before=(config/'config.json').read_bytes()
    p=run(['--config-dir',str(config),'--state-dir',str(state)],'1\n4\n3\nq\n')
    assert p.returncode==0,p.stdout+p.stderr
    rec=json.loads((state/'recovery.json').read_text());assert rec['lastObservation']=='unknown' and '作業がない' in rec['userResult']
    memo=(state/'recovery.json').read_bytes()
    p=run(['--readonly','--config-dir',str(config),'--state-dir',str(state)],'1\n3\n4\nq\n')
    assert p.returncode==0 and (state/'recovery.json').read_bytes()==memo and (config/'config.json').read_bytes()==before
    p=run(['--config-dir',str(config),'--state-dir',str(state)],'')
    assert p.returncode==0,'EOF must finish'
    p=run(['check','--json','--config-dir',str(config)])
    data=json.loads(p.stdout);assert data['summary']['state']=='unknown'
    for secret in ['S-1-5-test','TEST\\dev','example.invalid','OpenAI.Test!App']:assert secret not in p.stdout
    assert not (d/'unexpected').exists()
    print('PASS Windows setup selection / denied diagnostic / memo / readonly / EOF / private JSON / card')
