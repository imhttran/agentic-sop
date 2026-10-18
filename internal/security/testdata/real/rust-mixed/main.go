package mixed

import "os/exec"

// Mixed is a synthetic Go half of the mixed-language fixture. It is never
// executed.
func Mixed(arg string) error {
	return exec.Command(arg).Run()
}
