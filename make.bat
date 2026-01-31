@echo off
setlocal

rem API key for OpenRouter (used by make run)
set "OPENROUTER_API_KEY=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"

if "%~1"=="" goto usage
if /i "%~1"=="build" goto build
if /i "%~1"=="test" goto test
if /i "%~1"=="run" goto run
goto usage

:build
go build -o buildmax.exe ./cmd/buildmax
exit /b %errorlevel%

:test
if not exist testing-sandbox mkdir testing-sandbox
set "HOME_DIR=%CD%\testing-sandbox"
go test ./...
exit /b %errorlevel%

:run
if not exist testing-sandbox mkdir testing-sandbox
set "HOME_DIR=%CD%\testing-sandbox"
go build -o buildmax.exe ./cmd/buildmax
if errorlevel 1 exit /b 1
buildmax.exe -p "what can you do"
exit /b %errorlevel%

:usage
echo Usage: make.bat ^<command^>
echo   build   Build buildmax.exe
echo   test    Run go test with testing-sandbox as data dir
echo   run     Manual test run: build and run with -p, HOME_DIR=testing-sandbox
exit /b 0
