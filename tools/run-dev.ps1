# Starts the server and two Godot clients side by side for local testing.
# Usage: powershell -File tools/run-dev.ps1 [-Clients 2]
param([int]$Clients = 2)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

$godot = (Get-Command godot -ErrorAction SilentlyContinue).Source
if (-not $godot) {
    $godot = Get-ChildItem "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\GodotEngine.GodotEngine*\Godot_v4*_win64.exe" |
        Where-Object { $_.Name -notmatch "console" } | Select-Object -First 1 -ExpandProperty FullName
}
if (-not $godot) { throw "Godot 4 not found; install it with: winget install GodotEngine.GodotEngine" }

Push-Location "$root\server"
try {
    go build -o bin\ ./cmd/...
    if ($LASTEXITCODE -ne 0) { throw "server build failed" }
} finally {
    Pop-Location
}

$server = Start-Process "$root\server\bin\server.exe" -PassThru -WindowStyle Minimized
Start-Sleep -Milliseconds 500

$models = @("zuo_mo", "mannequin_f", "mannequin_m")
$names = @("ZuoMo", "Lin", "Bao")
for ($i = 0; $i -lt $Clients; $i++) {
    $x = 20 + ($i % 2) * 820
    $y = 60 + [math]::Floor($i / 2) * 540
    Start-Process $godot -ArgumentList "--path", "`"$root\client`"", "--", "--name=$($names[$i % 3])$i",
        "--model=$($models[$i % 3])", "--autoconnect", "--window=$x,$y,800,500"
}

Write-Host "Server PID $($server.Id). Close the client windows, then press Enter to stop the server."
Read-Host | Out-Null
Stop-Process -Id $server.Id -ErrorAction SilentlyContinue
