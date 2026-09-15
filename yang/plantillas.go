package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"matexe-wails/cas"
)

// Biblioteca de PLANTILLAS de documento: o que converte Yang nun editor de
// documentos estándar en vez de "só" un xerador de exames. Unha plantilla
// decide COMO se maqueta o documento (cabeceira do instituto, membrete dun
// orzamento, marxes, tipografía...); o .matex segue decidindo QUE contén.
// Aplícase á vista previa (que xa é o PDF real, ver run() en main.js) e a
// todas as exportacións: PDF, .tex, .md, .docx e .odt.
//
// Formato dunha plantilla: LaTeX normal e corrente cun marcador {{CORPO}}
// no sitio onde vai o documento xerado, máis {{VARIABLES}} en maiúsculas
// que o profesorado enche unha vez (centro, materia, empresa, CIF...). Hai
// dúas formas válidas, distinguidas SÓ por levar \documentclass ou non:
//
//   - Plantilla COMPLETA (leva \documentclass): manda ela de todo. Yang só
//     lle inxecta, xusto antes do \begin{document}, os \usepackage que lle
//     falten dos que necesitan as etiquetas de Matexe (<PLOT> precisa
//     gnuplot-lua-tikz, <IMG> precisa graphicx...) - ver
//     inxectarPreambuloObrigatorio. Así unha plantilla de orzamento pode
//     cambiar clase, marxes e fontes sen romper un gráfico.
//   - Plantilla FRAGMENTO (sen \documentclass): Yang envólvea no seu
//     preámbulo de sempre (o mesmo que se usa sen plantilla). Abonda para o
//     caso máis común - engadir unha cabeceira e un pé ao documento - sen
//     ter que saber nada de preámbulos.
//
// As imaxes (logotipos) non se meten na plantilla en base64: gárdanse en
// <config>/yang/plantillas/<id>/ e cópianse a carón do documento no intre
// de compilar (ver copiarImaxesPlantilla), así que na plantilla
// referéncianse polo seu nome de ficheiro a secas.

// PlantillaVar é unha variable que o profesorado enche unha soa vez na
// plantilla e Yang substitúe en cada xeración: {{CENTRO}}, {{MATERIA}}...
// Nome vai sempre en MAIÚSCULAS sen espazos (normalizarNomeVar), que é o
// que fai recoñecible un marcador dentro de código LaTeX sen confundilo cos
// {} propios de LaTeX.
type PlantillaVar struct {
	Nome     string `json:"nome"`
	Etiqueta string `json:"etiqueta"` // como se amosa na UI ("Centro educativo")
	Valor    string `json:"valor"`
}

// PlantillaLatex é unha plantilla completa da biblioteca.
type PlantillaLatex struct {
	ID         string `json:"id"`
	Nome       string `json:"nome"`
	Descricion string `json:"descricion"`
	// Latex é a plantilla en si (completa ou fragmento, ver arriba). Ten que
	// conter {{CORPO}}.
	Latex string `json:"latex"`
	// Markdown é a versión equivalente para .md/.docx/.odt (as exportacións
	// que non pasan por LaTeX, ver markdowndoc.go/docdoc.go): mesmo
	// {{CORPO}} e mesmas variables, pero en Markdown. Baleiro = esas tres
	// exportacións saen exactamente coma antes de haber plantillas.
	Markdown string `json:"markdown"`
	// Motor forza o compilador desta plantilla ("xelatex" para unha que use
	// fontspec/fontes do sistema, por exemplo). Baleiro = o de Opcións.
	Motor string `json:"motor"`
	// Variables son as definidas POLA plantilla. As automáticas ({{DATA}},
	// {{FICHEIRO}}, {{TITULO}}) non se listan aquí: dáas Yang (ver
	// variablesAutomaticas) e unha variable con ese mesmo nome definida aquí
	// gaña, para poder fixar un título propio.
	Variables []PlantillaVar `json:"variables"`
	// Imaxes é sempre o contido REAL do cartafol de imaxes da plantilla:
	// recalcúlase en cada listaxe/gardado (imaxesDaPlantilla) en vez de
	// fiarse do que houbese no JSON, así non se desincroniza se alguén toca
	// o cartafol á man ou copia un plantillas.json doutro equipo.
	Imaxes     []string `json:"imaxes"`
	Creado     string   `json:"creado"`
	Modificado string   `json:"modificado"`
	// DeFabrica marca as plantillas de exemplo que Yang sementa a primeira
	// vez (ver plantillasDeFabrica). Só serve para que a UI as poida
	// distinguir; edítanse e bórranse coma calquera outra.
	DeFabrica bool `json:"deFabrica"`
}

