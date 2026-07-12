#ifndef AppVersion
  #error AppVersion must be supplied with /DAppVersion=x.y.z
#endif
#ifndef BundleDir
  #error BundleDir must point to the verified Flutter release bundle
#endif

#define AppGuid "{28D66AE0-9468-4D30-B9D2-8D5680266CB4}"
#define WebView2Client "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

[Setup]
AppId={{#AppGuid}
AppName=Agent Card Container
AppVersion={#AppVersion}
AppPublisher=Agent Card Container
DefaultDirName={localappdata}\Programs\Agent Card Container
DefaultGroupName=Agent Card Container
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=lowest
OutputBaseFilename=AgentCardContainer-{#AppVersion}-windows-x64
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\agent_card_desktop.exe
VersionInfoVersion={#AppVersion}
CloseApplications=yes
RestartApplications=no

[Files]
Source: "{#BundleDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs; Excludes: "*.db,*.sqlite,*.sqlite3,*.log,*.env,*.key,*.pem"

[Icons]
Name: "{group}\Agent Card Container"; Filename: "{app}\agent_card_desktop.exe"
Name: "{autodesktop}\Agent Card Container"; Filename: "{app}\agent_card_desktop.exe"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional shortcuts:"; Flags: unchecked

[Run]
Filename: "{app}\agent_card_desktop.exe"; Description: "Launch Agent Card Container"; Flags: nowait postinstall skipifsilent

[Code]
function WebView2Version(): String;
begin
  Result := '';
  if not RegQueryStringValue(HKCU32, '{#WebView2Client}', 'pv', Result) then
    RegQueryStringValue(HKLM32, '{#WebView2Client}', 'pv', Result);
end;

function InitializeSetup(): Boolean;
var
  Version: String;
  ErrorCode: Integer;
begin
  Version := WebView2Version();
  Result := (Version <> '') and (Version <> '0.0.0.0');
  if not Result then
  begin
    MsgBox(
      'Microsoft Edge WebView2 Evergreen Runtime is required. The official download page will now open; install the x64 Evergreen Runtime, then run this installer again.',
      mbError,
      MB_OK
    );
    ShellExec(
      'open',
      'https://developer.microsoft.com/microsoft-edge/webview2/#download-section',
      '',
      '',
      SW_SHOWNORMAL,
      ewNoWait,
      ErrorCode
    );
  end;
end;
