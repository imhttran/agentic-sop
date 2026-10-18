package seeded

import "os/exec"

// Insecure is a synthetic SAST fixture. It is never executed.
func Insecure(arg string) error {
	return exec.Command(arg).Run()
}
