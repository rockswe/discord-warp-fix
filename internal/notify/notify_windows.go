package notify

import (
	"os"
	"os/exec"
	"syscall"
)

// UNTESTED on real Windows. Uses the toast API that ships with Windows
// PowerShell 5.1, borrowing PowerShell's app id so no registration is needed.
const toastScript = `[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$x = $t.GetElementsByTagName('text')
$x.Item(0).AppendChild($t.CreateTextNode($env:DWF_TITLE)) > $null
$x.Item(1).AppendChild($t.CreateTextNode($env:DWF_MSG)) > $null
$id = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($id).Show([Windows.UI.Notifications.ToastNotification]::new($t))`

func send(msg string) error {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", toastScript)
	cmd.Env = append(os.Environ(), "DWF_TITLE="+Title, "DWF_MSG="+msg)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, HideWindow: true}
	return cmd.Run()
}