// almacenPlantillas é o ficheiro completo: as plantillas máis o estado da
// escolla. Vai TODO nun ficheiro propio (non en settings.json) por unha
// razón práctica: SaveSettings (settings.go) substitúe o Settings enteiro
// polo que manda o diálogo de Opcións, así que gardar aquí a plantilla
// activa faríaa desaparecer cada vez que alguén premese "Gardar" en
// Opcións.
type almacenPlantillas struct {
	Plantillas []PlantillaLatex `json:"plantillas"`
	// Activa é a plantilla que se aplica agora mesmo ("" = ningunha).
	Activa string `json:"activa"`
	// PorFicheiro lembra que plantilla usaba cada .matex (ruta absoluta ->
	// ID), para que abrir un exame do instituto ou un orzamento restaure a
	// súa maqueta sen tocar o formato .matex. Límpase soa: as entradas de
	// ficheiros que xa non existen bótanse ao gardar (ver podarPorFicheiro).
	PorFicheiro map[string]string `json:"porFicheiro"`
	// Sementado marca que xa se pasou algunha vez por aquí. Xa non decide el
	// só se hai que sementar (ver sementarSeFai): consérvase para non
	// romper un plantillas.json escrito por unha versión anterior.
	Sementado bool `json:"sementado"`
	// FabricaBorradas son as IDs de plantillas DE FÁBRICA que o profesorado
	// eliminou. Sen esta lista non se podería distinguir "esta plantilla é
	// nova nesta versión de Yang, hai que dala de alta" de "esta xa a
	// borrou a propietaria e non a quere ver máis" - e volverían aparecer
	// soas en cada arranque.
	FabricaBorradas []string `json:"fabricaBorradas"`
}

const marcadorCorpo = "{{CORPO}}"

// varRe recoñece un marcador {{NOME}}. Esixir MAIÚSCULAS (e que empece por
// letra) é o que evita confundir unha variable cun grupo LaTeX lexítimo:
// \frac{{a+b}}{2} non ten ningún marcador, \textbf{ {{CENTRO}} } si.
var varRe = regexp.MustCompile(`\{\{\s*([A-ZÁÉÍÓÚÜÑ][A-Z0-9_ÁÉÍÓÚÜÑ]*)\s*\}\}`)

func plantillasPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "yang")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "plantillas.json"), nil
}

// plantillaImaxesDir é o cartafol de imaxes DUNHA plantilla. Un cartafol por
// plantilla (non un común) para que duplicar/borrar unha plantilla sexa
// copiar/borrar un cartafol, sen levar contas de que logo usa quen.
func plantillaImaxesDir(id string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "yang", "plantillas", id), nil
}

func loadAlmacenPlantillas() (almacenPlantillas, error) {
	var alm almacenPlantillas
	path, err := plantillasPath()
	if err != nil {
		return alm, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sementarSeFai(almacenPlantillas{})
		}
		return alm, err
	}
	if err := json.Unmarshal(b, &alm); err != nil {
		return alm, fmt.Errorf("plantillas.json ilexible: %w", err)
	}
	alm, err = repararFabricaEditadas(alm)
	if err != nil {
		return alm, err
	}
	return sementarSeFai(alm)
}

// repararFabricaEditadas arranxa, unha soa vez e por equipo, o estropicio que
// deixaban as versións de Yang anteriores a bifurcarDeFabrica: alí gardar por
// riba dunha plantilla de fábrica pisábaa EN SITIO, deixando unha entrada
// marcada DeFabrica pero con contido do profesorado. Iso tiña dous efectos
// malos que non se ven ata que é tarde:
//
//   - a ID de fábrica quedaba ocupada, e sementarSeFai() (que só repón as que
//     FALTAN) xa nunca lle daba as melloras dunha versión nova de Yang,
//   - datos privados (o IBAN dunha empresa, o nome dun centro) quedaban baixo
//     unha entrada que a interface segue a amosar coma "de exemplo", cun
//     risco evidente en canto alguén comparte o seu plantillas.json.
//
// A reparación é a mesma operación que fai agora bifurcarDeFabrica, pero
// aplicada ao que xa está gardado: o contido editado pasa a unha plantilla
// propia (ID nova, DeFabrica=false) que herda imaxes, escolla activa e
// documentos, e a de fábrica recupera o seu contido orixinal. Non se perde
// nada e non hai que pedirlle nada ao profesorado.
//
// Detección por comparación co orixinal (non por unha marca no ficheiro):
// así funciona tamén nun plantillas.json que veña doutro equipo, e unha
// plantilla de fábrica que nunca se tocou non se move do sitio.
func repararFabricaEditadas(alm almacenPlantillas) (almacenPlantillas, error) {
	orixinais := make(map[string]PlantillaLatex)
	for _, p := range plantillasDeFabrica() {
		orixinais[p.ID] = p
	}
	agora := time.Now().Format(time.RFC3339)
	cambiou := false
	// Percorre unha copia dos índices: bifurcarDeFabrica engade ao final, e
	// o engadido non hai que volvelo examinar (xa non é DeFabrica).
	for i := 0; i < len(alm.Plantillas); i++ {
		actual := alm.Plantillas[i]
		orixinal, ok := orixinais[actual.ID]
		if !ok || !actual.DeFabrica || !fabricaFoiEditada(actual, orixinal) {
			continue
		}
		// A de fábrica volve ao seu contido; o editado vaise a unha copia.
		orixinal.Creado, orixinal.Modificado = actual.Creado, agora
		alm.Plantillas[i] = orixinal
		var err error
		if alm, err = bifurcarEnSitio(alm, actual, agora); err != nil {
			return alm, err
		}
		cambiou = true
	}
	if !cambiou {
		return alm, nil
	}
	return alm, saveAlmacenPlantillas(alm)
}

