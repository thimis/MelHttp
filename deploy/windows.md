# Running melhttpd on Windows

`melhttpd.exe` is a single self-contained executable, and it can install itself
as a native Windows service.

## Quick start

```powershell
.\melc.exe build -o site .\my-site
.\melhttpd.exe -root .\site -addr :8080
```

## As a Windows service (recommended)

From an **elevated** PowerShell (Run as Administrator):

```powershell
.\melhttpd.exe -service install -root C:\melhttp\site -addr :80     # plus any other flags
.\melhttpd.exe -service start
New-NetFirewallRule -DisplayName "melhttpd" -Direction Inbound -Protocol TCP -LocalPort 80 -Action Allow
```

- **Startup and recovery:** the service starts automatically at boot and
  restarts after a crash (after 5 s, then 10 s, then every 30 s).
- **Flags:** every other flag you pass to `-service install` is stored in the
  service definition. Relative paths (`-root`, `-log-file`, `-tls-cert`, …)
  are made absolute, because services start in `C:\Windows\System32`.
- **Logs:** services have no console, so logs go to `-log-file`. If you don't
  give one, it defaults to `melhttpd.log` next to the executable.
- **Managing it:** `-service stop`, `-service start`, `-service uninstall`. You
  can also use `services.msc`, or `sc.exe query melhttpd`.
- **Several sites:** run each as its own service with
  `-service-name <name>` on every `-service` command.

To change flags later, run `-service uninstall`, then `-service install` again.

## HTTPS on Windows

```powershell
.\melhttpd.exe -service install -root C:\melhttp\site -addr :80 -tls-addr :443 `
  -acme-domains example.com -acme-email you@example.com -acme-cache C:\melhttp\certs -hsts 8760h
```

Allow ports 80 and 443 through the firewall. The ACME cache holds private
keys, so keep it in a folder only Administrators and the service can read.

## Without the service manager

Task Scheduler works too:

```powershell
$action  = New-ScheduledTaskAction -Execute "C:\melhttp\melhttpd.exe" `
           -Argument "-root C:\melhttp\site -addr :8080 -log-file C:\melhttp\melhttpd.log"
Register-ScheduledTask -TaskName "melhttpd" -Action $action -Trigger (New-ScheduledTaskTrigger -AtStartup) `
           -User "NT AUTHORITY\NETWORK SERVICE"
```
