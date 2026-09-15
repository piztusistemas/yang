//go:build !windows

package main

import "os/exec"

// ocultarConsola só ten sentido en Windows - ver execwindow_windows.go. En
// Linux/macOS un proceso fillo nunca abre a súa propia xanela de terminal
// (herda os descritores do pai, xa redirixidos a pipes/buffers).
func ocultarConsola(cmd *exec.Cmd) {}