// fabricaFoiEditada compara só o que o profesorado pode cambiar dende o
// editor de plantillas. Creado/Modificado/Imaxes quedan fóra a propósito:
// son metadatos que Yang toca por conta propia (imaxesDaPlantilla recalcula
// Imaxes en cada listaxe) e dispararían falsos positivos.
func fabricaFoiEditada(actual, orixinal PlantillaLatex) bool {
	if actual.Nome != orixinal.Nome || actual.Descricion != orixinal.Descricion ||
		actual.Latex != orixinal.Latex || actual.Markdown != orixinal.Markdown ||
		actual.Motor != orixinal.Motor || len(actual.Variables) != len(orixinal.Variables) {
		return true
	}
	for i := range actual.Variables {
		if actual.Variables[i] != orixinal.Variables[i] {
			return true
		}
	}
	return false
}

// bifurcarEnSitio é o núcleo compartido por bifurcarDeFabrica (ao gardar) e
// por repararFabricaEditadas (ao cargar): mete `p` coma plantilla propia
// nova e traspásalle todo o que colgaba da de fábrica. NON garda - déixao a
// quen chama, que sabe se hai máis cambios que escribir na mesma pasada.
func bifurcarEnSitio(alm almacenPlantillas, p PlantillaLatex, agora string) (almacenPlantillas, error) {
	orixeID := p.ID
	p.ID = strconv.FormatInt(time.Now().UnixNano(), 36)
	p.DeFabrica = false
	p.Creado = agora
	p.Modificado = agora
	alm.Plantillas = append(alm.Plantillas, p)

	if alm.Activa == orixeID {
		alm.Activa = p.ID
	}
	for ruta, id := range alm.PorFicheiro {
		if id == orixeID {
			alm.PorFicheiro[ruta] = p.ID
		}
	}
	return alm, copiarCartafolImaxes(orixeID, p.ID)
}

// sementarSeFai dá de alta as plantillas de fábrica que FALTEN, comparando
// por ID - non só a primeira vez. Iso é o que permite REPARTIR plantillas:
// unha versión nova de Yang que engada unha en plantillas_fabrica.go
// chégalle tamén a quen xa tiña Yang instalado dende antes, sen ter que
// borrar a súa configuración.
//
// Dúas regras que fan que iso sexa seguro:
//   - unha ID que xa existe NON se toca nunca (o profesorado pode ter
//     editado esa plantilla: pisarlla nunha actualización sería perder o
//     seu traballo sen avisar),
//   - unha que se borrou a propósito non revive (FabricaBorradas).
func sementarSeFai(alm almacenPlantillas) (almacenPlantillas, error) {
	xaEstan := make(map[string]bool, len(alm.Plantillas))
	for _, p := range alm.Plantillas {
		xaEstan[p.ID] = true
	}
	borradas := make(map[string]bool, len(alm.FabricaBorradas))
	for _, id := range alm.FabricaBorradas {
		borradas[id] = true
	}

	var novas []PlantillaLatex
	for _, p := range plantillasDeFabrica() {
		if xaEstan[p.ID] || borradas[p.ID] {
			continue
		}
		novas = append(novas, p)
	}
	if len(novas) == 0 && alm.Sementado {
		return alm, nil
	}
	alm.Sementado = true
	alm.Plantillas = append(alm.Plantillas, novas...)
	if err := saveAlmacenPlantillas(alm); err != nil {
		return alm, err
	}
	return alm, nil
}

func saveAlmacenPlantillas(alm almacenPlantillas) error {
	path, err := plantillasPath()
	if err != nil {
		return err
	}
	alm.PorFicheiro = podarPorFicheiro(alm.PorFicheiro)
	b, err := json.MarshalIndent(alm, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// podarPorFicheiro quita as entradas de documentos que xa non existen no
// disco - sen isto, a memoria "que plantilla usaba cada .matex" medraría
// para sempre con ficheiros renomeados ou borrados hai anos.
func podarPorFicheiro(m map[string]string) map[string]string {
	if len(m) == 0 {
		return m
	}
	out := make(map[string]string, len(m))
	for ruta, id := range m {
		if fileExists(ruta) {
			out[ruta] = id
		}
	}
	return out
}

// imaxesDaPlantilla le o cartafol de imaxes da plantilla. Un cartafol que
// non existe (o normal: aínda non se subiu ningunha) non é erro.
func imaxesDaPlantilla(id string) []string {
	dir, err := plantillaImaxesDir(id)
	if err != nil {
		return nil
	}
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var nomes []string
	for _, e := range entradas {
		if !e.IsDir() {
			nomes = append(nomes, e.Name())
		}
	}
	sort.Strings(nomes)
	return nomes
}

// ---------- API para o frontend ----------

// PlantillasEstado é o que precisa a barra de ferramentas dunha soa vez: a
// lista enteira e cal está activa.
type PlantillasEstado struct {
	Plantillas []PlantillaLatex `json:"plantillas"`
	Activa     string           `json:"activa"`
}

// ListarPlantillas devolve a biblioteca ordenada por nome, con `imaxes` xa
// resolto, e a plantilla activa.
func (a *App) ListarPlantillas() (PlantillasEstado, error) {
	alm, err := loadAlmacenPlantillas()
	if err != nil {
		return PlantillasEstado{}, err
	}
	items := alm.Plantillas
	for i := range items {
		items[i].Imaxes = imaxesDaPlantilla(items[i].ID)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Nome) < strings.ToLower(items[j].Nome)
	})
	if items == nil {
		items = []PlantillaLatex{}
	}
	return PlantillasEstado{Plantillas: items, Activa: alm.Activa}, nil
}

