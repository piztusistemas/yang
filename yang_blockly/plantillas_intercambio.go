package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Intercambio de PLANTILLAS entre compañeiros (ver plantillas.go). Unha
// plantilla non vive nun só ficheiro: o texto vai en plantillas.json e as
// imaxes nun cartafol á parte (<config>/yang/plantillas/<id>/), así que para
// compartila hai que xuntar as dúas cousas nun ficheiro portable.
//
// É un .json normal con TODO dentro - só o que o profesorado edita (nada de
// ID, datas nin DeFabrica: eses póñoos de novo no equipo que a importe) e as
// imaxes en base64. Texto plano a propósito, para poder revisalo (e, se fai
// falla, editalo) antes de mandarllo a ninguén. O que marca que ese .json é
// unha plantilla de Yang non é a extensión senón o campo "formato" de dentro
// (ver importarBundlePlantilla).

const (
	formatoBundlePlantilla   = "yang-plantilla"
	versionBundlePlantilla   = 1
	extensionBundlePlantilla = ".json"
)

// imaxeBundle é unha imaxe da plantilla metida no propio ficheiro de
// intercambio (mesmo par nome/base64 ca DocumentoMostra, ver plantillas_ia.go).
type imaxeBundle struct {
	Nome    string `json:"nome"`
	DataB64 string `json:"dataB64"`
}

// bundlePlantilla é o contido dun ficheiro .json de plantilla.
type bundlePlantilla struct {
	Formato string `json:"formato"`
	Version int    `json:"version"`
	// Plantilla reutiliza PlantillaLatex, pero os campos locais a cada equipo
	// (ID, Creado, Modificado, DeFabrica, Imaxes) van sempre baleiros: escríbeos
	// de novo o equipo que importe (ver importarBundlePlantilla).
	Plantilla PlantillaLatex `json:"plantilla"`
	Imaxes    []imaxeBundle  `json:"imaxes"`
}

// construirBundlePlantilla arma o bundle dunha plantilla xa gardada: o seu
// contido editable, sen os metadatos locais, e as súas imaxes lidas do
// cartafol e postas en base64.
func construirBundlePlantilla(p PlantillaLatex) (bundlePlantilla, error) {
	b := bundlePlantilla{
		Formato: formatoBundlePlantilla,
		Version: versionBundlePlantilla,
		Plantilla: PlantillaLatex{
			Nome:       strings.TrimSpace(p.Nome),
			Descricion: strings.TrimSpace(p.Descricion),
			Latex:      p.Latex,
			Markdown:   p.Markdown,
			Motor:      p.Motor,
			Variables:  normalizarVariables(p.Variables),
		},
	}

	dir, err := plantillaImaxesDir(p.ID)
	if err != nil {
		return bundlePlantilla{}, err
	}
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return b, nil // a plantilla non ten imaxes
	}
	for _, e := range entradas {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return bundlePlantilla{}, err
		}
		b.Imaxes = append(b.Imaxes, imaxeBundle{
			Nome:    e.Name(),
			DataB64: base64.StdEncoding.EncodeToString(data),
		})
	}
	return b, nil
}

