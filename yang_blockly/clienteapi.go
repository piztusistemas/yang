package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"matexe-wails/internal/api"
)

// clienteAPIInterno fala coa API interna de Yang (yang/internal/api) sobre
// o seu propio socket Unix — a canle que agora usan
// PublicarEstadoResultado/GetEstadoResultado/AccionResultado*
// (xanelaresultado.go) para comunicar o editor coa xanela de resultado, en
// vez de ler un campo compartido en memoria directamente (ver
// yang/docs/api-yang.md §11). Deliberadamente á parte de yangclient/ (o SDK
// público para módulos EXTERNOS): estes endpoints son un mecanismo interno
// de UI de Yang, non un contrato estable para terceiros.
type clienteAPIInterno struct {
	http  *http.Client
	token string
}

func novoClienteAPIInterno(socketPath, token string) *clienteAPIInterno {
	return &clienteAPIInterno{
		token: token,
		http: &http.Client{
			Timeout: 5 * time.Minute, // recompilar PDF/DOCX pode tardar
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

func (c *clienteAPIInterno) PublicarResultado(ctx context.Context, req api.PublicarResultadoRequest) (api.SnapshotResultado, error) {
	var res api.SnapshotResultado
	err := c.facer(ctx, http.MethodPost, "/api/v1/resultado", req, &res)
	return res, err
}

func (c *clienteAPIInterno) Resultado(ctx context.Context) (api.SnapshotResultado, error) {
	var res api.SnapshotResultado
	err := c.facer(ctx, http.MethodGet, "/api/v1/resultado", nil, &res)
	return res, err
}

func (c *clienteAPIInterno) ResultadoPDF(ctx context.Context) (api.ResultadoPDFResponse, error) {
	var res api.ResultadoPDFResponse
	err := c.facer(ctx, http.MethodGet, "/api/v1/resultado/pdf", nil, &res)
	return res, err
}

func (c *clienteAPIInterno) ResultadoTex(ctx context.Context) (api.ResultadoTexResponse, error) {
	var res api.ResultadoTexResponse
	err := c.facer(ctx, http.MethodGet, "/api/v1/resultado/tex", nil, &res)
	return res, err
}

func (c *clienteAPIInterno) ResultadoFonte(ctx context.Context) (api.ResultadoFonteResponse, error) {
	var res api.ResultadoFonteResponse
	err := c.facer(ctx, http.MethodGet, "/api/v1/resultado/fonte", nil, &res)
	return res, err
}

func (c *clienteAPIInterno) facer(ctx context.Context, metodo, ruta string, corpo any, destino any) error {
	var lector io.Reader
	if corpo != nil {
		data, err := json.Marshal(corpo)
		if err != nil {
			return err
		}
		lector = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, "http://unix"+ruta, lector)
	if err != nil {
		return err
	}
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("non se puido contactar coa API interna de Yang: %w", err)
	}
	defer resp.Body.Close()

	corpoResposta, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return erroAPIInterna(resp.StatusCode, corpoResposta)
	}
	if destino == nil || len(corpoResposta) == 0 {
		return nil
	}
	return json.Unmarshal(corpoResposta, destino)
}

func erroAPIInterna(status int, corpo []byte) error {
	var envoltorio struct {
		Erro struct {
			Codigo  string `json:"codigo"`
			Mensaxe string `json:"mensaxe"`
		} `json:"erro"`
	}
	if json.Unmarshal(corpo, &envoltorio) == nil && envoltorio.Erro.Mensaxe != "" {
		return fmt.Errorf("API interna de Yang (%s, HTTP %d): %s", envoltorio.Erro.Codigo, status, envoltorio.Erro.Mensaxe)
	}
	return fmt.Errorf("API interna de Yang: HTTP %d", status)
}
