package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAPIRoundTrip cobre o camiño completo da API interna de Yang de punta
// a punta: le PIZTU_CONTEXTO, arrinca o servidor no socket Unix, publica o
// ficheiro de descubrimento, e responde whoami/biblioteca por HTTP real
// sobre ese socket cun token válido — e rexeita un token incorrecto. Non
// toca Maxima/LaTeX/GTK (xerar/pdf/markdown/docx/odt quedan fóra, ver
// TestBibliotecaRoundTrip para eses camiños internos xa cubertos á parte).
func TestAPIRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configIllado(t) // illa a biblioteca/plantillas do disco real (ver plantillas_test.go)

	contexto := filepath.Join(t.TempDir(), "contexto.json")
	dados, err := json.Marshal(map[string]string{"tmp_dir": tmpDir})
	if err != nil {
		t.Fatalf("marshal contexto: %v", err)
	}
	if err := os.WriteFile(contexto, dados, 0o600); err != nil {
		t.Fatalf("escribir contexto: %v", err)
	}
	t.Setenv("PIZTU_CONTEXTO", contexto)

	a := &App{}
	a.iniciarAPI()
	if a.api == nil {
		t.Fatal("iniciarAPI non arrincou o servidor (contexto válido presente)")
	}
	defer a.pecharAPI()

	descData, err := os.ReadFile(filepath.Join(tmpDir, "yang-api.json"))
	if err != nil {
		t.Fatalf("ler ficheiro de descubrimento: %v", err)
	}
	var desc struct {
		Socket string `json:"socket"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(descData, &desc); err != nil {
		t.Fatalf("parsear ficheiro de descubrimento: %v", err)
	}
	if desc.Socket == "" || desc.Token == "" {
		t.Fatal("ficheiro de descubrimento sen socket/token")
	}

	cli := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", desc.Socket)
			},
		},
	}

	facer := func(metodo, ruta, token string) *http.Response {
		req, err := http.NewRequest(metodo, "http://unix"+ruta, nil)
		if err != nil {
			t.Fatalf("crear petición %s %s: %v", metodo, ruta, err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatalf("facer petición %s %s: %v", metodo, ruta, err)
		}
		return resp
	}

	// Sen token válido -> 401, nunca 404 (a ruta existir non é segredo).
	if resp := facer("GET", "/api/v1/whoami", "token-lixo"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("whoami con token inválido: esperaba 401, obtiven %d", resp.StatusCode)
	}

	// whoami confirma servizo+versión coa API.
	resp := facer("GET", "/api/v1/whoami", desc.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("whoami: esperaba 200, obtiven %d", resp.StatusCode)
	}
	var whoami struct {
		Servizo    string `json:"servizo"`
		APIVersion string `json:"api_version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&whoami); err != nil {
		t.Fatalf("decodificar whoami: %v", err)
	}
	resp.Body.Close()
	if whoami.Servizo != "yang" || whoami.APIVersion != "v1" {
		t.Errorf("whoami inesperado: %+v", whoami)
	}

	// Biblioteca baleira ao arrincar.
	resp = facer("GET", "/api/v1/biblioteca", desc.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("listar biblioteca: esperaba 200, obtiven %d", resp.StatusCode)
	}
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decodificar biblioteca: %v", err)
	}
	resp.Body.Close()
	if len(items) != 0 {
		t.Errorf("esperaba biblioteca baleira, atopei %d elementos", len(items))
	}

	// /resultado antes de publicar nada: sen PDF/Tex/fonte aínda.
	if resp := facer("GET", "/api/v1/resultado/pdf", desc.Token); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("resultado/pdf sen publicar: esperaba 400, obtiven %d", resp.StatusCode)
	}

	// Publicar e ler de volta - mesmo corpo que envía
	// PublicarEstadoResultado (xanelaresultado.go).
	corpo, _ := json.Marshal(map[string]any{
		"source": "<EX>ola</EX>", "seed": 3, "iterations": 1,
		"nomeBase": "exame1", "pageImages": []string{"data:img1"},
		"warnings": []string{}, "pdfBase64": "UERGZmFsc28=", "texSource": "\\documentclass{article}",
	})
	req, _ := http.NewRequest("POST", "http://unix/api/v1/resultado", bytes.NewReader(corpo))
	req.Header.Set("Authorization", "Bearer "+desc.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = cli.Do(req)
	if err != nil {
		t.Fatalf("publicar resultado: %v", err)
	}
	var snap struct {
		Version int  `json:"version"`
		HasPDF  bool `json:"hasPdf"`
		HasTex  bool `json:"hasTex"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decodificar snapshot: %v", err)
	}
	resp.Body.Close()
	if snap.Version != 1 || !snap.HasPDF || !snap.HasTex {
		t.Errorf("snapshot inesperado tras publicar: %+v", snap)
	}

	resp = facer("GET", "/api/v1/resultado", desc.Token)
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decodificar GET resultado: %v", err)
	}
	resp.Body.Close()
	if snap.Version != 1 {
		t.Errorf("GET resultado non reflicte o publicado: %+v", snap)
	}

	resp = facer("GET", "/api/v1/resultado/pdf", desc.Token)
	var pdfResp struct {
		PDFBase64 string `json:"pdfBase64"`
		NomeBase  string `json:"nomeBase"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pdfResp); err != nil {
		t.Fatalf("decodificar resultado/pdf: %v", err)
	}
	resp.Body.Close()
	if pdfResp.PDFBase64 != "UERGZmFsc28=" || pdfResp.NomeBase != "exame1" {
		t.Errorf("resultado/pdf inesperado: %+v", pdfResp)
	}

	resp = facer("GET", "/api/v1/resultado/fonte", desc.Token)
	var fonteResp struct {
		Source string `json:"source"`
		Seed   int    `json:"seed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fonteResp); err != nil {
		t.Fatalf("decodificar resultado/fonte: %v", err)
	}
	resp.Body.Close()
	if fonteResp.Source != "<EX>ola</EX>" || fonteResp.Seed != 3 {
		t.Errorf("resultado/fonte inesperado: %+v", fonteResp)
	}
}

// TestPublicarEGetEstadoResultado proba o mesmo camiño ca TestAPIRoundTrip
// pero a través dos BINDINGS que realmente chama o frontend
// (xanelaresultado.go) en vez de HTTP explícito - confirma que
// PublicarEstadoResultado/GetEstadoResultado pasan de verdade pola API
// interna (e non por un campo compartido en memoria) de punta a punta.
func TestPublicarEGetEstadoResultado(t *testing.T) {
	t.Setenv("PIZTU_CONTEXTO", "")
	a := &App{}
	a.iniciarAPI()
	defer a.pecharAPI()
	if a.clienteAPI == nil {
		t.Fatal("iniciarAPI non deixou clienteAPI listo")
	}

	if snap := a.GetEstadoResultado(); snap.Version != 0 {
		t.Errorf("esperaba version 0 antes de publicar, obtiven %+v", snap)
	}

	a.PublicarEstadoResultado(
		GenerateMarkdownRequest{Source: "<EX>ola</EX>", Seed: 1, Iterations: 1},
		"exame1", []string{"data:img1"}, nil, "cGRm", "\\documentclass{article}",
	)

	snap := a.GetEstadoResultado()
	if snap.Version != 1 || !snap.HasPDF || !snap.HasTex || len(snap.PageImages) != 1 {
		t.Errorf("GetEstadoResultado tras publicar: %+v", snap)
	}
}

// TestIniciarAPISenContextoUsaTmpDirPropio comproba que, sen PIZTU_CONTEXTO
// (execución manual, fóra de Piztu), a API interna arrinca igual nun
// directorio temporal propio - necesaria para que o modo dúas xanelas
// funcione tamén fóra de Piztu (ver §3/§11, yang/docs/api-yang.md) - e que
// pecharAPI borra ese directorio.
func TestIniciarAPISenContextoUsaTmpDirPropio(t *testing.T) {
	t.Setenv("PIZTU_CONTEXTO", "")
	a := &App{}
	a.iniciarAPI()
	if a.api == nil {
		t.Fatal("esperaba que a API arrincase igual sen PIZTU_CONTEXTO (tmp_dir propio)")
	}
	if !a.apiTmpDirPropio || a.apiTmpDir == "" {
		t.Fatal("esperaba un tmp_dir propio marcado para borrar")
	}
	if _, err := os.Stat(filepath.Join(a.apiTmpDir, "yang-api.json")); err != nil {
		t.Fatalf("ficheiro de descubrimento non atopado no tmp_dir propio: %v", err)
	}

	a.pecharAPI()
	if _, err := os.Stat(a.apiTmpDir); !os.IsNotExist(err) {
		t.Errorf("esperaba que pecharAPI borrase o tmp_dir propio, err=%v", err)
	}
}
