@echo off
setlocal

rem Load env from .env if present (copy .env.example to .env and fill in keys)
call "%~dp0loadenv.bat"

if "%~1"=="" goto usage
if /i "%~1"=="build" goto build
if /i "%~1"=="test" goto test
if /i "%~1"=="smoke" goto smoke
goto usage

:build
go build -o buildmax.exe ./cmd/buildmax
exit /b %errorlevel%

:test
if not exist testing-sandbox mkdir testing-sandbox
set "BUILDMAX_HOME=%CD%\testing-sandbox"
go test ./...
exit /b %errorlevel%

:smoke
if not exist testing-sandbox mkdir testing-sandbox
set "BUILDMAX_HOME=%CD%\testing-sandbox"
go build -o buildmax.exe ./cmd/buildmax
if errorlevel 1 exit /b 1
rem reset log level to debug
set "BUILDMAX_LOG_LEVEL=debug"
buildmax.exe -p "/smoke 0"
exit /b %errorlevel%

:usage
echo Usage: make.bat ^<command^>
echo   build   Build buildmax.exe
echo   test    Run go test with testing-sandbox as data dir
echo   smoke   Smoke test: build and run with /smoke 0, BUILDMAX_HOME=testing-sandbox
exit /b 0
