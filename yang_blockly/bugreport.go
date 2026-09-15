package main

import (
	"fmt"
	"os"
	"strings"
)

// bugreport.go implementa "🐛 Informar deste erro" (frontend/src/
// bug-report.js): cando falla unha compilación (Xerar/PDF), o profesorado
// pode pedirlle á IA un DIAGNÓSTICO + suxestión de arranxo do propio
// YANG (non do seu exercicio) pensado para o DESENVOLVEDOR - a diferenza
// do asistente normal (AsistenteIA, app.go), que axuda a corrixir O
// DOCUMENTO da profesora.
//
// IMPORTANTE (decisión de deseño explícita, non un descoido): isto NUNCA
// toca nin recompila código de Yang. A IA só devolve TEXTO (un informe en
// Markdown, ver systemPromptInformeErro en ia_client.go) que se garda nun
// ficheiro para que o desenvolvedor o revise e o aplique á man, coma
// calquera outro parte de erro - deixar que unha IA reescribise e
// recompilase a propia aplicación en marcha, sen revisión humana, seria
// unha porta de entrada a cambios de comportamento non revisados en
// calquera copia instalada (e ademais non hai toolchain de Go/Wails no
// equipo dunha profesora para recompilar nada). Ver conversa/deseño
// orixinal para o razoamento completo.

// InformeErroRequest é o que manda o frontend: o anaco de documento .matex
// que fallou (ou o documento enteiro, se non se puido illar mellor) máis a
// mensaxe de erro literal que devolveu Maxima/LaTeX/Yang.
type InformeErroRequest struct {
	Anaco       string `json:"anaco"`
	MensaxeErro string `json:"mensaxeErro"`
}

// XerarInformeErroIA chama á IA (mesmo chamarIA ca XerarContidoIA/
// XerarExercicioIA) co prompt de informe de erro - devolve texto Markdown
// listo para revisar e gardar, nunca código aplicado.
func (a *App) XerarInformeErroIA(req InformeErroRequest) (string, error) {
	if strings.TrimSpace(req.MensaxeErro) == "" {
		return "", fmt.Errorf("non hai mensaxe de erro que informar")
	}
	peticion := fmt.Sprintf("ANACO/DOCUMENTO QUE FALLOU:\n%s\n\nMENSAXE DE ERRO:\n%s", req.Anaco, req.MensaxeErro)
	return chamarIA(a.settings, systemPromptInformeErro, peticion)
}

// GardarInformeErroDialog garda o informe (xa revisado/editado pola
// profesora ou o desenvolvedor) coma un ficheiro .md - mesmo patrón ca
// SaveFileDialog (app.go): selector nativo "gardar como", escribe o
// contido tal cal.
func (a *App) GardarInformeErroDialog(defaultName, content string) (string, error) {
	dir, base := splitDialogDefault(defaultName)
	path, err := saveFileDialogCompat("Gardar informe de erro", dir, base, "Markdown (*.md)", "*.md")
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
