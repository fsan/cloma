package sandbox

// Stop stops a running sandbox.
// A stopped sandbox can be restarted later with exec commands.
func Stop(sandboxName string) error {
	cmd := Command("stop", sandboxName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return CommandError("failed to stop sandbox "+sandboxName, err, output)
	}
	return nil
}

// StopIfExists stops a sandbox if it exists and is running.
// Returns nil if the sandbox doesn't exist or is already stopped.
func StopIfExists(sandboxName string) error {
	running, err := IsRunning(sandboxName)
	if err != nil {
		return err
	}

	if !running {
		return nil
	}

	return Stop(sandboxName)
}
