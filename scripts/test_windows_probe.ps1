# Native Windows collector smoke test; no SSH, GUI acceptance or configuration changes.
$ErrorActionPreference='Stop'
$path=Join-Path $PSScriptRoot '../internal/winremote/probe.ps1'
$errors=$null; $tokens=$null
[System.Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$errors) > $null
if ($errors.Count) { throw ($errors | Out-String) }
$data=@{requestId='isolated-smoke';user=[Security.Principal.WindowsIdentity]::GetCurrent().Name;sid='';appId='';certificate=$false}
$payload=@{code=[IO.File]::ReadAllText((Resolve-Path $path));data=$data}|ConvertTo-Json -Depth 5 -Compress
$boot=@'
$ErrorActionPreference='Stop'
[Console]::InputEncoding=New-Object System.Text.UTF8Encoding($false)
$raw=[Console]::In.ReadToEnd()
$prefix=($raw.ToCharArray()|Select-Object -First 8|ForEach-Object {[int]$_}) -join ','
[Console]::Error.WriteLine("stdin length=$($raw.Length) prefix=$prefix")
$r=$raw|ConvertFrom-Json
& ([ScriptBlock]::Create($r.code)) $r.data
'@
$encoded=[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($boot))
$null=$payload | ConvertFrom-Json
$start=New-Object Diagnostics.ProcessStartInfo
$start.FileName='powershell.exe'
$start.Arguments='-NoLogo -NoProfile -NonInteractive -EncodedCommand '+$encoded
$start.UseShellExecute=$false
$start.RedirectStandardInput=$true
$start.RedirectStandardOutput=$true
$start.RedirectStandardError=$true
$child=New-Object Diagnostics.Process
$child.StartInfo=$start
if(-not $child.Start()){throw 'collector did not start'}
try {
 $bytes=[Text.Encoding]::UTF8.GetBytes($payload)
 $child.StandardInput.BaseStream.Write($bytes,0,$bytes.Length)
 $child.StandardInput.Close()
 $out=$child.StandardOutput.ReadToEnd()
 $err=$child.StandardError.ReadToEnd()
 $child.WaitForExit()
 if($child.ExitCode -ne 0){throw "collector failed: $($err.Substring(0,[Math]::Min(2000,$err.Length)))"}
} finally { $child.Dispose() }
$o=$out|ConvertFrom-Json
if($o.schemaVersion -ne 1 -or $o.requestId -ne 'isolated-smoke' -or $o.identity.status -ne 'observed'){throw 'contract failed'}
foreach($name in @('sessions','app','rdp','machine','candidates','certificate','tailscale')){if(-not $o.$name.status){throw "missing $name"}}
if($o.app.status -eq 'observed'){throw 'unselected app must remain unknown'}
Write-Host 'PASS Windows collector smoke (not GUI/Remote acceptance)'