// importarBundlePlantilla valida un .json de plantilla xa lido e gárdao coma
// plantilla NOVA neste equipo: ID nova por GardarPlantilla (que reaproveita
// a validación de nome e {{CORPO}}), e despois as imaxes unha a unha polo
// mesmo camiño ca "Engadir imaxe…" do editor. Devolve a plantilla gardada,
// xa coa ID e coas imaxes resoltas.
func (a *App) importarBundlePlantilla(data []byte) (PlantillaLatex, error) {
	var b bundlePlantilla
	if err := json.Unmarshal(data, &b); err != nil {
		return PlantillaLatex{}, fmt.Errorf("o ficheiro non é unha plantilla de Yang válida: %w", err)
	}
	if b.Formato != formatoBundlePlantilla {
		return PlantillaLatex{}, fmt.Errorf("o ficheiro non é unha plantilla de Yang")
	}
	if b.Version > versionBundlePlantilla {
		return PlantillaLatex{}, fmt.Errorf("esta plantilla fíxose cunha versión máis nova de Yang: actualiza Yang para importala")
	}

	p := b.Plantilla
	p.ID = ""
	p.DeFabrica = false
	p.Creado = ""
	p.Modificado = ""
	p.Imaxes = nil
	if strings.TrimSpace(p.Nome) == "" {
		p.Nome = "Plantilla importada"
	}

	gardada, err := a.GardarPlantilla(p)
	if err != nil {
		return PlantillaLatex{}, err
	}

	for _, im := range b.Imaxes {
		if _, err := a.SubirImaxePlantilla(SubirImaxePlantillaRequest{
			ID:       gardada.ID,
			FileName: im.Nome,
			DataB64:  im.DataB64,
		}); err != nil {
			// A plantilla xa está gardada: devólvese igual, avisando de que
			// unha imaxe non entrou, en vez de perder o texto (o caro de
			// reescribir) por un logo que se pode volver subir.
			gardada.Imaxes = imaxesDaPlantilla(gardada.ID)
			return gardada, fmt.Errorf("plantilla importada, pero a imaxe %q non se puido gardar: %w", im.Nome, err)
		}
	}
	gardada.Imaxes = imaxesDaPlantilla(gardada.ID)
	return gardada, nil
}

// nomeFicheiroPlantilla convérte o nome dunha plantilla nun nome de ficheiro
// razoable para propoñer no diálogo de gardar ("Circular do centro" ->
// "Circular do centro"). Quita só o que non pode ir nun nome de ficheiro en
// ningún sistema; o profesorado sempre pode renomear no propio diálogo.
func nomeFicheiroPlantilla(nome string) string {
	nome = strings.TrimSpace(nome)
	var b strings.Builder
	for _, r := range nome {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 0x20 {
			b.WriteRune('-')
			continue
		}
		b.WriteRune(r)
	}
	limpo := strings.Trim(strings.TrimSpace(b.String()), ".")
	if limpo == "" {
		return "plantilla"
	}
	return limpo
}

// ExportarPlantilla garda unha plantilla da biblioteca nun ficheiro
// .json (texto + imaxes en base64) para compartila cos compañeiros.
// Devolve a ruta escollida, ou "" se se cancelou o diálogo.
func (a *App) ExportarPlantilla(id string) (string, error) {
	alm, err := loadAlmacenPlantillas()
	if err != nil {
		return "", err
	}
	var atopada *PlantillaLatex
	for i := range alm.Plantillas {
		if alm.Plantillas[i].ID == id {
			atopada = &alm.Plantillas[i]
			break
		}
	}
	if atopada == nil {
		return "", fmt.Errorf("non existe esa plantilla")
	}

	bundle, err := construirBundlePlantilla(*atopada)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return "", err
	}

	base := nomeFicheiroPlantilla(atopada.Nome) + extensionBundlePlantilla
	path, err := saveFileDialogCompat("Exportar plantilla", "", base,
		"Plantilla de Yang (*"+extensionBundlePlantilla+")", "*"+extensionBundlePlantilla)
	if err != nil || path == "" {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(path), extensionBundlePlantilla) {
		path += extensionBundlePlantilla
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ImportarPlantilla abre un diálogo para escoller un ficheiro .json
// e engádeo á biblioteca coma plantilla nova (ver importarBundlePlantilla).
// Devolve a plantilla gardada; se se cancela o diálogo devolve unha
// PlantillaLatex baleira (ID "") sen erro, para que o frontend o distinga.
func (a *App) ImportarPlantilla() (PlantillaLatex, error) {
	path, err := application.Get().Dialog.OpenFile().
		SetTitle("Importar plantilla").
		AddFilter("Plantilla de Yang (*"+extensionBundlePlantilla+")", "*"+extensionBundlePlantilla).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return PlantillaLatex{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PlantillaLatex{}, err
	}
	return a.importarBundlePlantilla(data)
}
