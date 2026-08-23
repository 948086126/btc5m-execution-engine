$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
New-Item -Force -ItemType Directory bin/windows-amd64, bin/linux-amd64 | Out-Null
$env:CGO_ENABLED="0"
$cmds = @("verify","replay","mock_live","mock_source","mock_collector","live_source")
foreach ($c in $cmds) {
  $env:GOOS="windows"; $env:GOARCH="amd64"
  go build -trimpath -o ("bin/windows-amd64/btc5m-" + ($c -replace '_','-') + ".exe") ("./cmd/"+$c)
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  $env:GOOS="linux"; $env:GOARCH="amd64"
  go build -trimpath -o ("bin/linux-amd64/btc5m-" + ($c -replace '_','-')) ("./cmd/"+$c)
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
Get-ChildItem bin/windows-amd64,bin/linux-amd64 -File | Get-FileHash -Algorithm SHA256 | Format-Table -AutoSize
