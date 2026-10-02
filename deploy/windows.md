# Running melhttpd on Windows

`melhttpd.exe` is a single self-contained executable.

## Quick start

```powershell
.\melc.exe build -o site .\my-site
.\melhttpd.exe -root .\site -addr :8080
```

## Start automatically at boot (Task Scheduler)

```powershell
$action  = New-ScheduledTaskAction -Execute "C:\melhttp\melhttpd.exe" `
           -Argument "-root C:\melhttp\site -addr :8080 -log-format json"
$trigger = New-ScheduledTaskTrigger -AtStartup
Register-ScheduledTask -TaskName "melhttpd" -Action $action -Trigger $trigger `
           -User "NT AUTHORITY\NETWORK SERVICE" -RunLevel Limited
New-NetFirewallRule -DisplayName "melhttpd" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
```

## As a Windows service

melhttpd does not yet implement the Windows service control protocol (that is
on the roadmap). In the meantime, wrap it with a service manager such as
[NSSM](https://nssm.cc/) or [WinSW](https://github.com/winsw/winsw):

```powershell
nssm install melhttpd C:\melhttp\melhttpd.exe -root C:\melhttp\site -addr :8080
nssm start melhttpd
```
