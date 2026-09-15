package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// TaoContextoActivo reports whether Yang was launched by Tao to edit a
// session's task material (see contexto_tao.go). The frontend uses this to
// decide whether to offer "Enviar a Tao" at all — bound as a plain getter
// (no request body) so it's cheap to poll once at startup.
func (a *App) TaoContextoActivo() bool {
	return a.taoContextoActiva
}

// TaoAlumnos returns the roster Tao handed Yang at launch (docs/api-tao.md
// §11.4) — empty when TaoContextoActivo is false, or when Tao couldn't
// resolve the roster for that materia (best-effort on Tao's side, see
// tao/yang_launch.go alumnosDaMateria). The frontend uses this to precargar
// "Iteracións" con len(alumnos) e para o reparto de EnviarReparto: cada
// variante xerada empaŕellase co alumno na mesma posición desta lista.
func (a *App) TaoAlumnos() []AlumnoTao {
	return a.taoContexto.Alumnos
}

// TaoAlumnoIndividual returns the username Tao asked to edit individually
// (contexto_tao.go, Username) — "" when Yang is editing the general task (or
// wasn't launched by Tao at all). The frontend uses this to swap "Enviar a
// Tao" for "Gardar para <alumno>" (EnviarATarefaTaoAlumno) and to hide
// "Repartir" (non ten sentido repartir cando xa se edita un só alumno).
func (a *App) TaoAlumnoIndividual() string {
	return a.taoContexto.Username
}

// EnviarATarefaTaoResult mirrors GenerateMarkdownResult (markdowndoc.go) but
// has no Path: nothing is written to disk here, the content goes straight
// to Tao.
type EnviarATarefaTaoResult struct {
	Warnings []string `json:"warnings"`
}

// EnviarATarefaTao generates the current document's Markdown — same core
// as ExportMarkdown and the internal API's POST /markdown
// (generateMarkdownBody, markdowndoc.go) — and pushes it straight into the
// session's "Tarefa" tab in Tao, live, instead of writing a .md file. See
// tao/yang_launch.go and docs/api-tao.md for the other end of the pipe.
//
// Requires Yang to have been launched by Tao (TaoContextoActivo). Images
// referenced via <PLOT>/<TIKZ>/<IMG> are not carried over — same known
// limitation as the content-only POST /api/v1/markdown of Yang's own API
// (yang/docs/api-yang.md §7.1): the Markdown here is generated into a
// throwaway temp dir, never a real folder a "images/" subpath could
// survive in.
func (a *App) EnviarATarefaTao(req GenerateMarkdownRequest) (EnviarATarefaTaoResult, error) {
	result := EnviarATarefaTaoResult{}
	if !a.taoContextoActiva || a.clienteTao == nil {
		return result, fmt.Errorf("Yang non foi aberto dende Tao — non hai onde enviar a tarefa")
	}

	tmpDir, err := os.MkdirTemp("", "yang-tao-md-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tmpDir)

	body, warnings, err := a.generateMarkdownBody(req, tmpDir)
	result.Warnings = warnings
	if err != nil {
		return result, err
	}

	if err := a.clienteTao.EnviarTarefa(context.Background(), body); err != nil {
		return result, err
	}
	return result, nil
}

// EnviarATarefaTaoAlumno is EnviarATarefaTao but for the ONE student Tao
// asked to edit (TaoAlumnoIndividual) — pushes with EnviarTarefaAlumno so it
// lands in that student's own override, never touching the general text.
// Requires TaoAlumnoIndividual() non baleiro, same as EnviarATarefaTao
// requires TaoContextoActivo.
func (a *App) EnviarATarefaTaoAlumno(req GenerateMarkdownRequest) (EnviarATarefaTaoResult, error) {
	result := EnviarATarefaTaoResult{}
	if !a.taoContextoActiva || a.clienteTao == nil {
		return result, fmt.Errorf("Yang non foi aberto dende Tao — non hai onde enviar a tarefa")
	}
	username := strings.TrimSpace(a.taoContexto.Username)
	if username == "" {
		return result, fmt.Errorf("Yang non foi aberto para editar un alumno en concreto")
	}

	tmpDir, err := os.MkdirTemp("", "yang-tao-md-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tmpDir)

	body, warnings, err := a.generateMarkdownBody(req, tmpDir)
	result.Warnings = warnings
	if err != nil {
		return result, err
	}

	if err := a.clienteTao.EnviarTarefaAlumno(context.Background(), body, username); err != nil {
		return result, err
	}
	return result, nil
}

// RepartoResultado is one destinatario's outcome within EnviarReparto — a
// alumno pode fallar independentemente dos demais (docs/api-tao.md §11.6),
// así que a UI de Yang sabe exactamente cales reintentar en vez dun erro
// todo-ou-nada.
type RepartoResultado struct {
	Username string `json:"username"`
	Ok       bool   `json:"ok"`
	Erro     string `json:"erro,omitempty"`
}

// EnviarRepartoResult mirrors EnviarATarefaTaoResult but for the per-student
// batch (docs/api-tao.md §11.5/§11.6).
type EnviarRepartoResult struct {
	Warnings   []string           `json:"warnings"`
	Resultados []RepartoResultado `json:"resultados"`
}

// EnviarReparto generates ONE variant per username in usernames (semente
// req.Seed+i para o i-ésimo, req.Iterations forzado a 1 en cada chamada — un
// Markdown por destinatario, nunca o documento multi-variante concatenado que
// xera EnviarATarefaTao) e empúxao decontado ao propio override dese alumno
// en Tao (taoclient.go, EnviarTarefaAlumno) — docs/api-tao.md §11.5/§11.6.
//
// O reparto en si (que username vai en que posición, se algún se repite ou
// queda fóra) decídeo o chamador (a UI): esta función só xera e envía na
// orde recibida, non valida usernames contra TaoAlumnos.
//
// Require TaoContextoActivo, mesmo gate que EnviarATarefaTao — toda esta UI
// de reparto só existe dentro do ecosistema (docs/api-tao.md §11.2).
func (a *App) EnviarReparto(req GenerateMarkdownRequest, usernames []string) (EnviarRepartoResult, error) {
	result := EnviarRepartoResult{}
	if !a.taoContextoActiva || a.clienteTao == nil {
		return result, fmt.Errorf("Yang non foi aberto dende Tao — non hai onde enviar o reparto")
	}
	if len(usernames) == 0 {
		return result, fmt.Errorf("non hai alumnado ao que repartir")
	}

	baseSeed := req.Seed
	for i, username := range usernames {
		username = strings.TrimSpace(username)
		if username == "" {
			continue
		}

		variante := req
		variante.Seed = baseSeed + i
		variante.Iterations = 1

		tmpDir, err := os.MkdirTemp("", "yang-tao-md-*")
		if err != nil {
			result.Resultados = append(result.Resultados, RepartoResultado{Username: username, Erro: err.Error()})
			continue
		}
		body, warnings, err := a.generateMarkdownBody(variante, tmpDir)
		os.RemoveAll(tmpDir)
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil {
			result.Resultados = append(result.Resultados, RepartoResultado{Username: username, Erro: err.Error()})
			continue
		}
		if err := a.clienteTao.EnviarTarefaAlumno(context.Background(), body, username); err != nil {
			result.Resultados = append(result.Resultados, RepartoResultado{Username: username, Erro: err.Error()})
			continue
		}
		result.Resultados = append(result.Resultados, RepartoResultado{Username: username, Ok: true})
	}
	return result, nil
}
