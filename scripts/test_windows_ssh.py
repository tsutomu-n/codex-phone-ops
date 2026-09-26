#!/usr/bin/env python3
"""Opt-in isolated Windows SSH test. Never defaults to a personal host.
Run separately against disposable sshd with cmd.exe and PowerShell default shells.
This script does not alter sshd, host trust or any Windows settings.
"""
import argparse, base64, json, subprocess, uuid
from pathlib import Path
p=argparse.ArgumentParser()
p.add_argument('--isolated-host',required=True)
p.add_argument('--ssh-config',required=True)
p.add_argument('--expected-host',required=True)
p.add_argument('--user',required=True)
a=p.parse_args()
request_id=uuid.uuid4().hex
code=(Path(__file__).resolve().parents[1]/'internal/winremote/probe.ps1').read_text()
boot="$ErrorActionPreference='Stop';[Console]::InputEncoding=New-Object System.Text.UTF8Encoding($false);$r=[Console]::In.ReadToEnd()|ConvertFrom-Json;& ([ScriptBlock]::Create($r.code)) $r.data"
command='powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand '+base64.b64encode(boot.encode('utf-16le')).decode()
payload=json.dumps(dict(code=code,data=dict(requestId=request_id,user=a.user,sid='',appId='',certificate=False)),ensure_ascii=True)
cmd=['ssh','-F',a.ssh_config,'-T','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','UpdateHostKeys=no','-o','ForwardAgent=no','-o','ConnectTimeout=6','-o','ConnectionAttempts=1','--',a.isolated_host,command]
r=subprocess.run(cmd,input=payload,text=True,capture_output=True,timeout=12)
assert r.returncode==0,'SSH/command failed; raw stderr intentionally not exported'
o=json.loads(r.stdout.lstrip('\ufeff'))
assert o['requestId']==request_id and o['schemaVersion']==1
assert o['identity']['status']=='observed'
assert o['identity']['value']['host'].casefold()==a.expected_host.casefold()
assert o['identity']['value']['user'].casefold()==a.user.casefold()
assert o['app']['status']!='observed'
print('PASS isolated SSH contract; no GUI / Remote acceptance claim')