// GardarPlantilla crea (ID baleira) ou actualiza unha plantilla. Valida o
// mínimo imprescindible aquí, no backend, porque a mesma plantilla pode
// chegar do editor manual, do asistente de IA ou dun plantillas.json
// editado á man.
func (a *App) GardarPlantilla(p PlantillaLatex) (PlantillaLatex, error) {
	p.Nome = strings.TrimSpace(p.Nome)
	if p.Nome == "" {
		return PlantillaLatex{}, fmt.Errorf("dálle un nome á plantilla")
	}
	if strings.TrimSpace(p.Latex) == "" {
		return PlantillaLatex{}, fmt.Errorf("a plantilla non ten código LaTeX")
	}
	if !strings.Contains(p.Latex, marcadorCorpo) {
		return PlantillaLatex{}, fmt.Errorf("o LaTeX da plantilla ten que levar %s no sitio onde vai o documento", marcadorCorpo)
	}
	if strings.TrimSpace(p.Markdown) != "" && !strings.Contains(p.Markdown, marcadorCorpo) {
		return PlantillaLatex{}, fmt.Errorf("o Markdown da plantilla ten que levar %s no sitio onde vai o documento", marcadorCorpo)
	}
	p.Variables = normalizarVariables(p.Variables)

	alm, err := loadAlmacenPlantillas()
	if err != nil {
		return PlantillaLatex{}, err
	}
	agora := time.Now().Format(time.RFC3339)
	if strings.TrimSpace(p.ID) == "" {
		p.ID = strconv.FormatInt(time.Now().UnixNano(), 36)
		p.Creado = agora
		p.Modificado = agora
		alm.Plantillas = append(alm.Plantillas, p)
	} else {
		atopada := false
		for i := range alm.Plantillas {
			if alm.Plantillas[i].ID != p.ID {
				continue
			}
			// Gardar por riba dunha DE FÁBRICA non a pisa: bifúrcase.
			if alm.Plantillas[i].DeFabrica {
				return bifurcarDeFabrica(alm, p, agora)
			}
			p.Creado = alm.Plantillas[i].Creado
			p.DeFabrica = alm.Plantillas[i].DeFabrica
			p.Modificado = agora
			alm.Plantillas[i] = p
			atopada = true
			break
		}
		if !atopada {
			// ID que xa non existe (borrada noutra xanela): gárdase igual
			// coma nova en vez de perder o traballo do profesorado.
			p.Creado = agora
			p.Modificado = agora
			alm.Plantillas = append(alm.Plantillas, p)
		}
	}
	if err := saveAlmacenPlantillas(alm); err != nil {
		return PlantillaLatex{}, err
	}
	p.Imaxes = imaxesDaPlantilla(p.ID)
	return p, nil
}

// bifurcarDeFabrica: editar unha plantilla DE FÁBRICA e gardala non a
// sobrescribe - créase unha copia propia (ID novo, DeFabrica=false) co que
// o profesorado acaba de escribir, e a de fábrica queda intacta ao lado.
//
// Por que fai falla: sementarSeFai() só repón as IDs de fábrica que FALTAN.
// Unha de fábrica editada en sitio quedaba coa ID "ocupada" para sempre, así
// que xa nunca recibía as melloras que trouxese unha versión nova de Yang.
// E de camiño mesturaba datos privados (o IBAN dunha empresa, o nome dun
// centro) cunha entrada que a interface segue a marcar coma "de exemplo".
//
// A bifurcación herda TODO o que a de fábrica tiña asociado - as imaxes, ser
// a plantilla activa, e os documentos que a usaban - a propósito: de cara ao
// profesorado, gardar ten que seguir facendo o de sempre (o meu traballo
// aplícase aos meus documentos). O único que cambia é que a de fábrica
// sobrevive, limpa, en vez de desaparecer baixo os cambios.
func bifurcarDeFabrica(alm almacenPlantillas, p PlantillaLatex, agora string) (PlantillaLatex, error) {
	alm, errImaxes := bifurcarEnSitio(alm, p, agora)
	nova := alm.Plantillas[len(alm.Plantillas)-1]
	if err := saveAlmacenPlantillas(alm); err != nil {
		return PlantillaLatex{}, err
	}
	// O erro de copiar imaxes devólvese DESPOIS de gardar, non antes: se
	// falla, a bifurcación xa existe e o texto (o caro de reescribir) está a
	// salvo; só faltarían as imaxes, que se poden volver subir.
	if errImaxes != nil {
		return nova, errImaxes
	}
	nova.Imaxes = imaxesDaPlantilla(nova.ID)
	return nova, nil
}

// EliminarPlantilla bórraa xunto co seu cartafol de imaxes. Se era a
// activa, queda "ningunha" - senón as xeracións seguintes fallarían
// apuntando a unha plantilla que xa non está.
func (a *App) EliminarPlantilla(id string) error {
	alm, err := loadAlmacenPlantillas()
	if err != nil {
		return err
	}
	out := alm.Plantillas[:0]
	for _, p := range alm.Plantillas {
		if p.ID != id {
			out = append(out, p)
			continue
		}
		// Anotar as de fábrica borradas para que sementarSeFai non as
		// volva dar de alta no seguinte arranque.
		if p.DeFabrica {
			alm.FabricaBorradas = append(alm.FabricaBorradas, p.ID)
		}
	}
	alm.Plantillas = out
	if alm.Activa == id {
		alm.Activa = ""
	}
	for ruta, usada := range alm.PorFicheiro {
		if usada == id {
			delete(alm.PorFicheiro, ruta)
		}
	}
	if err := saveAlmacenPlantillas(alm); err != nil {
		return err
	}
	if dir, err := plantillaImaxesDir(id); err == nil && id != "" {
		os.RemoveAll(dir)
	}
	return nil
}

