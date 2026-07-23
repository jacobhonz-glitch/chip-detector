@echo off
title Chip Detector 安装
echo ================================
echo   Chip Detector v0.2.0 安装
echo ================================
echo.
set INSTALLDIR=%USERPROFILE%\ChipDetector
mkdir "%INSTALLDIR%" 2>nul
copy /Y "%~dp0ChipDetector.exe" "%INSTALLDIR%\ChipDetector.exe" >nul
xcopy /Y /E "%~dp0web\*" "%INSTALLDIR%\web\" >nul 2>nul

:: 创建桌面快捷方式
powershell -Command "$s=(New-Object -COM WScript.Shell).CreateShortcut('%USERPROFILE%\Desktop\ChipDetector.lnk');$s.TargetPath='%INSTALLDIR%\ChipDetector.exe';$s.WorkingDirectory='%INSTALLDIR%';$s.Save()"

echo.
echo 安装完成！
echo 桌面已创建快捷方式: ChipDetector
echo.
start "" "%INSTALLDIR%\ChipDetector.exe"
start http://localhost:19527
pause
