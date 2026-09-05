@echo off
setlocal EnableExtensions EnableDelayedExpansion
chcp 65001 >nul

set "EVIDENCE=%TEMP%\f1-control-results.txt"
(
  echo F1-44 native Windows evidence
  echo =============================
  ver
  echo PROCESSOR_ARCHITECTURE=%PROCESSOR_ARCHITECTURE%
  powershell.exe -NoProfile -NonInteractive -Command "$os = Get-CimInstance Win32_OperatingSystem; [pscustomobject]@{ Caption = $os.Caption; Version = $os.Version; BuildNumber = $os.BuildNumber; OSArchitecture = $os.OSArchitecture } | Format-List"
  echo.
  powershell.exe -NoProfile -NonInteractive -Command "Get-FileHash -Algorithm SHA256 '%~dp0f1-control-probe.exe' | Format-List Algorithm,Hash"
  powershell.exe -NoProfile -NonInteractive -Command "Get-FileHash -Algorithm SHA256 '%~dp0f1-control.test.exe' | Format-List Algorithm,Hash"
  echo.
  set "F1_CONTROL_PROBE_BINARY=%~dp0f1-control-probe.exe"
  "%~dp0f1-control.test.exe" -test.v
  echo TEST_EXIT_CODE=!ERRORLEVEL!
) > "%EVIDENCE%" 2>&1

start "" notepad.exe "%EVIDENCE%"
endlocal
