# Native Windows collector smoke test; no SSH, GUI acceptance or configuration changes.
$ErrorActionPreference='Stop'
$path=Join-Path $PSScriptRoot '../internal/winremote/probe.ps1'
$errors=$null; $tokens=$null
[System.Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$errors) > $null
if ($errors.Count) { throw ($errors | Out-String) }
$data=@{requestId='isolated-smoke';user=[Security.Principal.WindowsIdentity]::GetCurrent().Name;sid='';appId='';certificate=$false}
$payload=@{code=[IO.File]::ReadAllText((Resolve-Path $path));data=$data}|ConvertTo-Json -Depth 5 -Compress
$boot='$ErrorActionPreference="Stop";[Console]::InputEncoding=New-Object System.Text.UTF8Encoding($false);$r=[Console]::In.ReadToEnd()|ConvertFrom-Json;& ([ScriptBlock]::Create($r.code)) $r.data'
$encoded=[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($boot))
# ASCII escaped JSON preserves the stdin contract under Windows PowerShell 5.1.
$ascii=-join ($payload.ToCharArray()|ForEach-Object {if([int]$_ -gt 127){'\u{0:x4}' -f [int]$_}else{[string]$_}})
$null=$ascii | ConvertFrom-Json
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
 $child.StandardInput.Write($ascii)
 $child.StandardInput.Close()
 $out=$child.StandardOutput.ReadToEnd()
 $null=$child.StandardError.ReadToEnd()
 $child.WaitForExit()
 if($child.ExitCode -ne 0){throw 'collector failed'}
} finally { $child.Dispose() }
$o=$out|ConvertFrom-Json
if($o.schemaVersion -ne 1 -or $o.requestId -ne 'isolated-smoke' -or $o.identity.status -ne 'observed'){throw 'contract failed'}
foreach($name in @('sessions','app','rdp','machine','candidates','certificate','tailscale')){if(-not $o.$name.status){throw "missing $name"}}
if($o.app.status -eq 'observed'){throw 'unselected app must remain unknown'}
Write-Host 'PASS Windows collector smoke (not GUI/Remote acceptance)'
