@echo off
title Copy Trading Bot 24/7
cd /d "%~dp0"

echo ========================================
echo   Copy Trading Bot - 24/7 Mode
echo ========================================
echo.
echo Press Ctrl+C to stop the bot.
echo Logs are saved to: bot_logs\
echo.

if not exist bot_logs mkdir bot_logs

:restart
echo [%date% %time%] Starting bot...

REM Generate log filename with date
for /f "tokens=1-3 delims=/ " %%a in ('date /t') do set datestr=%%c-%%a-%%b
set LOGFILE=bot_logs\bot_%datestr%.log

echo [%date% %time%] === Bot Started === >> "%LOGFILE%"

REM Run the bot with auto-confirm
live-trading.exe --auto-confirm >> "%LOGFILE%" 2>&1

echo.
echo [%date% %time%] Bot stopped. Restarting in 30 seconds...
echo [%date% %time%] Bot stopped >> "%LOGFILE%"

timeout /t 30 /nobreak
goto restart
