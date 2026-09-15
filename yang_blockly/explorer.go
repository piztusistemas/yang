package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// explorer.go implementa o explorador de cartafoles/ficheiros do frontend
// (frontend/src/explorer.js), estilo VS Code: unha árbore navegable do
// sistema de ficheiros real, á parte do selector nativo "Abrir"
// (OpenFileDialog, app.go) - pensada para moverse por varios exames/
// prácticas nun mesmo cartafol de traballo sen ter que abrir ese selector
// cada vez. O cartafol raíz elíxese unha vez (EscollerCartafolDialog) e
// lémbrase entre sesións (Settings.UltimoCartafol, ver settings.go).
//
// EntradaCartafol é a mesma forma que consome explorer.js - nomes en
// galego coma o resto do backend, JSON en inglés coma o resto dos tipos
// expostos a JS (ver p.ex. SaveImageRequest en app.go).
type EntradaCartafol struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// ListarCartafol devolve as entradas DIRECTAS de path (non recursivo - a
// árbore do frontend pide cada nivel só ao despregalo, ver explorer.js) -
// cartafoles primeiro, despois ficheiros, cada grupo por orde alfabética
// case-insensible (mesmo criterio ca VS Code). Non filtra nada (nin
// ficheiros ocultos coma ".gitignore"): amosar TODO é o comportamento
// pedido, o único filtrado (destacar .matex) é cousa do frontend.
func (a *App) ListarCartafol(path string) ([]EntradaCartafol, error) {
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	entradas := make([]EntradaCartafol, 0, len(dirEntries))
	for _, de := range dirEntries {
		entradas = append(entradas, EntradaCartafol{
			Name:  de.Name(),
			Path:  filepath.Join(path, de.Name()),
			IsDir: de.IsDir(),
		})
	}
	sort.Slice(entradas, func(i, j int) bool {
		if entradas[i].IsDir != entradas[j].IsDir {
			return entradas[i].IsDir // cartafoles antes ca ficheiros
		}
		return strings.ToLower(entradas[i].Name) < strings.ToLower(entradas[j].Name)
	})
	return entradas, nil
}

// EscollerCartafolDialog amosa o selector nativo "Escoller cartafol" -
// Wails v3 non ten un diálogo "OpenDirectory" á parte, é o mesmo
// OpenFileDialogStruct ca OpenFileDialog (app.go) pero pedíndolle
// CanChooseDirectories(true)/CanChooseFiles(false) - devolve "" (sen erro)
// se a usuaria cancela.
func (a *App) EscollerCartafolDialog() (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle("Escoller cartafol").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		PromptForSingleSelection()
}

