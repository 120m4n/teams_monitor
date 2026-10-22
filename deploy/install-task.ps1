# Registers Teams Monitor as a Task Scheduler task for the current user:
# starts at logon, restarts on failure, and re-launches every 5 min if it died
# (MultipleInstancesPolicy=IgnoreNew makes that a no-op while it's running).
#
#   powershell -ExecutionPolicy Bypass -File deploy\install-task.ps1 -Exe C:\Tools\teams_monitor.exe -Arguments "--serial COM3"
param(
    [Parameter(Mandatory)] [string] $Exe,
    [string] $Arguments = "",
    [string] $TaskName = "TeamsMonitor"
)
$ErrorActionPreference = "Stop"

$exePath = (Resolve-Path $Exe).Path
$user    = "$env:USERDOMAIN\$env:USERNAME"
$esc     = { param($s) [Security.SecurityElement]::Escape($s) }

$xml = @"
<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Detects Microsoft Teams calls and drives the ESP32-S3 LED ring.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>$(& $esc $user)</UserId>
      <Repetition>
        <Interval>PT5M</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>$(& $esc $user)</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>999</Count>
    </RestartOnFailure>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>$(& $esc $exePath)</Command>
      <Arguments>$(& $esc $Arguments)</Arguments>
    </Exec>
  </Actions>
</Task>
"@

$tmp = Join-Path $env:TEMP "$TaskName.xml"
$xml | Set-Content -Path $tmp -Encoding Unicode   # schtasks /xml wants UTF-16
schtasks /Create /TN $TaskName /XML $tmp /F
if ($LASTEXITCODE -ne 0) { throw "schtasks failed ($LASTEXITCODE)" }
Remove-Item $tmp
schtasks /Run /TN $TaskName
