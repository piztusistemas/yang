//go:build !windows

package cas

import "os/exec"

// ocultarConsola só ten sentido en Windows - ver execwindow_windows.go.
func ocultarConsola(cmd *exec.Cmd) {}
