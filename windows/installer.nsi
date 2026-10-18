!include "MUI2.nsh"

!ifndef REPOROOT
  !define REPOROOT "."
!endif

Name "Timetable"
OutFile "${REPOROOT}\build\timetable-setup-x64.exe"
InstallDir "$PROGRAMFILES64\Timetable"
RequestExecutionLevel admin
SetCompressor /SOLID lzma

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Russian"

Section "Install"
  SetOutPath "$INSTDIR"
  File "${REPOROOT}\windows\staging\timetable.exe"
  File "${REPOROOT}\windows\staging\ortools_csat.dll"
  File /nonfatal "${REPOROOT}\windows\staging\ortools.dll"
  File /nonfatal "${REPOROOT}\windows\staging\vcruntime140.dll"
  File /nonfatal "${REPOROOT}\windows\staging\vcruntime140_1.dll"
  File /nonfatal "${REPOROOT}\windows\staging\msvcp140.dll"
  File /nonfatal "${REPOROOT}\windows\staging\msvcp140_1.dll"
  File /nonfatal "${REPOROOT}\windows\staging\msvcp140_codecvt_ids.dll"
  File "${REPOROOT}\windows\staging\MicrosoftEdgeWebView2RuntimeInstallerX64.exe"

  DetailPrint "Установка WebView2 Runtime (офлайн)..."
  ExecWait '"$INSTDIR\MicrosoftEdgeWebView2RuntimeInstallerX64.exe" /silent /install'

  CreateShortcut "$SMPROGRAMS\Timetable.lnk" "$INSTDIR\timetable.exe"
  CreateShortcut "$DESKTOP\Timetable.lnk" "$INSTDIR\timetable.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
  Delete "$SMPROGRAMS\Timetable.lnk"
  Delete "$DESKTOP\Timetable.lnk"
  Delete "$INSTDIR\timetable.exe"
  Delete "$INSTDIR\ortools.dll"
  Delete "$INSTDIR\ortools_csat.dll"
  Delete "$INSTDIR\vcruntime140.dll"
  Delete "$INSTDIR\vcruntime140_1.dll"
  Delete "$INSTDIR\msvcp140.dll"
  Delete "$INSTDIR\msvcp140_1.dll"
  Delete "$INSTDIR\msvcp140_codecvt_ids.dll"
  Delete "$INSTDIR\MicrosoftEdgeWebView2RuntimeInstallerX64.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
