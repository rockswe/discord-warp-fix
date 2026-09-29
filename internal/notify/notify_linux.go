package notify

import "os/exec"

func send(msg string) error {
	p, err := exec.LookPath("notify-send")
	if err != nil {
		return err
	}
	return exec.Command(p, "--app-name="+Title, Title, msg).Run()
}
