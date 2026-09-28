#!/usr/bin/env python3
"""PowerShell collection logic with explicitly substituted OS APIs.
Runs on Linux pwsh; does NOT validate WTS/PInvoke/OpenSSH/Windows permissions.
Production probe and transport remain fixed and contain no test hooks.
"""
import base64,json,shutil,subprocess
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
pwsh=shutil.which('pwsh')
if not pwsh:
    raise SystemExit('pwsh is required for this optional collection-logic test')
source=(ROOT/'internal/winremote/probe.ps1').read_text()
start=source.index("Add-Type -TypeDefinition @'")
end=source.index("'@ | Out-Null",start)+len("'@ | Out-Null")
fake=r'''Add-Type -TypeDefinition @'
using System;
 public static class CCProbe {
 public class Session { public int id=3; public string state="Disconnected"; public string sid="S-1-5-test"; }
 public static Session[] Sessions(string sid) {
  string mode=Environment.GetEnvironmentVariable("CC_SCENARIO");
  if(mode=="denied")throw new UnauthorizedAccessException();
  if(mode=="none")return new Session[0];
  if(mode=="multi")return new Session[]{new Session(),new Session(){id=4}};
  return new Session[]{new Session()};
 }
 public static string AppID(int pid) {
  string mode=Environment.GetEnvironmentVariable("CC_SCENARIO");
  Console.Error.WriteLine("APP_CALL");
  if(mode=="path-denied")throw new UnauthorizedAccessException();
  return mode=="classic"?"Classic!App":mode=="unrelated"?"Other!App":"OpenAI.Test!App";
 }
}
'@ | Out-Null'''
source=source[:start]+fake+source[end:]
source=source.replace('$who = [Security.Principal.WindowsIdentity]::GetCurrent()',"$who = [pscustomobject]@{Name='TEST\\dev';User=[pscustomobject]@{Value='S-1-5-test'}}")
mocks=r'''
function Get-ItemProperty {
 param($LiteralPath)
 if($env:CC_SCENARIO -eq 'machine-unavailable' -and $LiteralPath -match 'Cryptography'){throw 'machine denied'}
 [pscustomobject]@{MachineGuid='test';PortNumber=3389}
}
function Get-CimInstance {
 param($ClassName,$Filter)
 if($ClassName -eq 'Win32_OperatingSystem'){return [pscustomobject]@{LastBootUpTime=[DateTime]'2026-01-01'}}
 if($env:CC_SCENARIO -eq 'absent'){return}
 if($Filter -match 'ProcessId'){
  $created=[DateTime]'2026-01-01';if($env:CC_SCENARIO -eq 'generation-change'){$created=[DateTime]'2026-01-02'}
  return [pscustomobject]@{SessionId=3;ProcessId=100;CreationDate=$created}
 }
 if($Filter -notmatch 'SessionId = 3'){throw 'unscoped process query'}
 if($env:CC_SCENARIO -eq 'multi' -and $Filter -notmatch 'SessionId = 4'){throw 'missing second session'}
 if($env:CC_SCENARIO -eq 'early'){
  return @([pscustomobject]@{SessionId=3;ProcessId=100;CreationDate=[DateTime]'2026-01-01'},
    [pscustomobject]@{SessionId=3;ProcessId=101;CreationDate=[DateTime]'2026-01-01'})
 }
 [pscustomobject]@{SessionId=3;ProcessId=100;CreationDate=[DateTime]'2026-01-01'}
}
function Invoke-CimMethod {
 param($InputObject,$MethodName)
 [Console]::Error.WriteLine('OWNER_CALL')
 $sid='S-1-5-test';if($env:CC_SCENARIO -eq 'other-owner'){$sid='other'}
 [pscustomobject]@{ReturnValue=0;Sid=$sid}
}
function Get-StartApps {
 if($env:CC_SCENARIO -eq 'no-candidates'){return}
 [pscustomobject]@{Name='ChatGPT';AppID='OpenAI.Test!App'}
 [pscustomobject]@{Name='ChatGPT Classic';AppID='Classic!App'}
}
function Get-Service {param($Name);[pscustomobject]@{Status='Running'}}
function Get-NetTCPConnection {param($State);[pscustomobject]@{LocalPort=3389}}
'''
boot="$ErrorActionPreference='Stop';$r=[Console]::In.ReadToEnd()|ConvertFrom-Json;. ([ScriptBlock]::Create($r.mocks));$env:CC_SCENARIO=$r.mode;& ([ScriptBlock]::Create($r.code)) $r.data"
command=[pwsh,'-NoProfile','-NonInteractive','-EncodedCommand',base64.b64encode(boot.encode('utf-16le')).decode()]
for mode in ['running','none','denied','absent','classic','other-owner','path-denied','no-candidates','machine-unavailable','unrelated','generation-change','multi','early']:
    payload=dict(code=source,mocks=mocks,mode=mode,data=dict(requestId='test',user='TEST\\dev',sid='S-1-5-test',appId='OpenAI.Test!App',certificate=False))
    r=subprocess.run(command,input=json.dumps(payload),text=True,capture_output=True,timeout=12)
    assert r.returncode==0,(mode,r.stderr)
    o=json.loads(r.stdout)
    assert o['rdp']['status']=='observed',o
    if mode=='denied':assert o['sessions']['status']=='denied' and o['app']['status']!='observed',o
    if mode=='none':assert o['sessions']['value']==[],o
    if mode in ['running','machine-unavailable','multi','early']:assert o['app']['value']['running'] is True,o
    if mode in ['none','absent','classic','other-owner','unrelated']:assert o['app']['value']['running'] is False,o
    if mode in ['path-denied','no-candidates','generation-change']:assert o['app']['status']!='observed',o
    if mode in ['none','classic','unrelated']:assert 'OWNER_CALL' not in r.stderr,(mode,r.stderr)
    if mode=='running':assert r.stderr.count('OWNER_CALL')==1,r.stderr
    if mode=='early':assert r.stderr.count('APP_CALL')==1,r.stderr
    print('PASS mocked PowerShell collection',mode)
