package notify

import "os/exec"

func send(msg string) error {
	return exec.Command("osascript",
		"-e", "on run argv",
		"-e", `display notification (item 1 of argv) with title "`+Title+`"`,
		"-e", "end run", msg).Run()
}
