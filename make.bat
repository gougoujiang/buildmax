@echo off
setlocal

rem API key for OpenRouter (used by make test)
set "OPENROUTER_API_KEY=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"

if "%~1"=="" goto usage
if /i "%~1"=="build" goto build
if /i "%~1"=="test" goto test
goto usage

:build
go build -o buildmax.exe ./cmd/buildmax
exit /b %errorlevel%

:test
if not exist buildmax.exe (
  echo building first...
  call go build -o buildmax.exe ./cmd/buildmax
  if errorlevel 1 exit /b 1
)
buildmax.exe -p "what can you do"
exit /b %errorlevel%

:usage
echo Usage: make.bat ^<command^>
echo   build   Build buildmax.exe
echo   test    Quick test using API key from this script
exit /b 0
