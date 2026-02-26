@echo off
REM Copy Trading Bot - 24/7 Runner
REM This script keeps the bot running continuously

set LOGFILE=bot_output.log
set RESTART_DELAY=30

:loop
echo [%date% %time%] Starting Copy Trading Bot... >> %LOGFILE%
echo [%date% %time%] Starting Copy Trading Bot...

REM Run the bot with YES confirmation piped in
echo YES | live-trading.exe >> %LOGFILE% 2>&1

echo [%date% %time%] Bot stopped. Restarting in %RESTART_DELAY% seconds... >> %LOGFILE%
echo [%date% %time%] Bot stopped. Restarting in %RESTART_DELAY% seconds...

timeout /t %RESTART_DELAY% /nobreak > nul
goto loop
