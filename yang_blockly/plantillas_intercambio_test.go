package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// TestBundlePlantillaRoundTrip: exportar unha plantilla (co seu logo) e
// importala nun equipo "limpo" ten que reproducila enteira - texto,
// variables e imaxes - pero coma plantilla NOVA (ID propia, non de fábrica).
func TestBundlePlantillaRoundTrip(t *testing.T) {
	configIllado(t)
	a := &App{}

	orixe, err := a.GardarPlantilla(PlantillaLatex{
		Nome:       "Circular do centro",
		Descricion: "Cabeceira do IES",
		Latex:      "{{CENTRO}}\n\n{{CORPO}}\n",
		Markdown:   "# {{CENTRO}}\n\n{{CORPO}}\n",
		Motor:      "xelatex",
		Variables:  []PlantillaVar{{Nome: "centro", Etiqueta: "Centro", Valor: "IES Xistral"}},
	})
	if err != nil {
		t.Fatalf("gardar: %v", err)
	}

	pngB64 := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nfake"))
	if _, err := a.SubirImaxePlantilla(SubirImaxePlantillaRequest{
		ID: orixe.ID, FileName: "logo.png", DataB64: pngB64,
	}); err != nil {
		t.Fatalf("subir imaxe: %v", err)
	}

	// --- exportar ---
	recarga, _ := a.ListarPlantillas()
	var conImaxe PlantillaLatex
	for _, p := range recarga.Plantillas {
		if p.ID == orixe.ID {
			conImaxe = p
		}
	}
	bundle, err := construirBundlePlantilla(conImaxe)
	if err != nil {
		t.Fatalf("construir bundle: %v", err)
	}
	if bundle.Formato != formatoBundlePlantilla || bundle.Version != versionBundlePlantilla {
		t.Fatalf("cabeceira do bundle inesperada: %+v", bundle)
	}
	if bundle.Plantilla.ID != "" || bundle.Plantilla.DeFabrica || bundle.Plantilla.Creado != "" {
		t.Errorf("o bundle non debe levar metadatos locais: %+v", bundle.Plantilla)
	}
	if len(bundle.Imaxes) != 1 || bundle.Imaxes[0].Nome != "logo.png" || bundle.Imaxes[0].DataB64 != pngB64 {
		t.Fatalf("a imaxe non viaxou no bundle: %+v", bundle.Imaxes)
	}

	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	// --- importar nun equipo limpo ---
	configIllado(t)
	b := &App{}
	base, _ := b.ListarPlantillas() // sementa as de fábrica
	nImaxe := len(base.Plantillas)

	imp, err := b.importarBundlePlantilla(data)
	if err != nil {
		t.Fatalf("importar: %v", err)
	}
	if imp.ID == "" || imp.DeFabrica {
		t.Errorf("a plantilla importada ten que ser nova e propia: %+v", imp)
	}
	if imp.Nome != "Circular do centro" || imp.Motor != "xelatex" {
		t.Errorf("perdéronse campos ao importar: %+v", imp)
	}
	if len(imp.Variables) != 1 || imp.Variables[0].Nome != "CENTRO" || imp.Variables[0].Valor != "IES Xistral" {
		t.Errorf("as variables non se conservaron: %+v", imp.Variables)
	}
	if len(imp.Imaxes) != 1 || imp.Imaxes[0] != "logo.png" {
		t.Errorf("a imaxe non se escribiu no equipo que importa: %+v", imp.Imaxes)
	}

	fin, _ := b.ListarPlantillas()
	if len(fin.Plantillas) != nImaxe+1 {
		t.Fatalf("importar debería engadir exactamente 1 plantilla, pasou de %d a %d", nImaxe, len(fin.Plantillas))
	}
}

// TestImportarBundleInvalido: os ficheiros que non son plantillas de Yang (ou
// dunha versión máis nova) rexéitanse cun erro claro, sen tocar a biblioteca.
func TestImportarBundleInvalido(t *testing.T) {
	configIllado(t)
	a := &App{}

	casos := map[string][]byte{
		"non é JSON":          []byte("isto non é json"),
		"formato descoñecido": mustJSON(t, map[string]any{"formato": "outra-cousa", "version": 1}),
		"versión do futuro":   mustJSON(t, map[string]any{"formato": formatoBundlePlantilla, "version": versionBundlePlantilla + 1}),
		"sen {{CORPO}}": mustJSON(t, bundlePlantilla{
			Formato: formatoBundlePlantilla, Version: versionBundlePlantilla,
			Plantilla: PlantillaLatex{Nome: "Rota", Latex: `\section{Ola}`},
		}),
	}
	for nome, data := range casos {
		if _, err := a.importarBundlePlantilla(data); err == nil {
			t.Errorf("%s: esperaba erro", nome)
		}
	}

	// Un bundle sen nome recibe un por defecto en vez de fallar.
	ok := mustJSON(t, bundlePlantilla{
		Formato: formatoBundlePlantilla, Version: versionBundlePlantilla,
		Plantilla: PlantillaLatex{Latex: "{{CORPO}}"},
	})
	imp, err := a.importarBundlePlantilla(ok)
	if err != nil {
		t.Fatalf("bundle sen nome: %v", err)
	}
	if strings.TrimSpace(imp.Nome) == "" {
		t.Error("unha plantilla importada sen nome debería recibir un por defecto")
	}
}

// TestNomeFicheiroPlantilla: o nome que se propón no diálogo de gardar nunca
// leva caracteres que rompan un nome de ficheiro.
func TestNomeFicheiroPlantilla(t *testing.T) {
	casos := map[string]string{
		"Circular do centro":  "Circular do centro",
		"Orzamento 2026/2027": "Orzamento 2026-2027",
		`  raro: *?"<>|  `:    "raro- ------",
		"":                    "plantilla",
		"...":                 "plantilla",
	}
	for entrada, esperado := range casos {
		if got := nomeFicheiroPlantilla(entrada); got != esperado {
			t.Errorf("nomeFicheiroPlantilla(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
