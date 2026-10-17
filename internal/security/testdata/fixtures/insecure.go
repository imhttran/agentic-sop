package fixture

// Insecure is a synthetic SAST fixture. It is never executed by a test.
func Insecure(input string) {
	_ = RunCommand(input)
}

// RunCommand exists so the fixture resembles a supported insecure-code shape.
func RunCommand(string) error { return nil }