// DuplicarPlantilla fai unha copia editable (imaxes incluídas) - a forma
// natural de partir dunha plantilla de fábrica sen perdela.
func (a *App) DuplicarPlantilla(id string) (PlantillaLatex, error) {
	alm, err := loadAlmacenPlantillas()
	if err != nil {
		return PlantillaLatex{}, err
	}
	for _, p := range alm.Plantillas {
		if p.ID != id {
			continue
		}
		copia := p
		copia.ID = ""
		copia.DeFabrica = false
		copia.Nome = p.Nome + " (copia)"
		nova, err := a.GardarPlantilla(copia)
		if err != nil {
			return PlantillaLatex{}, err
		}
		if err := copiarCartafolImaxes(id, nova.ID); err != nil {
			return nova, err
		}
		nova.Imaxes = imaxesDaPlantilla(nova.ID)
		return nova, nil
	}
	return PlantillaLatex{}, fmt.Errorf("non existe esa plantilla")
}

func copiarCartafolImaxes(orixeID, destinoID string) error {
	orixe, err := plantillaImaxesDir(orixeID)
	if err != nil {
		return err
	}
	entradas, err := os.ReadDir(orixe)
	if err != nil {
		return nil // sen imaxes que copiar
	}
	destino, err := plantillaImaxesDir(destinoID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destino, 0o755); err != nil {
		return err
	}
	for _, e := range entradas {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(orixe, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destino, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// SubirImaxePlantillaRequest mirra SaveImageRequest (app.go), pero o destino
// non é o cartafol do documento: é o da plantilla, que ten que funcionar
// con calquera .matex, estea onde estea.
type SubirImaxePlantillaRequest struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
	DataB64  string `json:"dataB64"`
}

// SubirImaxePlantilla garda un logo (ou calquera imaxe) no cartafol da
// plantilla e devolve o nome co que hai que referenciala: na plantilla
// LaTeX \includegraphics{<nome>}, no Markdown ![](images/<nome>) - ver
// copiarImaxesPlantilla, que as deixa nos dous sitios no intre de xerar.
func (a *App) SubirImaxePlantilla(req SubirImaxePlantillaRequest) (string, error) {
	if strings.TrimSpace(req.ID) == "" {
		return "", fmt.Errorf("garda a plantilla antes de engadirlle imaxes")
	}
	data, err := base64.StdEncoding.DecodeString(req.DataB64)
	if err != nil {
		return "", fmt.Errorf("imaxe non válida: %w", err)
	}
	dir, err := plantillaImaxesDir(req.ID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	nome := dedupeFileName(dir, filepath.Base(req.FileName), "imaxe")
	if err := os.WriteFile(filepath.Join(dir, nome), data, 0o644); err != nil {
		return "", err
	}
	return nome, nil
}

// EliminarImaxePlantilla borra unha imaxe da plantilla. filepath.Base sobre
// o nome recibido evita que un "../../algo" saia do cartafol da plantilla.
func (a *App) EliminarImaxePlantilla(id, nome string) error {
	dir, err := plantillaImaxesDir(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(nome) == "" {
		return fmt.Errorf("falta o nome da imaxe")
	}
	return os.Remove(filepath.Join(dir, filepath.Base(nome)))
}

// EscollerPlantilla fixa a plantilla activa e, se hai un documento aberto,
// lembra que ESE documento usa esta plantilla (id "" = ningunha). Chámase
// dende o selector da barra de ferramentas.
func (a *App) EscollerPlantilla(id, ficheiro string) error {
	alm, err := loadAlmacenPlantillas()
	if err != nil {
		return err
	}
	alm.Activa = id
	if ruta := strings.TrimSpace(ficheiro); ruta != "" {
		if alm.PorFicheiro == nil {
			alm.PorFicheiro = map[string]string{}
		}
		if id == "" {
			delete(alm.PorFicheiro, ruta)
		} else {
			alm.PorFicheiro[ruta] = id
		}
	}
	return saveAlmacenPlantillas(alm)
}

// PlantillaParaFicheiro devolve (e activa) a plantilla lembrada para ese
// documento ao abrilo. Se ese .matex nunca tivo plantilla, mantense a
// activa actual: cambiar de documento non debe desfacer a escolla que
// acaba de facer o profesorado para traballar.
func (a *App) PlantillaParaFicheiro(ficheiro string) (string, error) {
	alm, err := loadAlmacenPlantillas()
	if err != nil {
		return "", err
	}
	ruta := strings.TrimSpace(ficheiro)
	if ruta == "" {
		return alm.Activa, nil
	}
	id, ok := alm.PorFicheiro[ruta]
	if !ok {
		return alm.Activa, nil
	}
	// A plantilla lembrada pode ter sido borrada mentres tanto.
	existe := false
	for _, p := range alm.Plantillas {
		if p.ID == id {
			existe = true
			break
		}
	}
	if !existe {
		return alm.Activa, nil
	}
	if alm.Activa != id {
		alm.Activa = id
		if err := saveAlmacenPlantillas(alm); err != nil {
			return "", err
		}
	}
	return id, nil
}

// plantillaActiva resolve a plantilla que hai que aplicar agora mesmo, ou
// nil se non hai ningunha (comportamento de sempre). Nunca devolve erro
// "brando": se o ficheiro está corrupto ou a plantilla activa desapareceu,
// xérase sen plantilla en vez de bloquear o traballo.
func (a *App) plantillaActiva() *PlantillaLatex {
	alm, err := loadAlmacenPlantillas()
	if err != nil || alm.Activa == "" {
		return nil
	}
	for i := range alm.Plantillas {
		if alm.Plantillas[i].ID == alm.Activa {
			p := alm.Plantillas[i]
			return &p
		}
	}
	return nil
}

// ---------- Substitución e composición ----------

// normalizarNomeVar deixa o nome dunha variable en MAIÚSCULAS e sen nada
// que non poida aparecer nun marcador {{...}} - o profesorado escribe
// "centro educativo" e a plantilla usa {{CENTRO_EDUCATIVO}}.
func normalizarNomeVar(nome string) string {
	nome = strings.ToUpper(strings.TrimSpace(nome))
	var b strings.Builder
	for _, r := range nome {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case strings.ContainsRune("ÁÉÍÓÚÜÑ", r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func normalizarVariables(vars []PlantillaVar) []PlantillaVar {
	out := make([]PlantillaVar, 0, len(vars))
	vistas := map[string]bool{}
	for _, v := range vars {
		v.Nome = normalizarNomeVar(v.Nome)
		if v.Nome == "" || v.Nome == "CORPO" || vistas[v.Nome] {
			continue
		}
		vistas[v.Nome] = true
		out = append(out, v)
	}
	return out
}

// variablesAutomaticas son as que enche Yang soa en cada xeración. Unha
// variable definida na plantilla co mesmo nome ten prioridade (ex.: fixar
// un {{TITULO}} propio en vez do nome do ficheiro).
func variablesAutomaticas(nomeDoc string) map[string]string {
	nomeDoc = strings.TrimSpace(nomeDoc)
	return map[string]string{
		"DATA":     time.Now().Format("02/01/2006"),
		"FICHEIRO": nomeDoc,
		"TITULO":   nomeDoc,
		"ANO":      time.Now().Format("2006"),
	}
}

// baleiroLatex é o que se pon no sitio dunha variable SEN valor cando se
// está a compoñer LaTeX. Non pode ser a cadea baleira: unha liña de
// plantilla coma "{\\LARGE {{EMPRESA}} }\\\\[4pt]" quedaría cun \\\\ sen nada
// diante e pdflatex morre con "There's no line here to end" - un orzamento
// recén creado, coas variables aínda por encher, non compilaría. \\mbox{}
// non se ve no PDF pero xa é "algo" para LaTeX. En Markdown non fai falla
// nada disto (unha liña baleira alí non é erro), así que alí vai baleiro.
const baleiroLatex = `\mbox{}`

// substituirVariables cambia cada {{NOME}} polo seu valor. Un marcador sen
// valor NON se deixa tal cal (aparecería literalmente no PDF): queda baleiro
// e avísase, que é o que deixa ver o erro sen estragar o documento.
// {{CORPO}} non se toca aquí - insérese despois, para que un {{X}} escrito
// dentro do propio exame non se interprete coma variable da plantilla.
//
// `escapar` (non nil só no camiño LaTeX) escapa o VALOR de cada variable
// antes de metelo na maqueta: o valor é texto plano dun formulario (nome do
// centro, materia) ou o nome do ficheiro, e un "_", "&", "%", "→"... sen
// escapar rompía a compilación con "Missing $ inserted" (report real: un
// boletín titulado "unidade_2" -> "\bfseries unidade_2" -> erro). A maqueta
// en si NON se escapa (leva LaTeX a propósito); en Markdown non fai falla,
// así que alí vai nil.
func substituirVariables(texto string, p *PlantillaLatex, auto map[string]string, baleiro string, escapar func(string) string) (string, []string) {
	valores := map[string]string{}
	for k, v := range auto {
		valores[k] = v
	}
	for _, v := range p.Variables {
		valores[v.Nome] = v.Valor
	}
	var avisos []string
	vistos := map[string]bool{}
	out := varRe.ReplaceAllStringFunc(texto, func(m string) string {
		nome := varRe.FindStringSubmatch(m)[1]
		if nome == "CORPO" {
			return m
		}
		val, ok := valores[nome]
		if !ok {
			if !vistos[nome] {
				vistos[nome] = true
				avisos = append(avisos, fmt.Sprintf("a plantilla usa {{%s}}, que non está definida: quedou baleira", nome))
			}
			return baleiro
		}
		if strings.TrimSpace(val) == "" {
			return baleiro
		}
		if escapar != nil {
			return escapar(val)
		}
		return val
	})
	return out, avisos
}

// paquetesDeclarados recolle os nomes de paquete que a plantilla xa carga,
// para non duplicarllos (un \usepackage repetido con opcións distintas é un
// erro duro de LaTeX: "Option clash for package ...").
var usepackageRe = regexp.MustCompile(`\\(?:usepackage|RequirePackage)\s*(?:\[[^\]]*\])?\s*\{([^}]*)\}`)

func paquetesDeclarados(latex string) map[string]bool {
	out := map[string]bool{}
	for _, m := range usepackageRe.FindAllStringSubmatch(latex, -1) {
		for _, nome := range strings.Split(m[1], ",") {
			out[strings.TrimSpace(nome)] = true
		}
	}
	return out
}

// paquetesMatexe son os que necesitan as etiquetas de Matexe para renderizar:
// sen graphicx non hai <IMG> nin <PLOT>, sen gnuplot-lua-tikz non hai
// gráfico en TikZ, etc. Inxéctanse na plantilla do profesorado só se lle
// faltan (ver inxectarPreambuloObrigatorio).
var paquetesMatexe = []struct{ nome, linha string }{
	{"amsmath", `\usepackage{amsmath,amssymb}`},
	{"graphicx", `\usepackage{graphicx}`},
	{"tikz", `\usepackage{tikz}`},
	{"gnuplot-lua-tikz", `\usepackage{gnuplot-lua-tikz}`},
	{"circuitikz", `\usepackage{circuitikz}`},
	{"xcolor", `\usepackage{xcolor}`},
	{"hyperref", `\usepackage[hidelinks]{hyperref}`},
}

// inxectarPreambuloObrigatorio engade, xusto antes do \begin{document} da
// plantilla, o que fai falla para que as etiquetas de Matexe sigan
// funcionando: os paquetes que falten, o soporte de UTF-8 propio de cada
// motor, e babel. Non toca nada do que a plantilla xa declare - a idea é
// que o profesorado poida cambiar clase, marxes e fontes libremente e que
// un <PLOT> siga saíndo igual.
func inxectarPreambuloObrigatorio(doc, engine string) string {
	pos := strings.Index(doc, `\begin{document}`)
	if pos < 0 {
		// Sen \begin{document} non hai onde inxectar (plantilla a medio
		// escribir): déixase tal cal e que falle o compilador, que dá unha
		// mensaxe moito máis clara ca calquera adiviña nosa.
		return doc
	}
	declarados := paquetesDeclarados(doc[:pos])
	var faltan []string
	if engineUnicodeNativo(engine) {
		if !declarados["fontspec"] {
			faltan = append(faltan, `\usepackage{fontspec}`)
		}
	} else {
		if !declarados["inputenc"] {
			faltan = append(faltan, `\usepackage[utf8]{inputenc}`)
		}
		if !declarados["fontenc"] {
			faltan = append(faltan, `\usepackage[T1]{fontenc}`)
		}
		faltan = append(faltan, strings.TrimRight(latexUnicodeDeclarations, "\n"))
	}
	for _, p := range paquetesMatexe {
		if !declarados[p.nome] {
			faltan = append(faltan, p.linha)
		}
	}
	if !declarados["babel"] {
		if b := strings.TrimRight(babelLine(), "\n"); b != "" {
			faltan = append(faltan, b)
		}
	}
	if len(faltan) == 0 {
		return doc
	}
	bloque := "% --- engadido por Yang para as etiquetas de Matexe ---\n" +
		strings.Join(faltan, "\n") + "\n"
	return doc[:pos] + bloque + doc[pos:]
}

// documentoLatexConPlantilla compón o .tex final a partir do corpo xa
// xerado (o exame/documento pasado por Maxima) e da plantilla escollida.
// Con plantilla nil devolve exactamente o de sempre: preámbulo por defecto
// + corpo + \end{document}.
func documentoLatexConPlantilla(corpo, engine string, p *PlantillaLatex, nomeDoc string, tip *TipografiaOpts) (string, []string, error) {
	// tamaño/interliñado van xusto tras \begin{document}: aplícanse sempre,
	// aínda que a plantilla traia \documentclass propio. babelListquotFix
	// (latexdoc.go) vai igual de "sempre" - arranxa un choque real entre
	// babel galego/castelán e enumitem que rompe calquera <ol>/<ul> de dous
	// ou máis elementos ("Incomplete \iffalse"), tanto co preámbulo por
	// defecto coma cunha plantilla propia.
	corpo = babelListquotFix + tipografiaCorpo(tip) + corpo
	// familia de fonte: vai no preámbulo, canda a liña de babel. Só cando o
	// preámbulo o pon Yang - se a plantilla ten \documentclass propio, é ela
	// quen manda na fonte (só se lle aplica o tamaño/interliñado de arriba).
	fonte := tipografiaPreambulo(tip, engine)
	if p == nil {
		return fmt.Sprintf(preambuloPorDefecto(engine), babelLine()+fonte) + corpo + latexPostamble, nil, nil
	}
	if !strings.Contains(p.Latex, marcadorCorpo) {
		return "", nil, fmt.Errorf("a plantilla %q non leva %s", p.Nome, marcadorCorpo)
	}
	texto, avisos := substituirVariables(p.Latex, p, variablesAutomaticas(nomeDoc), baleiroLatex, cas.EscapeLatexProse)
	// O corpo insérese DESPOIS de substituír as variables: así un {{...}}
	// que aparecese no propio documento do profesorado queda intacto.
	doc := strings.ReplaceAll(texto, marcadorCorpo, corpo)
	if strings.Contains(doc, `\documentclass`) {
		return inxectarPreambuloObrigatorio(doc, engine), avisos, nil
	}
	// Fragmento: a plantilla é só cabeceira/pé, o preámbulo pono Yang.
	return fmt.Sprintf(preambuloPorDefecto(engine), babelLine()+fonte) + doc + latexPostamble, avisos, nil
}

// documentoMarkdownConPlantilla é o equivalente para .md/.docx/.odt. Sen
// plantilla (ou con plantilla sen parte Markdown) devolve o corpo tal cal.
func documentoMarkdownConPlantilla(corpo string, p *PlantillaLatex, nomeDoc string) (string, []string) {
	if p == nil || strings.TrimSpace(p.Markdown) == "" {
		return corpo, nil
	}
	texto, avisos := substituirVariables(p.Markdown, p, variablesAutomaticas(nomeDoc), "", nil)
	return strings.ReplaceAll(texto, marcadorCorpo, corpo), avisos
}

// copiarImaxesPlantilla deixa as imaxes da plantilla onde as vai buscar o
// .tex que se está a compilar: a carón do propio doc.tex (para que o
// \includegraphics{logo.png} que escribiu o profesorado funcione sen rutas)
// e tamén dentro de images/, que é onde markdowndoc.go/latexdoc.go poñen xa
// as imaxes de <PLOT>/<IMG> - así o mesmo nome vale nos dous formatos.
func copiarImaxesPlantilla(p *PlantillaLatex, destDir string) error {
	return escribirImaxesPlantilla(p, destDir, filepath.Join(destDir, "images"))
}

// copiarImaxesPlantillaMarkdown só usa images/: a diferenza do .tex (que se
// compila nun temporal), o .md exportado escríbese no cartafol REAL do
// profesorado, e non hai por que espallarlle alí o logo solto cando o
// Markdown xa o referencia coma images/logo.png.
func copiarImaxesPlantillaMarkdown(p *PlantillaLatex, destDir string) error {
	return escribirImaxesPlantilla(p, filepath.Join(destDir, "images"))
}

func escribirImaxesPlantilla(p *PlantillaLatex, dirs ...string) error {
	if p == nil {
		return nil
	}
	orixe, err := plantillaImaxesDir(p.ID)
	if err != nil {
		return err
	}
	entradas, err := os.ReadDir(orixe)
	if err != nil {
		return nil // a plantilla non ten imaxes
	}
	for _, e := range entradas {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(orixe, e.Name()))
		if err != nil {
			return err
		}
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// motorParaPlantilla: unha plantilla que use fontspec/fontes do sistema
// precisa xelatex aínda que en Opcións estea pdflatex, e ao revés. Baleiro
// = manda Opcións, coma sempre.
func motorParaPlantilla(p *PlantillaLatex, porDefecto string) string {
	if p != nil && strings.TrimSpace(p.Motor) != "" {
		return strings.TrimSpace(p.Motor)
	}
	return porDefecto
}

// corpoDemoPlantilla é o "documento de mentira" que se compila ao
// previsualizar unha plantilla no seu editor: leva as pezas típicas dun
// exame (título, enunciado numerado, fórmula) para que se vexa como quedan
// dentro da maqueta. Non pasa por Maxima a propósito - previsualizar unha
// cabeceira ten que ser instantáneo e funcionar aínda que Maxima non estea
// instalado.
const corpoDemoPlantilla = `\textbf{1.} Calcula a derivada de $f(x)=x^{2}+3x-2$ e representa a función.

\vspace{3mm}
$f'(x) = 2x+3$

\vspace{5mm}
\textbf{2.} Isto é só unha vista previa da plantilla: aquí, dentro da
maqueta, é onde vai aparecer o documento real.
`

// PrevisualizarPlantilla compila a plantilla cun documento de exemplo e
// devolve as páxinas coma imaxes, exactamente polo mesmo camiño ca un Xerar
// normal (documentoLatexConPlantilla + compileLatexAuto) - así o que se ve
// na vista previa é o que vai saír de verdade, erros de LaTeX incluídos. A
// plantilla NON ten por que estar gardada; se xa o está, tamén se copian as
// súas imaxes.
func (a *App) PrevisualizarPlantilla(p PlantillaLatex) (GeneratePDFResult, error) {
	result := GeneratePDFResult{}
	if strings.TrimSpace(p.Latex) == "" {
		return result, fmt.Errorf("a plantilla non ten código LaTeX")
	}
	if !strings.Contains(p.Latex, marcadorCorpo) {
		return result, fmt.Errorf("o LaTeX da plantilla ten que levar %s no sitio onde vai o documento", marcadorCorpo)
	}
	p.Variables = normalizarVariables(p.Variables)

	tmpDir, err := os.MkdirTemp("", "matexe-plantilla-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tmpDir)

	engine := strings.TrimSpace(motorParaPlantilla(&p, a.settings.LatexEngine))
	if engine == "" {
		engine = "pdflatex"
	}
	pdf, texSource, log, _, avisos, err := compileLatexAuto(tmpDir, corpoDemoPlantilla, engine, &p, "Documento de exemplo", nil)
	result.Log = log
	result.LatexSource = texSource
	result.Warnings = avisos
	if err != nil {
		// Mesmo criterio ca GeneratePDF: probar unha plantilla é o momento
		// máis probable de descubrir que falta un paquete (ver
		// paquetelatex.go).
		anotarPaqueteQueFalta(log)
		return result, err
	}
	limparPaqueteQueFalta()
	result.PDFBase64 = base64.StdEncoding.EncodeToString(pdf)
	if commandExists("pdftoppm") {
		if images, imgErr := pdfPageImages(tmpDir, filepath.Join(tmpDir, "doc.pdf")); imgErr == nil {
			result.PageImages = images
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("non se puido xerar a vista previa: %v", imgErr))
		}
	}
	return result, nil
}
