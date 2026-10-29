@echo off
chcp 65001 >nul

echo.
echo 执行 git add . ...
git add .

echo.
set /p msg="请输入提交信息: "
if "%msg%"=="" set msg="update"

echo.
echo 执行 git commit -m "%msg%" ...
git commit -m "%msg%"

echo.
echo 执行 git push ...
git push

echo.
echo 完成！
pause