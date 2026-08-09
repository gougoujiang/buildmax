@echo off
rem Load .env (KEY=value, one per line; lines starting with # are skipped).
rem .env is read from the same directory as this script (project root).
rem In terminal: cd to project root, then  call loadenv.bat
pushd "%~dp0"
if not exist .env goto done
for /f "usebackq eol=# tokens=1* delims==" %%a in (".env") do set "%%a=%%b"
:done
popd
exit /b 0
