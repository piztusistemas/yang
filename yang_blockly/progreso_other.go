//go:build !darwin

package main

// mbDescargaEnCurso: fóra de macOS non se sabe canto se leva descargado.
//
// apt e winget si escriben progreso na súa saída estándar (que Yang xa recolle
// no log), así que non fai falla ir buscalo a ningures - o problema é
// específico de Homebrew, que cala por completo cando a saída non é un
// terminal. Ver progreso_darwin.go.
//
// O informe de tempo transcorrido (informarProgresoInstalacion, installer.go)
// segue funcionando en todas as plataformas: un `apt-get install` de texlive
// tamén pode pasar minutos sen dicir nada.
func mbDescargaEnCurso() int64 { return 0 }
