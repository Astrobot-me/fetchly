@echo off
rem Fetchly launcher for Windows. Double-click to start.
rem Closing this window quits the app.
setlocal EnableExtensions EnableDelayedExpansion
title Fetchly
chcp 65001 >nul

rem ---- Release settings -------------------------------------------------
rem GitHub repository that hosts the release files (owner/name).
set "REPO=astrobot-me/fetchly"
set "ASSET=fetchly-windows-amd64.exe"
rem ----------------------------------------------------------------------

rem YTGUI_DOWNLOAD_BASE and YTGUI_HOME are for testing only.
if defined YTGUI_DOWNLOAD_BASE (set "BASE=%YTGUI_DOWNLOAD_BASE%") else (set "BASE=https://github.com/%REPO%/releases/latest/download")
if defined YTGUI_HOME (set "DATA=%YTGUI_HOME%") else (set "DATA=%LOCALAPPDATA%\ytgui")
set "APPDIR=%DATA%\app"
set "EXE=%APPDIR%\fetchly.exe"

rem The app is downloaded only when it is missing. An existing copy is
rem never replaced here; updates happen inside the app.
if exist "%EXE%" goto run

echo.
echo   Fetchly is starting for the first time.
echo   Downloading the app (about 8 MB)...
echo.

if "%REPO%"=="OWNER/REPO" if not defined YTGUI_DOWNLOAD_BASE (
  set "MSG=This launcher has not been set up with a download address yet."
  goto fail
)
if not exist "%APPDIR%" mkdir "%APPDIR%" 2>nul
if not exist "%APPDIR%" (
  set "MSG=Could not create the folder %APPDIR%"
  goto fail
)

set "TMPEXE=%APPDIR%\fetchly.download"
set "SUMS=%APPDIR%\checksums.txt"
del /f /q "%TMPEXE%" "%SUMS%" >nul 2>&1

rem curl.exe ships with Windows 10 (1803) and later.
where curl.exe >nul 2>&1
if errorlevel 1 (
  set "MSG=This version of Windows is too old. Windows 10 or 11 is needed."
  goto fail
)
curl.exe -fsSL --retry 3 -o "%SUMS%" "%BASE%/checksums.txt"
if errorlevel 1 goto fail_download
curl.exe -fL --retry 3 --progress-bar -o "%TMPEXE%" "%BASE%/%ASSET%"
if errorlevel 1 goto fail_download

rem Check the download against the published SHA-256 before running it.
set "WANT="
for /f "usebackq tokens=1,2" %%a in ("%SUMS%") do (
  set "NAME=%%b"
  if "!NAME:~0,1!"=="*" set "NAME=!NAME:~1!"
  if /i "!NAME!"=="%ASSET%" set "WANT=%%a"
)
set "GOT="
for /f "skip=1 delims=" %%h in ('certutil -hashfile "%TMPEXE%" SHA256') do (
  if not defined GOT set "GOT=%%h"
)
if not defined WANT goto fail_verify
if not defined GOT goto fail_verify
rem Older certutil versions print the hash with spaces.
set "GOT=!GOT: =!"
if /i not "!GOT!"=="!WANT!" goto fail_verify

move /y "%TMPEXE%" "%EXE%" >nul
if errorlevel 1 (
  set "MSG=Could not install the app into %APPDIR%"
  goto fail
)
del /f /q "%SUMS%" >nul 2>&1

:run
"%EXE%" %*
set "CODE=%ERRORLEVEL%"
if not "%CODE%"=="0" (
  echo.
  echo   Fetchly stopped because of a problem. See the message above.
  echo.
  pause
)
exit /b %CODE%

:fail_download
del /f /q "%TMPEXE%" "%SUMS%" >nul 2>&1
set "MSG=Could not download the app. Check your internet connection and try again."
goto fail

:fail_verify
del /f /q "%TMPEXE%" "%SUMS%" >nul 2>&1
set "MSG=The download could not be verified, so it was not installed. Please try again later."
goto fail

:fail
echo.
echo   !MSG!
echo.
pause
exit /b 1
