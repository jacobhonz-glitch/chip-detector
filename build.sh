#!/bin/bash
# chip-detector/build.sh
# 一键编译 mac + windows 安装包

set -e

VERSION="0.2.0"
APP_NAME="ChipDetector"
BUILD_DIR="dist"
MAC_APP="${APP_NAME}.app"
DMG_NAME="${APP_NAME}-${VERSION}.dmg"

echo "🔨 开始构建 ${APP_NAME} v${VERSION}"
rm -rf ${BUILD_DIR}
mkdir -p ${BUILD_DIR}

# ========== macOS ==========
echo "🍎 编译 macOS 版本..."
GOOS=darwin GOARCH=amd64 go build -o ${BUILD_DIR}/${APP_NAME}-mac main.go
GOOS=darwin GOARCH=arm64 go build -o ${BUILD_DIR}/${APP_NAME}-mac-arm64 main.go

# 创建通用二进制
lipo -create ${BUILD_DIR}/${APP_NAME}-mac ${BUILD_DIR}/${APP_NAME}-mac-arm64 -output ${BUILD_DIR}/${APP_NAME}
rm ${BUILD_DIR}/${APP_NAME}-mac ${BUILD_DIR}/${APP_NAME}-mac-arm64

# 打包成 .app
mkdir -p ${BUILD_DIR}/${MAC_APP}/Contents/MacOS
mkdir -p ${BUILD_DIR}/${MAC_APP}/Contents/Resources
cp ${BUILD_DIR}/${APP_NAME} ${BUILD_DIR}/${MAC_APP}/Contents/MacOS/
cp web/* ${BUILD_DIR}/${MAC_APP}/Contents/Resources/

cat > ${BUILD_DIR}/${MAC_APP}/Contents/Info.plist << 'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>ChipDetector</string>
    <key>CFBundleDisplayName</key>
    <string>Chip Detector</string>
    <key>CFBundleIdentifier</key>
    <string>com.chipdetector.app</string>
    <key>CFBundleVersion</key>
    <string>0.2.0</string>
    <key>CFBundleExecutable</key>
    <string>ChipDetector</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSUIElement</key>
    <true/>
</dict>
</plist>
PLIST

# 创建启动脚本
cat > ${BUILD_DIR}/${MAC_APP}/Contents/MacOS/launcher << 'SH'
#!/bin/bash
DIR="$(cd "$(dirname "$0")" && pwd)"
open http://localhost:19527
exec "$DIR/ChipDetector"
SH
chmod +x ${BUILD_DIR}/${MAC_APP}/Contents/MacOS/launcher

# 修改 Info.plist 指向 launcher
sed -i '' 's/ChipDetector<\/key>/ChipDetector<\/key>\n    <key>CFBundleExecutable<\/key>\n    <string>launcher<\/string>/' ${BUILD_DIR}/${MAC_APP}/Contents/Info.plist 2>/dev/null || true

# 打包 dmg
echo "📦 打包 macOS DMG..."
hdiutil create -volname "${APP_NAME}" -srcfolder ${BUILD_DIR}/${MAC_APP} -ov -format UDZO ${BUILD_DIR}/${DMG_NAME} 2>/dev/null || true
rm -rf ${BUILD_DIR}/${MAC_APP} ${BUILD_DIR}/${APP_NAME}

# ========== Windows ==========
echo "🪟 编译 Windows 版本..."
GOOS=windows GOARCH=amd64 go build -o ${BUILD_DIR}/${APP_NAME}.exe main.go

# 创建 Windows 自解压安装脚本（NSIS 简化版）
cat > ${BUILD_DIR}/install.bat << 'BAT'
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
BAT

# 创建 web 目录的副本给 Windows
mkdir -p ${BUILD_DIR}/web
cp web/* ${BUILD_DIR}/web/

echo ""
echo "✅ 构建完成！"
echo ""
echo "📦 输出文件:"
ls -lh ${BUILD_DIR}/*.dmg ${BUILD_DIR}/*.exe 2>/dev/null || true
echo ""
echo "macOS:  ${BUILD_DIR}/${DMG_NAME}"
echo "Windows: ${BUILD_DIR}/${APP_NAME}.exe + install.bat"