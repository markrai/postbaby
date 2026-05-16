@echo off
setlocal EnableExtensions
cd /d "%~dp0"

REM Portainer "Upload image" / Images -> Import expects: docker save (image layers), NOT a tar of source files.

echo.
echo [1/2] Building Docker image...
docker build -t postbaby:local .
if errorlevel 1 (
  echo.
  echo Docker build failed. Start Docker Desktop ^(or your engine^) and retry.
  exit /b 1
)

set "OUT=%~dp0postbaby.tar"
if exist "%OUT%" del /f /q "%OUT%"

echo.
echo [2/2] Exporting image for Portainer ^(docker save^)...
echo       %OUT%
echo.

docker save -o "%OUT%" postbaby:local
if errorlevel 1 (
  echo docker save failed.
  exit /b 1
)

echo.
echo Done. In Portainer:
echo   1. Images -^> Import -^> upload this file: %OUT%
echo   2. Stacks -^> use docker-compose.portainer.yml ^(image postbaby:local, no build on the NAS^)
echo.
echo Note: Build on the same CPU family you deploy to ^(e.g. amd64 vs arm64^) or use docker buildx.
echo.
endlocal
exit /b 0
