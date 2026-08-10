package cli

// Windows does not permit opening a directory for FlushFileBuffers. The
// temporary config file itself is synced before the atomic install or replace.
func syncConfigDirectory(string) error {
	return nil
}
