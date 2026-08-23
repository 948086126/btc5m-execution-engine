$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
Write-Host "== Go unit/integration tests =="
go test -count=1 ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "== Core fail-closed verification =="
go run ./cmd/verify
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "== Local mock-live integration =="
$mockOut = Join-Path "artifacts" "local-mock-live-verify"
if (Test-Path $mockOut) { Remove-Item -Recurse -Force $mockOut }
go run ./cmd/mock_live --duration=6s --out $mockOut
exit $LASTEXITCODE
