@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

set "APP_NAME=NexFatal"
set "MODULE_PATH=github.com/LangTu2025/NexFatal"
set "MAIN_PKG=./cmd/cli"
set "DIST_DIR=%APP_NAME%_Storage\dist"
set "NO_PAUSE="
set "CHECK_ONLY="

:parse_args
if "%~1"=="" goto args_done
if /i "%~1"=="--no-pause" set "NO_PAUSE=1"
if /i "%~1"=="--check" set "CHECK_ONLY=1"
shift
goto parse_args
:args_done

set "APP_VERSION="
if exist version (
    set /p APP_VERSION=<version
)

call :detect_git_hash

set "LDFLAGS=-s -w -checklinkname=0"
if defined APP_VERSION set "LDFLAGS=!LDFLAGS! -X %MODULE_PATH%/constants.Version=!APP_VERSION!"
if defined GIT_HASH set "LDFLAGS=!LDFLAGS! -X %MODULE_PATH%/constants.CommitHash=!GIT_HASH!"

echo =========================================
echo   %APP_NAME% - 多平台编译
echo =========================================
if defined APP_VERSION (
    echo 版本: !APP_VERSION!
) else (
    echo 版本: 使用源码默认值
)
if defined GIT_HASH (
    echo 提交号: !GIT_HASH!
) else (
    echo 提交号: ??? ^(未检测到 git 仓库^)
)
echo 输出目录: %DIST_DIR%
echo.

if defined CHECK_ONLY (
    echo 检查完成，未执行编译命令
    call :maybe_pause
    exit /b 0
)

if exist "%DIST_DIR%" rmdir /s /q "%DIST_DIR%"
mkdir "%DIST_DIR%" 2>nul
if errorlevel 1 (
    echo 错误: 无法创建输出目录 %DIST_DIR%
    call :maybe_pause
    exit /b 1
)

set "FAILED=0"

call :build_target 1 7 android arm64 ""
call :build_target 2 7 linux amd64 ""
call :build_target 3 7 linux arm64 ""
call :build_target 4 7 windows amd64 ".exe"
call :build_target 5 7 windows arm64 ".exe"
call :build_target 6 7 darwin amd64 ""
call :build_target 7 7 darwin arm64 ""

echo.
echo =========================================
if !FAILED! equ 0 (
    echo 编译完成，全部目标成功
) else (
    echo 编译完成，失败目标数: !FAILED!
)
echo =========================================
dir "%DIST_DIR%"
echo.

if !FAILED! equ 0 (
    call :maybe_pause
    exit /b 0
) else (
    call :maybe_pause
    exit /b 1
)

:build_target
set "TARGET_INDEX=%~1"
set "TARGET_TOTAL=%~2"
set "TARGET_GOOS=%~3"
set "TARGET_GOARCH=%~4"
set "TARGET_EXT=%~5"
set "OUTPUT=%DIST_DIR%\%APP_NAME%_!TARGET_GOOS!_!TARGET_GOARCH!!TARGET_EXT!"

echo [!TARGET_INDEX!/!TARGET_TOTAL!] 编译 !TARGET_GOOS!/!TARGET_GOARCH!...
set "GOOS=!TARGET_GOOS!"
set "GOARCH=!TARGET_GOARCH!"
set "CGO_ENABLED=0"

go build -trimpath -ldflags="!LDFLAGS!" -o "!OUTPUT!" %MAIN_PKG%
if errorlevel 1 (
    echo   编译失败
    set /a FAILED+=1
    echo.
    exit /b 0
)

echo   编译成功
echo.
exit /b 0

:maybe_pause
if not defined NO_PAUSE pause
exit /b 0

:detect_git_hash
set "GIT_HASH="
for /f "delims=" %%i in ('git -C "%CD%" rev-parse --short=7 HEAD 2^>nul') do (
    set "GIT_HASH=%%i"
    exit /b 0
)

set "GIT_DIR=%CD%\.git"
if not exist "%GIT_DIR%\HEAD" (
    if exist "%CD%\.git" (
        set "GIT_FILE_LINE="
        for /f "usebackq delims=" %%i in ("%CD%\.git") do (
            if not defined GIT_FILE_LINE set "GIT_FILE_LINE=%%i"
        )
        if "!GIT_FILE_LINE:~0,8!"=="gitdir: " set "GIT_DIR=!GIT_FILE_LINE:~8!"
    )
)

if not exist "%GIT_DIR%\HEAD" exit /b 0

set "HEAD_LINE="
for /f "usebackq delims=" %%i in ("%GIT_DIR%\HEAD") do (
    if not defined HEAD_LINE set "HEAD_LINE=%%i"
)
if not defined HEAD_LINE exit /b 0

set "FULL_HASH="
if "!HEAD_LINE:~0,5!"=="ref: " (
    set "REF_NAME=!HEAD_LINE:~5!"
    if exist "%GIT_DIR%\!REF_NAME!" (
        for /f "usebackq delims=" %%i in ("%GIT_DIR%\!REF_NAME!") do (
            if not defined FULL_HASH set "FULL_HASH=%%i"
        )
    ) else if exist "%GIT_DIR%\packed-refs" (
        for /f "usebackq tokens=1,2" %%a in ("%GIT_DIR%\packed-refs") do (
            if "%%b"=="!REF_NAME!" if not defined FULL_HASH set "FULL_HASH=%%a"
        )
    )
) else (
    set "FULL_HASH=!HEAD_LINE!"
)

if defined FULL_HASH set "GIT_HASH=!FULL_HASH:~0,7!"
exit /b 0
