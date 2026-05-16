@echo off
setlocal EnableExtensions
cd /d "%~dp0"

if exist "src\script.js" (
  echo [tests] Maintainer-only source present
  echo [tests] Node syntax check: src\script.js
  node --check "src\script.js"
  if errorlevel 1 (
    echo [tests] FAILED: node --check
    exit /b 1
  )

  echo [tests] Verify generated public artifact
  call npm run verify:public-js
  if errorlevel 1 (
    echo [tests] FAILED: verify:public-js
    exit /b 1
  )
) else (
  echo [tests] src\script.js not present; skipping maintainer-only artifact verification
)

echo [tests] Node syntax check: js\script.js
node --check "js\script.js"
if errorlevel 1 (
  echo [tests] FAILED: node --check generated artifact
  exit /b 1
)

echo [tests] Go tests: postbaby-backend
pushd "postbaby-backend" || exit /b 1
go test ./...
set "GOEXIT=%ERRORLEVEL%"
popd
if not "%GOEXIT%"=="0" (
  echo [tests] FAILED: go test
  exit /b %GOEXIT%
)

echo [tests] OK
exit /b 0
