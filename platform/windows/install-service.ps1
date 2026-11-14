param(
    [string]$BinaryPath = "C:\Program Files\IslandWeave\islandd.exe",
    [string]$DataDir = "$env:ProgramData\IslandWeave"
)

New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Service -Name "IslandWeaveDaemon" `
    -BinaryPathName "`"$BinaryPath`" --data-dir `"$DataDir`"" `
    -DisplayName "IslandWeave Core Daemon (islandd)" `
    -StartupType Automatic `
    -Description "Decentralized offline-first IslandWeave networking daemon."
Start-Service -Name "IslandWeaveDaemon"