// LerFicheiroTexto le o contido dun ficheiro coma texto - única peza que
// faltaba para abrir un .matex premido na árbore (OpenFileDialog xa
// mestura escoller+ler nunha soa chamada, non serve para isto).
func (a *App) LerFicheiroTexto(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// NovoCartafol crea un subcartafol dentro de parentPath e devolve a súa
// ruta completa. err de os.Mkdir xa distingue "xa existe"/permisos etc. -
// non fai falla envolvelo, explorer.js amosa err.Error() tal cal (mesmo
// criterio cás outras pontes de ficheiro en app.go).
func (a *App) NovoCartafol(parentPath, nome string) (string, error) {
	if err := nomeSinguelo(nome); err != nil {
		return "", err
	}
	path := filepath.Join(parentPath, nome)
	if err := os.Mkdir(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// RenomearFicheiro renoma un ficheiro/cartafol DENTRO do seu propio
// directorio (novoNome é só un nome, non unha ruta - nomeSinguelo
// rexéitao se leva "/" ou "\", para non poder moverse fóra do cartafol
// por un nome mal escrito) e devolve a ruta nova completa.
func (a *App) RenomearFicheiro(path, novoNome string) (string, error) {
	if err := nomeSinguelo(novoNome); err != nil {
		return "", err
	}
	novoPath := filepath.Join(filepath.Dir(path), novoNome)
	if err := os.Rename(path, novoPath); err != nil {
		return "", err
	}
	return novoPath, nil
}

// EliminarFicheiro elimina PERMANENTEMENTE un ficheiro ou cartafol (con
// todo o seu contido) - sen papeleira, Go non ten unha API multiplataforma
// xa dispoñible no proxecto para iso. explorer.js confirma sempre coa
// usuaria antes de chamar isto (ver blocks.exercises... non, ver
// explorer.js openDeleteConfirm) - deixar clara esa responsabilidade aquí
// tamén, para quen lea este código sen ver o frontend.
func (a *App) EliminarFicheiro(path string) error {
	return os.RemoveAll(path)
}

// SubirFicheiroRequest carga un ficheiro (imaxe OU pdf, escollido cun
// <input type="file"> normal do navegador - mesmo circuíto ca
// uploadImageFile en main.js, ver fileToBase64) que o profesorado quere
// subir a un cartafol calquera do explorador (destDir - "onde estea situado
// o explorador nese intre", non só a beira do .matex aberto, a diferenza de
// SaveUploadedImage en app.go), base64-codificado. baseDir é o
// directorio do .matex ABERTO agora mesmo (dirOf(currentPath) en main.js),
// "" se aínda non se gardou - só fai falla para calcular RelPath.
type SubirFicheiroRequest struct {
	DestDir  string `json:"destDir"`
	BaseDir  string `json:"baseDir"`
	FileName string `json:"fileName"`
	DataB64  string `json:"dataB64"`
}

// SubirFicheiroResult devolve onde quedou o ficheiro escrito (Path,
// absoluto) e, se se puido calcular (BaseDir non baleiro), RelPath: a ruta
// RELATIVA a BaseDir que hai que usar en <IMG src="..."> ou <A href="...">
// para que se resolva ben (renderImg en latexdoc.go, e o seu equivalente en
// markdowndoc.go, len sempre relativo a baseDir - o directorio do .matex
// aberto, non o de destDir). RelPath queda "" se BaseDir era baleiro (exame
// aínda sen gardar) ou nun disco distinto en Windows (filepath.Rel falla) -
// o frontend amosa entón un aviso "garda o exame primeiro" en vez de
// ofrecer unha etiqueta que nunca ía resolver.
type SubirFicheiroResult struct {
	Path    string `json:"path"`
	RelPath string `json:"relPath"`
}

// SubirFicheiroCartafol copia o ficheiro de SubirFicheiroRequest.DataB64
// dentro de DestDir, evitando colisións de nome coa mesma regra ca
// SaveUploadedImage (dedupeFileName, app.go) - único código compartido
// entre os dous camiños de subida (o antigo, por documento, e este novo, a
// calquera cartafol do explorador).
func (a *App) SubirFicheiroCartafol(req SubirFicheiroRequest) (SubirFicheiroResult, error) {
	result := SubirFicheiroResult{}
	if strings.TrimSpace(req.DestDir) == "" {
		return result, fmt.Errorf("cartafol de destino non válido")
	}
	data, err := base64.StdEncoding.DecodeString(req.DataB64)
	if err != nil {
		return result, fmt.Errorf("ficheiro non válido: %w", err)
	}
	name := dedupeFileName(req.DestDir, req.FileName, "ficheiro")
	fullPath := filepath.Join(req.DestDir, name)
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return result, err
	}
	result.Path = fullPath

	if strings.TrimSpace(req.BaseDir) != "" {
		if rel, err := filepath.Rel(req.BaseDir, fullPath); err == nil {
			result.RelPath = filepath.ToSlash(rel)
		}
	}
	return result, nil
}

// nomeSinguelo valida un "nome" solto (non unha ruta) para
// NovoCartafol/RenomearFicheiro - evita "../etc" ou "sub/nome" (que
// moverían/crearían fóra do cartafol agardado por un erro de escritura,
// non un límite de seguridade: é a propia usuaria a dona do seu sistema de
// ficheiros).
func nomeSinguelo(nome string) error {
	if strings.TrimSpace(nome) == "" {
		return fmt.Errorf("o nome non pode estar baleiro")
	}
	if strings.ContainsAny(nome, "/\\") {
		return fmt.Errorf("o nome non pode levar \"/\" nin \"\\\"")
	}
	if nome == "." || nome == ".." {
		return fmt.Errorf("nome non válido")
	}
	return nil
}
