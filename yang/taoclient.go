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
)

// clienteTao pushes the Markdown Yang just generated straight into Tao's
// "Tarefa" tab, over Tao's own local API (docs/api-tao.md,
// POST /api/v1/tarefa) — the Yang-side half of the pipe Tao sets up in
// tao/yang_launch.go. Built directly from the TAO_CONTEXTO Tao handed Yang
// at launch (contexto_tao.go), never from a discovery file: this is a
// one-shot pipe tied to a single launch, not a general "find any Tao on
// this machine" mechanism — deliberately at the opposite end of the
// isolation CLAUDE.md documents for Xesta (Yang is not Xesta, and this
// pipe only exists because Tao itself dialled it, not because Yang went
// looking for Tao).
type clienteTao struct {
	http      *http.Client
	token     string
	materiaID string
	date      string
}

func novoClienteTao(ctx contextoTao) *clienteTao {
	socket := ctx.APISocket
	return &clienteTao{
		materiaID: ctx.MateriaID,
		date:      ctx.Date,
		token:     ctx.APIToken,
		http: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(dialCtx, "unix", socket)
				},
			},
		},
	}
}

// EnviarTarefa POSTs the GENERAL markdown to Tao's /api/v1/tarefa for the
// materia/date this context was launched for (username baleiro — docs/api-tao.md
// §11.4). Tao saves it (SaveTaskDescription) and live-updates its "Tarefa" tab —
// see tao/api_handlers.go handleSaveTarefa.
func (c *clienteTao) EnviarTarefa(ctx context.Context, markdown string) error {
	return c.enviarTarefaCorpo(ctx, markdown, "")
}

// EnviarTarefaAlumno POSTs one student's OWN override (username non baleiro) —
// docs/api-tao.md §11.4/§11.6. O reparto (tarefa_tao.go, EnviarReparto) fai unha
// chamada destas por destinatario: non hai endpoint de lote en Tao, o volume é
// pequeno (≤30 alumnos, socket local).
func (c *clienteTao) EnviarTarefaAlumno(ctx context.Context, markdown, username string) error {
	return c.enviarTarefaCorpo(ctx, markdown, username)
}

func (c *clienteTao) enviarTarefaCorpo(ctx context.Context, markdown, username string) error {
	corpo, err := json.Marshal(map[string]string{
		"materiaId": c.materiaID,
		"date":      c.date,
		"markdown":  markdown,
		"username":  username,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://tao/api/v1/tarefa", bytes.NewReader(corpo))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("non se puido conectar con Tao: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(res.Body)
		return fmt.Errorf("Tao respondeu %d: %s", res.StatusCode, string(data))
	}
	return nil
}
