package main

import (
	"encoding/json"
	"os"
)

// contextoTao is what Tao writes when it launches Yang to edit a session's
// task material (see tao/yang_launch.go) — read via the TAO_CONTEXTO env
// var, same file-plus-env-var pattern as PIZTU_CONTEXTO (contexto.go), for
// the same macOS .app reason (see tao/yang_launch.go, lanzarYang). Present
// only when Tao itself opened Yang this way: opening Yang from Piztu's map,
// or by hand, leaves this unset and the "push to Tao" flow (taoclient.go)
// simply never activates — no coupling at all in that case.
type contextoTao struct {
	MateriaID string `json:"materia_id"`
	Date      string `json:"date"`
	APISocket string `json:"api_socket"`
	APIToken  string `json:"api_token"`
	// Alumnos é o alumnado da materia (docs/api-tao.md §11.4) — permite que a UI
	// de reparto (só visible con TaoContextoActivo, ver tarefa_tao.go) suxira
	// Iteracións=len(Alumnos) e empareille cada variante xerada cun alumno real.
	// Pode vir baleiro (Tao non puido resolver o alumnado): a UI simplemente non
	// suxire nada, non é un erro.
	Alumnos []AlumnoTao `json:"alumnos,omitempty"`
	// Username, cando vén cheo, significa que Tao abriu Yang para editar SÓ a
	// tarefa individual dese alumno (App.tsx, modal de excepcións → "Editar
	// con Yang" dentro do editor dun alumno) — non a tarefa xeral nin un
	// reparto a varios. Ver TaoAlumnoIndividual/EnviarATarefaTaoAlumno.
	Username string `json:"username,omitempty"`
}

// AlumnoTao é un alumno tal e como o manda Tao — espello de alumnoContexto en
// tao/yang_launch.go. Exportado (non "alumnoTao") porque Wails ten que xeralo
// coma tipo público para o binding TaoAlumnos (tarefa_tao.go).
type AlumnoTao struct {
	Username string `json:"username"`
	Nome     string `json:"nome"`
}

// lerContextoTao reads the context Tao wrote at launch, if any. ok=false
// covers both "Yang wasn't launched by Tao" and "the context is
// incomplete" — either way, nothing partial gets used.
func lerContextoTao() (contextoTao, bool) {
	var ctx contextoTao
	ruta := os.Getenv("TAO_CONTEXTO")
	if ruta == "" {
		return ctx, false
	}
	dados, err := os.ReadFile(ruta)
	if err != nil {
		return ctx, false
	}
	if err := json.Unmarshal(dados, &ctx); err != nil {
		return ctx, false
	}
	if ctx.APISocket == "" || ctx.APIToken == "" || ctx.MateriaID == "" || ctx.Date == "" {
		return ctx, false
	}
	return ctx, true
}
