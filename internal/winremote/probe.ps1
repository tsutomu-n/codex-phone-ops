param($Request)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
function Unknown($status = 'unavailable') { @{status=$status; completeness='partial'; value=$null} }
function Observed($value) { @{status='observed'; completeness='full'; value=$value} }
function Failure($record) {
    $ex = $record.Exception
    while ($ex) {
        if (($ex -is [ComponentModel.Win32Exception] -and $ex.NativeErrorCode -eq 5) -or $ex -is [UnauthorizedAccessException]) { return (Unknown 'denied') }
        $ex = $ex.InnerException
    }
    if ($record.Exception -is [UnauthorizedAccessException] -or $record.Exception.HResult -eq -2147024891 -or $record.CategoryInfo.Category -eq 'PermissionDenied') { return (Unknown 'denied') }
    return (Unknown)
}
$result = @{schemaVersion=1; requestId=$Request.requestId; capturedAt=[DateTime]::UtcNow.ToString('o')}
foreach ($name in @('identity','machine','sessions','candidates','app','rdp','tailscale','certificate')) { $result[$name] = Unknown }
try {
    $who = [Security.Principal.WindowsIdentity]::GetCurrent()
    $result.identity = Observed @{host=[Environment]::MachineName; user=$who.Name; sid=$who.User.Value}
} catch { $result.identity = Failure $_ }
function Machine {
    $guid = (Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Cryptography').MachineGuid
    if (-not $guid) { throw 'identity unavailable' }
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $hash = ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($guid)))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
    $boot = (Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime().ToString('o')
    return @{hash=$hash;boot=$boot}
}
try { $result.machine = Observed (Machine) } catch { $result.machine = Failure $_ }
try {
Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Text;
public static class CCProbe {
 [StructLayout(LayoutKind.Sequential)] struct WTS { public int Id; public IntPtr Name; public int State; }
 public class Session { public int id; public string state; public string sid; }
 [DllImport("wtsapi32.dll", EntryPoint="WTSEnumerateSessionsW", SetLastError=true)] static extern bool Enumerate(IntPtr server,int reserved,int version,out IntPtr buffer,out int count);
 [DllImport("wtsapi32.dll", EntryPoint="WTSQuerySessionInformationW", SetLastError=true)] static extern bool Query(IntPtr server,int id,int kind,out IntPtr buffer,out int bytes);
 [DllImport("wtsapi32.dll")] static extern void WTSFreeMemory(IntPtr p);
 [DllImport("kernel32.dll",SetLastError=true)] static extern IntPtr OpenProcess(int access,bool inherit,int id);
 [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode)] static extern int GetApplicationUserModelId(IntPtr process,ref uint length,StringBuilder id);
 static string Text(int id,int kind) { IntPtr p;int n;if(!Query(IntPtr.Zero,id,kind,out p,out n))throw new Win32Exception();try{return Marshal.PtrToStringUni(p)??"";}finally{WTSFreeMemory(p);} }
 public static Session[] Sessions(string expectedSid) {
  IntPtr p;int n;if(!Enumerate(IntPtr.Zero,0,1,out p,out n))throw new Win32Exception();
  var rows=new List<Session>();
  try {int size=Marshal.SizeOf(typeof(WTS));for(int i=0;i<n;i++) {
   var row=(WTS)Marshal.PtrToStructure(IntPtr.Add(p,i*size),typeof(WTS));
   string user=Text(row.Id,5);if(user.Length==0)continue;
   string domain=Text(row.Id,7);
   string sid=((SecurityIdentifier)new NTAccount(domain,user).Translate(typeof(SecurityIdentifier))).Value;
   if(sid==expectedSid) rows.Add(new Session{id=row.Id,sid=sid,state=row.State==0?"Active":row.State==4?"Disconnected":"Other"});
  }}finally{WTSFreeMemory(p);}return rows.ToArray();
 }
 public static string AppID(int pid) {
  IntPtr h=OpenProcess(0x1000,false,pid);if(h==IntPtr.Zero)throw new Win32Exception();
  try {uint n=0;int rc=GetApplicationUserModelId(h,ref n,null);if(rc==15703)return "";if(rc!=122)throw new Win32Exception(rc);
   var b=new StringBuilder((int)n);rc=GetApplicationUserModelId(h,ref n,b);if(rc!=0)throw new Win32Exception(rc);return b.ToString();
  }finally{CloseHandle(h);}
 }
}
'@ | Out-Null
    $targetSID = $Request.sid
    if (-not $targetSID) { $targetSID = ([Security.Principal.NTAccount]::new($Request.user)).Translate([Security.Principal.SecurityIdentifier]).Value }
    $result.sessions = Observed @([CCProbe]::Sessions($targetSID))
} catch { $result.sessions = Failure $_ }
try {
    # Discovery is scoped to the SSH user's Start menu, never installation proof.
    $apps = @(Get-StartApps | Where-Object { $_.Name -match 'ChatGPT|Codex' } | ForEach-Object { @{name=$_.Name;id=$_.AppID} })
    $result.candidates = Observed $apps
} catch { $result.candidates = Failure $_ }
try {
    if ($Request.appId -and $result.sessions.status -eq 'observed' -and $result.identity.value.sid -eq $targetSID -and $result.candidates.status -eq 'observed') {
        $exact = @($apps | Where-Object { $_.id -ceq $Request.appId })
        if ($exact.Count -eq 1) {
            $ids = @($result.sessions.value | ForEach-Object { $_.id })
            $processes = @(Get-CimInstance Win32_Process)
            $found = $null; $complete = $true
            foreach ($p in $processes) {
                if ($ids -notcontains [int]$p.SessionId) { continue }
                try {
                    $owner = Invoke-CimMethod -InputObject $p -MethodName GetOwnerSid
                    if ($owner.ReturnValue -ne 0) { $complete=$false; continue }
                    if ($owner.Sid -ne $targetSID) { continue }
                    $aid = [CCProbe]::AppID([int]$p.ProcessId)
                    if ($aid -ceq $Request.appId) { $found=@{id=$aid;running=$true;sessionId=[int]$p.SessionId;sid=$targetSID} }
                } catch { $complete=$false }
            }
            if ($found) { $result.app = Observed $found }
            elseif ($complete) { $result.app = Observed @{id=$Request.appId;running=$false;sessionId=-1;sid=$targetSID} }
        }
    }
} catch { $result.app = Failure $_ }
try {
    $service = (Get-Service -Name TermService).Status.ToString()
    $port = [int](Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp').PortNumber
    if ($port -lt 1 -or $port -gt 65535) { throw 'unknown port' }
    $listeners = @(Get-NetTCPConnection -State Listen)
    $result.rdp = Observed @{service=$service;port=$port;listening=(@($listeners | Where-Object { $_.LocalPort -eq $port }).Count -gt 0)}
} catch { $result.rdp = Failure $_ }
try { $result.tailscale = Observed ((Get-Service -Name Tailscale).Status.ToString()) } catch { $result.tailscale = Failure $_ }
if ($Request.certificate) {
    try {
        $cert = (Get-CimInstance -Namespace 'root/cimv2/TerminalServices' -ClassName Win32_TSGeneralSetting -Filter "TerminalName='RDP-tcp'").SSLCertificateSHA1Hash
        if (-not $cert) { throw 'certificate unknown' }
        $result.certificate = Observed $cert
    } catch { $result.certificate = Failure $_ }
}
# Recheck generation/session identity; never combine observations across a change.
try {
    $after = Machine
    if ($result.machine.status -eq 'observed' -and ($after.boot -ne $result.machine.value.boot -or $after.hash -ne $result.machine.value.hash)) { $result.sessions=Unknown; $result.app=Unknown }
} catch { $result.machine=Unknown }
try {
    if ($result.sessions.status -eq 'observed') {
        $again = @([CCProbe]::Sessions($targetSID))
        if (($again | ConvertTo-Json -Compress) -cne ($result.sessions.value | ConvertTo-Json -Compress)) { $result.sessions=Unknown; $result.app=Unknown }
    }
} catch { $result.app=Unknown; $result.sessions=Unknown }
$bytes = [Text.Encoding]::UTF8.GetBytes(($result | ConvertTo-Json -Depth 10 -Compress))
$stdout = [Console]::OpenStandardOutput()
$stdout.Write($bytes,0,$bytes.Length)
$stdout.Flush()
