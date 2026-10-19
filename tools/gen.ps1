# Regenerates protocol code for the server (Go) and the client (GDScript).
# Usage: powershell -File tools/gen.ps1
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$bin = Join-Path $PSScriptRoot "bin"

Push-Location $PSScriptRoot
try {
    New-Item -ItemType Directory -Force $bin | Out-Null
    go build -o "$bin\" github.com/bufbuild/buf/cmd/buf google.golang.org/protobuf/cmd/protoc-gen-go ./protoc-gen-gdscript
    if ($LASTEXITCODE -ne 0) { throw "building codegen tools failed" }
} finally {
    Pop-Location
}

$env:PATH = "$bin;$env:PATH"
Push-Location $root
try {
    buf lint
    if ($LASTEXITCODE -ne 0) { throw "buf lint failed" }
    buf generate
    if ($LASTEXITCODE -ne 0) { throw "buf generate failed" }
} finally {
    Pop-Location
}
Write-Host "Generated server/gen and client/gen"
