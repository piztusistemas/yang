package main

import (
	"testing"
	"time"
)

// TestIATimeoutResolto: "< 30s -> 240s" (0/sen configurar e valores
// demasiado baixos), o resto tal cal.
func TestIATimeoutResolto(t *testing.T) {
	casos := map[int]time.Duration{
		0:   240 * time.Second,
		10:  240 * time.Second,
		29:  240 * time.Second,
		30:  30 * time.Second,
		240: 240 * time.Second,
		600: 600 * time.Second,
	}
	for in, want := range casos {
		if got := (Settings{IATimeoutSegundos: in}).IATimeoutResolto(); got != want {
			t.Errorf("IATimeoutResolto(%d) = %v, want %v", in, got, want)
		}
	}
}

// TestReorganizarResolto: "0/sen configurar -> defecto" e cap nos dous
// extremos, mesmo criterio ca DecimaisResolto/TimeoutResolto.
func TestReorganizarResolto(t *testing.T) {
	sementes := map[int]int{0: 25, -5: 25, 1: 1, 25: 25, 120: 120, 200: 200, 999: 200}
	for in, want := range sementes {
		if got := (Settings{ReorganizarSementes: in}).ReorganizarSementesResolto(); got != want {
			t.Errorf("ReorganizarSementesResolto(%d) = %d, want %d", in, got, want)
		}
	}
	roldas := map[int]int{0: 3, -1: 3, 1: 1, 3: 3, 6: 6, 20: 6}
	for in, want := range roldas {
		if got := (Settings{ReorganizarRoldas: in}).ReorganizarRoldasResolto(); got != want {
			t.Errorf("ReorganizarRoldasResolto(%d) = %d, want %d", in, got, want)
		}
	}
	// MaxRechamadasIA: mesma regra ca ReorganizarRoldas (defecto 2, cap 6).
	rechamadas := map[int]int{0: 2, -3: 2, 1: 1, 2: 2, 6: 6, 99: 6}
	for in, want := range rechamadas {
		if got := (Settings{MaxRechamadasIA: in}).MaxRechamadasIAResolto(); got != want {
			t.Errorf("MaxRechamadasIAResolto(%d) = %d, want %d", in, got, want)
		}
	}
}

// TestAutorepararExerciciosResolto: os tres estados de *bool (nil = defecto
// true), mesmo criterio ca XerarAoGardarResolto.
func TestAutorepararExerciciosResolto(t *testing.T) {
	if !(Settings{}).AutorepararExerciciosResolto() {
		t.Error("sen configurar debería resolver a true")
	}
	activado, desactivado := true, false
	if !(Settings{AutorepararExercicios: &activado}).AutorepararExerciciosResolto() {
		t.Error("=true debería resolver a true")
	}
	if (Settings{AutorepararExercicios: &desactivado}).AutorepararExerciciosResolto() {
		t.Error("=false debería resolver a false")
	}
}

// TestXerarAoGardarResolto comproba os tres estados posibles de
// XerarAoGardar: sen configurar (nil, ficheiro vello ou primeira apertura)
// debe resolver a true (activado por defecto), e un valor explícito - true
// ou false - debe respectarse tal cal.
func TestXerarAoGardarResolto(t *testing.T) {
	if !(Settings{}).XerarAoGardarResolto() {
		t.Error("sen configurar (nil) debería resolver a true")
	}
	activado := true
	if !(Settings{XerarAoGardar: &activado}).XerarAoGardarResolto() {
		t.Error("XerarAoGardar=true debería resolver a true")
	}
	desactivado := false
	if (Settings{XerarAoGardar: &desactivado}).XerarAoGardarResolto() {
		t.Error("XerarAoGardar=false debería resolver a false")
	}
}

