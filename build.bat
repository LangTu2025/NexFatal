@echo off
setlocal
pushd "%~dp0"
if errorlevel 1 exit /b 1

where go >nul 2>nul
if errorlevel 1 (
    echo Error: Go is not available in PATH.
    goto failed
)

if not exist "NexFatal_Storage\dist\" mkdir "NexFatal_Storage\dist"
if errorlevel 1 goto failed

set "CGO_ENABLED=0"
set "GOARCH=amd64"

echo [1/2] Building Linux/amd64...
set "GOOS=linux"
go build -o "NexFatal_Storage\dist\NexFatal_linux_amd64" ./cmd/cli
if errorlevel 1 goto failed

echo [2/2] Building Windows/amd64...
set "GOOS=windows"
go build -o "NexFatal_Storage\dist\NexFatal_windows_amd64.exe" ./cmd/cli
if errorlevel 1 goto failed

echo Build complete. Output: NexFatal_Storage\dist
popd
exit /b 0

:failed
echo Build failed.
popd
exit /b 1
