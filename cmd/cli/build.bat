@echo off
setlocal
pushd "%~dp0..\.."
call build.bat %*
set "BUILD_RESULT=%errorlevel%"
popd
exit /b %BUILD_RESULT%