// TestExportadoresResolto comproba que os tres exportadores por defecto
// activados (PDF/Tex/Markdown) resolven a true cando non están
// configurados, e que un false explícito se respecta - mesmo criterio ca
// TestXerarAoGardarResolto. DOCX/ODT non se testan aquí: son bool normal
// (sen "sen configurar"), o valor cero de Go xa é o defecto (false).
func TestExportadoresResolto(t *testing.T) {
	if !(Settings{}).ExportarPDFResolto() {
		t.Error("sen configurar, ExportarPDF debería resolver a true")
	}
	if !(Settings{}).ExportarTexResolto() {
		t.Error("sen configurar, ExportarTex debería resolver a true")
	}
	if !(Settings{}).ExportarMarkdownResolto() {
		t.Error("sen configurar, ExportarMarkdown debería resolver a true")
	}
	desactivado := false
	if (Settings{ExportarPDF: &desactivado}).ExportarPDFResolto() {
		t.Error("ExportarPDF=false debería resolver a false")
	}
	if (Settings{ExportarTex: &desactivado}).ExportarTexResolto() {
		t.Error("ExportarTex=false debería resolver a false")
	}
	if (Settings{ExportarMarkdown: &desactivado}).ExportarMarkdownResolto() {
		t.Error("ExportarMarkdown=false debería resolver a false")
	}
}

// TestMigrarAxustesIAVellosCompleta cobre o caso real que motivou a
// migración: un settings.json gardado antes de que Yang soubese falar con
// máis ca Gemini (só tiña geminiApiKey/geminiModel) - despois de
// actualizar, un profesor que xa tiña a IA configurada non debería ter que
// volver escribir a clave.
func TestMigrarAxustesIAVellosCompleta(t *testing.T) {
	velloJSON := []byte(`{"maximaPath":"/usr/bin/maxima","geminiApiKey":"AIzaProba","geminiModel":"gemini-2.0-flash"}`)
	s := Settings{}
	migrarAxustesIAVellos(velloJSON, &s)

	if s.IAProvedor != string(ProvedorGemini) {
		t.Errorf("IAProvedor = %q, quería %q", s.IAProvedor, ProvedorGemini)
	}
	if s.IAAPIKey != "AIzaProba" {
		t.Errorf("IAAPIKey = %q, quería %q", s.IAAPIKey, "AIzaProba")
	}
	if s.IAModel != "gemini-2.0-flash" {
		t.Errorf("IAModel = %q, quería %q", s.IAModel, "gemini-2.0-flash")
	}
}

// TestMigrarAxustesIAVellosNonPisaConfigNova comproba que a migración non
// toca nada se xa hai unha configuración nova gardada (IAAPIKey xa cheo) -
// nin sequera se o JSON en disco aínda contén (por herdanza) os campos
// vellos.
func TestMigrarAxustesIAVellosNonPisaConfigNova(t *testing.T) {
	velloJSON := []byte(`{"geminiApiKey":"clave-vella","iaApiKey":"clave-nova","iaProvedor":"anthropic"}`)
	s := Settings{IAProvedor: "anthropic", IAAPIKey: "clave-nova"}
	migrarAxustesIAVellos(velloJSON, &s)

	if s.IAAPIKey != "clave-nova" {
		t.Errorf("a migración non debería pisar unha clave xa configurada, quedou %q", s.IAAPIKey)
	}
	if s.IAProvedor != "anthropic" {
		t.Errorf("a migración non debería pisar o provedor xa configurado, quedou %q", s.IAProvedor)
	}
}

// TestMigrarAxustesIAVellosSenNadaQueMigrar comproba que un settings.json
// sen configuración de IA ningunha (nin vella nin nova) non provoca erro
// nin dato inventado.
func TestMigrarAxustesIAVellosSenNadaQueMigrar(t *testing.T) {
	s := Settings{}
	migrarAxustesIAVellos([]byte(`{"maximaPath":"/usr/bin/maxima"}`), &s)
	if s.IAAPIKey != "" || s.IAProvedor != "" {
		t.Errorf("non debería migrar nada, obtiven %+v", s)
	}
}
