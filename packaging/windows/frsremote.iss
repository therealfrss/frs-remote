; FRS Remote — installer Windows (PC-ul de acasă)
; Include Sunshine (serverul de stream) și agentul FRS Remote. Un singur installer.

#ifndef AppVersion
  #define AppVersion "0.1.0"
#endif

[Setup]
AppId={{6F0B7C1E-3E7A-4C55-9D7B-2F5A1E0C0D01}
AppName=FRS Remote
AppVersion={#AppVersion}
AppPublisher=Freesh Media Products
DefaultDirName={autopf}\FRS Remote
DefaultGroupName=FRS Remote
DisableProgramGroupPage=yes
OutputDir=..\..\dist
OutputBaseFilename=FRS-Remote-Setup-{#AppVersion}
SetupIconFile=..\..\assets\app.ico
UninstallDisplayIcon={app}\frsremote.exe
Compression=lzma2
SolidCompression=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern

[Files]
Source: "..\..\build\windows\frsremote.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\build\windows\sunshine.msi"; DestDir: "{tmp}"; Flags: deleteafterinstall

[Icons]
Name: "{group}\FRS Remote"; Filename: "{app}\frsremote.exe"
Name: "{autostartup}\FRS Remote"; Filename: "{app}\frsremote.exe"

[Run]
; 1. Sunshine (MSI), instalat silențios: serviciu Windows + reguli firewall
Filename: "{sys}\msiexec.exe"; Parameters: "/i ""{tmp}\sunshine.msi"" /qn /norestart"; StatusMsg: "Instalez serverul de stream..."; Flags: waituntilterminated
; 2. Date de acces pentru Sunshine, generate automat
Filename: "{app}\frsremote.exe"; Parameters: "setup-sunshine"; StatusMsg: "Configurez serverul de stream..."; Flags: runhidden waituntilterminated
; 3. Firewall pentru conexiunea directă Mac <-> PC
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""FRS Remote"""; Flags: runhidden waituntilterminated
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""FRS Remote"" dir=in action=allow program=""{app}\frsremote.exe"" enable=yes"; Flags: runhidden waituntilterminated
; 4. Pornește agentul (ca utilizatorul normal, nu ca admin)
Filename: "{app}\frsremote.exe"; Description: "Pornește FRS Remote"; Flags: postinstall nowait runasoriginaluser

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM frsremote.exe"; Flags: runhidden; RunOnceId: "KillAgent"
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""FRS Remote"""; Flags: runhidden; RunOnceId: "DelFw"

[Code]
procedure CurStepChanged(CurStep: TSetupStep);
var
  R: Integer;
begin
  if CurStep = ssInstall then
    Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM frsremote.exe', '', SW_HIDE, ewWaitUntilTerminated, R);
end;
