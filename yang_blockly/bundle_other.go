//go:build !darwin

package main

import "os/exec"

// comandoRelanzar: fóra de macOS o executable é un binario solto e chega con
// executalo. Ver bundle_darwin.go, onde hai que pasar polo .app.
func comandoRelanzar(exe string) *exec.Cmd { return exec.Command(exe) }

// resinarBundleMacOS: só macOS ten bundles .app con sinatura de código que
// haxa que refacer despois de substituír o executable. En Linux un binario
// solto substituído funciona tal cal. Ver bundle_darwin.go.
func resinarBundleMacOS(exe string) error { return nil }
