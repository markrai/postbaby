@echo off
setlocal

REM Run from repository root even if launched elsewhere
pushd "%~dp0"
set "POSTBABY_PORT=8096"
docker compose up --build -d
set "EXIT_CODE=%ERRORLEVEL%"
popd

exit /b %EXIT_CODE%
