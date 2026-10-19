Unicode True
!include "MUI2.nsh"
!include "x64.nsh"
!include "WinVer.nsh"
Name "DevShell"
Caption "DevShell ${VERSION} 安装"
OutFile "${OUTPUT}"
InstallDir "$LOCALAPPDATA\Programs\DevShell"
InstallDirRegKey HKCU "Software\DevShell" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
SetCompressorDictSize 16
ShowInstDetails show
ShowUninstDetails show
VIProductVersion "${PACKAGE_VERSION}.${BUILD_REVISION}"
VIAddVersionKey /LANG=2052 "ProductName" "DevShell"
VIAddVersionKey /LANG=2052 "FileDescription" "DevShell 当前用户安装程序"
VIAddVersionKey /LANG=2052 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=2052 "LegalCopyright" "DevShell contributors · MIT"
!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "安装 DevShell"
!define MUI_WELCOMEPAGE_TEXT "将 DevShell 安装到当前用户目录，并创建开始菜单和桌面快捷方式。$\r$\n$\r$\n升级前请先退出 DevShell。原有 data 配置、密钥、字体和背景会保留；此前 DengShell 的安装位置与数据会自动沿用。$\r$\n$\r$\n应用首次启动会检查 WebView2，缺少时提供安装指引。"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${LICENSE_FILE}"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\DevShell.exe"
!define MUI_FINISHPAGE_RUN_TEXT "启动 DevShell"
!insertmacro MUI_PAGE_FINISH
!define MUI_UNCONFIRMPAGE_TEXT_TOP "卸载会移除程序和快捷方式，并保留 data 中的个人配置、密钥、字体和背景。"
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
  SetShellVarContext current
  SetRegView 64
  ReadRegStr $0 HKCU "Software\DevShell" "InstallDir"
  ${If} $0 == ""
    ; First DevShell release: adopt an existing DengShell install directory so
    ; personal data, keys and fonts survive the rename in place.
    ReadRegStr $0 HKCU "Software\DengShell" "InstallDir"
    ${If} $0 != ""
      StrCpy $INSTDIR $0
    ${EndIf}
  ${EndIf}
  ${IfNot} ${RunningX64}
    MessageBox MB_OK|MB_ICONINFORMATION "此安装包适用于 Windows x64。"
    Abort
  ${EndIf}
  ${IfNot} ${AtLeastWin10}
    MessageBox MB_OK|MB_ICONINFORMATION "DevShell 需要 Windows 10 或更新系统。"
    Abort
  ${EndIf}
FunctionEnd

Section "DevShell" SEC_MAIN
  SetShellVarContext current
  SetRegView 64
  SetOutPath "$INSTDIR"
  SetOverwrite on
  ClearErrors
  File /r "${FILESPEC}"
  IfErrors 0 +3
    MessageBox MB_OK|MB_ICONSTOP "程序文件未能完整写入。请退出 DevShell 后重新运行安装器。"
    Abort
  ; Rename migration: drop the previous DengShell program file, shortcuts and
  ; registry entries; personal data under data\ is untouched.
  Delete "$INSTDIR\DengShell.exe"
  Delete "$SMPROGRAMS\DengShell.lnk"
  Delete "$DESKTOP\DengShell.lnk"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DengShell"
  DeleteRegKey HKCU "Software\DengShell"
  CreateDirectory "$INSTDIR\data\support"
  WriteINIStr "$INSTDIR\data\support\installed.ini" "DevShell" "Product" "DevShell"
  WriteINIStr "$INSTDIR\data\support\installed.ini" "DevShell" "InstallDir" "$INSTDIR"
  WriteUninstaller "$INSTDIR\data\support\Uninstall.exe"
  CreateShortcut "$SMPROGRAMS\DevShell.lnk" "$INSTDIR\DevShell.exe" "" "$INSTDIR\DevShell.exe" 0
  CreateShortcut "$DESKTOP\DevShell.lnk" "$INSTDIR\DevShell.exe" "" "$INSTDIR\DevShell.exe" 0
  WriteRegStr HKCU "Software\DevShell" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell" "DisplayName" "DevShell"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell" "Publisher" "DevShell contributors"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell" "DisplayIcon" "$INSTDIR\DevShell.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell" "UninstallString" '"$INSTDIR\data\support\Uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell" "URLInfoAbout" "https://github.com/yuhongwei380/DevShell"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell" "EstimatedSize" ${ESTIMATED_SIZE}
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell" "NoRepair" 1
SectionEnd

Function un.onInit
  SetShellVarContext current
  SetRegView 64
  ; NSIS relaunches the uninstaller from TEMP and restores the ORIGINAL
  ; uninstaller directory in INSTDIR. EXEDIR can point to that temp copy.
  GetFullPathName $0 "$INSTDIR\..\.."
  ReadINIStr $1 "$0\data\support\installed.ini" "DevShell" "Product"
  ReadINIStr $2 "$0\data\support\installed.ini" "DevShell" "InstallDir"
  ${If} $1 != "DevShell"
    MessageBox MB_OK|MB_ICONEXCLAMATION "无法确认 DevShell 安装目录，已停止卸载。请重新安装到原位置后再卸载。"
    Abort
  ${EndIf}
  ${If} $2 != $0
    MessageBox MB_OK|MB_ICONEXCLAMATION "安装目录与记录不一致，已停止卸载。请重新安装到原位置后再卸载。"
    Abort
  ${EndIf}
  StrCpy $INSTDIR $0
FunctionEnd

Section "Uninstall"
  SetShellVarContext current
  SetRegView 64
un_remove_program:
  IfFileExists "$INSTDIR\DevShell.exe" 0 un_program_removed
  ClearErrors
  Delete "$INSTDIR\DevShell.exe"
  IfErrors 0 un_program_removed
  MessageBox MB_RETRYCANCEL|MB_ICONINFORMATION "DevShell 仍在运行或程序文件无法删除。请先退出应用，再重试；取消将保留卸载入口和快捷方式。" IDRETRY un_remove_program
  Abort
un_program_removed:
  ; Clean any leftovers from the pre-rename DengShell install.
  Delete "$INSTDIR\DengShell.exe"
  Delete "$SMPROGRAMS\DengShell.lnk"
  Delete "$DESKTOP\DengShell.lnk"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DengShell"
  DeleteRegKey HKCU "Software\DengShell"
  ReadRegStr $3 HKCU "Software\DevShell" "InstallDir"
  ${If} $3 == $INSTDIR
    Delete "$SMPROGRAMS\DevShell.lnk"
    Delete "$DESKTOP\DevShell.lnk"
  ${EndIf}
  !include "${UNINSTALL_FILES}"
  Delete "$INSTDIR\data\support\installed.ini"
  Delete "$INSTDIR\data\support\Uninstall.exe"
  RMDir "$INSTDIR\data\support"
  RMDir "$INSTDIR\data"
  RMDir "$INSTDIR"
  ${If} $3 == $INSTDIR
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\DevShell"
    DeleteRegKey HKCU "Software\DevShell"
  ${EndIf}
SectionEnd
