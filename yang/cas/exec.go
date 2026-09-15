package cas

import "os/exec"

// commandExists mirrors the helper of the same name in the main package
// (installer.go) - duplicated rather than imported, since cas is imported
// by main and importing back would be circular.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
