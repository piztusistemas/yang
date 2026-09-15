//go:build !windows

package cas

import "os"

// maximaEnv non fai falta tocalo fóra de Windows - ver tempenv_windows.go
// para o motivo (SBCL/Maxima fallando en escribir o seu .gnuplot temporal
// cando o TEMP do usuario ten un carácter non-ASCII, algo específico de
// como Windows deriva %TEMP% do nome de usuario).
func maximaEnv() []string {
	return os.Environ()
}
