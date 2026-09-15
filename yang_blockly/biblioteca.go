package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BibliotecaItem é un anaco reutilizable gardado polo profesorado: un exame
// completo, un exercicio solto, ou un bloque individual (ex.: a cabeceira
// cun nome/apelidos/logo do instituto) - ver os botóns "💾"/"📚 Biblioteca"
// en blocks.js. Contido é texto .matex cru, exactamente no mesmo dialecto
// que xa len/escriben blocks-serialize.js e Generate/GeneratePDF
// (elementsToText para "bloque"/"exercicio", documentToText para "exame"),
// así que cargalo de volta é só parseElements/parseDocument - non hai un
// formato separado que manter sincronizado.
type BibliotecaItem struct {
	ID      string `json:"id"`
	Nome    string `json:"nome"`
	Tipo    string `json:"tipo"` // "exame" | "exercicio" | "bloque"
	Contido string `json:"contido"`
	Creado  string `json:"creado"` // RFC3339, só para ordenar/amosar
}

// bibliotecaTiposValidos mirra tiposAnacoValidos (gemini_client.go): validar
// aquí evita que un ID/Tipo lixo chegue a gardarse e logo confunda o
// selector "Inserir" do frontend.
var bibliotecaTiposValidos = map[string]bool{"exame": true, "exercicio": true, "bloque": true}

// bibliotecaPath vive canda settings.json (mesmo cfg dir, ver settings.go) -
// local á máquina, nunca sincronizado nin subido a git, coma calquera outro
// dato persoal do profesorado (credenciais da IA incluídas).
func bibliotecaPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "yang")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "biblioteca.json"), nil
}

func loadBiblioteca() ([]BibliotecaItem, error) {
	path, err := bibliotecaPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []BibliotecaItem{}, nil
		}
		return nil, err
	}
	var items []BibliotecaItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func saveBiblioteca(items []BibliotecaItem) error {
	path, err := bibliotecaPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// ListarBiblioteca devolve os elementos gardados, máis recentes primeiro.
func (a *App) ListarBiblioteca() ([]BibliotecaItem, error) {
	items, err := loadBiblioteca()
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Creado > items[j].Creado })
	return items, nil
}

// GardarNaBiblioteca engade un elemento novo (exame/exercicio/bloque) á
// biblioteca local - chamado dende os botóns "💾" de blocks.js, un por
// nivel (exame enteiro, un exercicio, ou un só anaco).
func (a *App) GardarNaBiblioteca(nome, tipo, contido string) (BibliotecaItem, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return BibliotecaItem{}, fmt.Errorf("dálle un nome antes de gardar")
	}
	if !bibliotecaTiposValidos[tipo] {
		return BibliotecaItem{}, fmt.Errorf("tipo descoñecido: %s", tipo)
	}
	if strings.TrimSpace(contido) == "" {
		return BibliotecaItem{}, fmt.Errorf("non hai nada que gardar")
	}
	items, err := loadBiblioteca()
	if err != nil {
		return BibliotecaItem{}, err
	}
	item := BibliotecaItem{
		ID:      strconv.FormatInt(time.Now().UnixNano(), 36),
		Nome:    nome,
		Tipo:    tipo,
		Contido: contido,
		Creado:  time.Now().Format(time.RFC3339),
	}
	items = append(items, item)
	if err := saveBiblioteca(items); err != nil {
		return BibliotecaItem{}, err
	}
	return item, nil
}

// EliminarDaBiblioteca quita un elemento pola súa ID - un no-op silencioso
// se xa non existe (dobre clic en "eliminar", ou dous procesos Yang
// abertos á vez), non hai por que tratalo coma erro.
func (a *App) EliminarDaBiblioteca(id string) error {
	items, err := loadBiblioteca()
	if err != nil {
		return err
	}
	out := items[:0]
	for _, it := range items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	return saveBiblioteca(out)
}
