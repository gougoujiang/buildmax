@echo off
rem Shim for the task runner. Every task lives in cmd/mk (Go) so that macOS,
rem Linux, and Windows contributors run the same code; ./make is the same shim
rem for bash. Run `make.bat help` for the command list.
cd /d "%~dp0"
go run ./cmd/mk %*
exit /b %errorlevel%
