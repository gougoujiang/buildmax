@echo off
rem Shim for the task runner. Every task lives in tools/mk (Go) so that macOS,
rem Linux, and Windows contributors run the same code; ./make is the same shim
rem for bash. Run `make.bat help` for the command list.
cd /d "%~dp0"

rem `make.bat doctor` cannot be the thing that reports a missing Go: the task
rem runner is Go, so without this guard a new contributor's first command fails
rem with cmd.exe's own "not recognized" error. Go is the one prerequisite
rem nothing in this repository can install for you.
where go >nul 2>nul
if not errorlevel 1 goto run
rem Set outside a parenthesised block so %GOWANT% expands after the loop rather
rem than when the block was parsed.
for /f "tokens=2" %%v in ('findstr /b /c:"go " go.mod') do set "GOWANT=%%v"
echo BuildMax needs Go %GOWANT%, and 'go' is not on your PATH. 1>&2
echo Install it from https://go.dev/dl/, or run: winget install GoLang.Go 1>&2
exit /b 1

:run
go run ./tools/mk %*
exit /b %errorlevel%
