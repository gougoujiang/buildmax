# Build BuildMax for current OS
Set-Location $PSScriptRoot\..
go build -o buildmax.exe ./cmd/buildmax
