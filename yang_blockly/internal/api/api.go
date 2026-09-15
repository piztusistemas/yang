package api

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

// NomeSocket é o ficheiro do socket dentro do tmp_dir compartido de Piztu.
const NomeSocket = "yang-api.sock"

// NomeDescubrimento é o ficheiro onde Yang publica socket+token para que
// calquera proceso local o atope (ver yang/docs/api-yang.md §4) — 0600
// porque, a diferenza do socket, leva o token en claro.
const NomeDescubrimento = "yang-api.json"

// VersionAPI identifica o contrato exposto (ver GET /api/v1/whoami). Sobe
// cando hai un cambio incompatible nas rutas actuais — un endpoint novo non
// a move.
const VersionAPI = "v1"

// Servidor expón a API de Yang sobre un socket Unix propio. Unha soa
// instancia por proceso, creada e arrincada en ServiceStartup, pechada en
// ServiceShutdown (ver yang/apiservidor.go).
type Servidor struct {
	nucleo      Nucleo
	tmpDir      string
	versionYang string
	token       string
	listener    net.Listener

	// resultado é o estado do modo dúas xanelas (editor ↔ xanela de
	// resultado, ver resultado.go e yang/xanelaresultado.go) — mecanismo
	// interno, non parte do catálogo público (yang/docs/api-yang.md §11).
	resultado estadoResultado
}

// Novo crea o servidor (sen escoitar aínda: ver Escoitar). tmpDir é o
// tmp_dir compartido que Yang recibe de Piztu en PIZTU_CONTEXTO (ver
// yang/contexto.go) — quen chama xa comprobou que non está baleiro.
func Novo(nucleo Nucleo, tmpDir, versionYang string) *Servidor {
	return &Servidor{nucleo: nucleo, tmpDir: tmpDir, versionYang: versionYang}
}

// RutaSocket é a ruta completa do socket.
func (s *Servidor) RutaSocket() string {
	return filepath.Join(s.tmpDir, NomeSocket)
}

func (s *Servidor) rutaDescubrimento() string {
	return filepath.Join(s.tmpDir, NomeDescubrimento)
}

// Token devolve o token vixente desta sesión. Uso exclusivo do cliente
// interno de Yang (yang/clienteapi.go), que constrúe o seu propio cliente
// en memoria directamente con RutaSocket()+Token() en vez de ler o
// ficheiro de descubrimento coma faría un módulo externo — evita depender
// de PIZTU_CONTEXTO só para que Yang fale consigo mesmo (ver
// yang/docs/api-yang.md §3 e §11).
func (s *Servidor) Token() string {
	return s.token
}

// Escoitar xera o token desta sesión, abre o socket Unix, publica o
// ficheiro de descubrimento e arrinca a servir en fondo. Idempotente fronte
// a un peche anterior non limpo: bórrase calquera socket vello antes de
// crear o novo (senón net.Listen falla con "address already in use").
func (s *Servidor) Escoitar() error {
	s.token = novoToken()

	ruta := s.RutaSocket()
	_ = os.Remove(ruta)
	l, err := net.Listen("unix", ruta)
	if err != nil {
		return err
	}
	// 0660: mesmo criterio ca o socket de Piztu — primeira barreira (só
	// procesos do mesmo usuario/grupo poden sequera conectar); a segunda é
	// o token, ver auth.go.
	if err := os.Chmod(ruta, 0o660); err != nil {
		l.Close()
		return err
	}
	s.listener = l

	if err := s.escribirDescubrimento(); err != nil {
		l.Close()
		_ = os.Remove(ruta)
		return err
	}

	go func() {
		_ = http.Serve(l, s.mux())
	}()
	return nil
}

func (s *Servidor) escribirDescubrimento() error {
	data, err := json.MarshalIndent(map[string]string{
		"socket":      s.RutaSocket(),
		"token":       s.token,
		"api_version": VersionAPI,
	}, "", "  ")
	if err != nil {
		return err
	}
	// 0600: máis estrito có 660 do socket, porque este ficheiro leva o
	// token en claro (ver yang/docs/api-yang.md §4).
	return os.WriteFile(s.rutaDescubrimento(), data, 0o600)
}

// Pechar deixa de escoitar e borra o socket e o ficheiro de descubrimento.
// Chamado en ServiceShutdown.
func (s *Servidor) Pechar() {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	_ = os.Remove(s.RutaSocket())
	_ = os.Remove(s.rutaDescubrimento())
}

func (s *Servidor) mux() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/whoami", s.recuperar(s.autenticar(s.handleWhoami)))
	mux.Handle("POST /api/v1/xerar", s.recuperar(s.autenticar(s.handleXerar)))
	mux.Handle("POST /api/v1/pdf", s.recuperar(s.autenticar(s.handlePDF)))
	mux.Handle("POST /api/v1/markdown", s.recuperar(s.autenticar(s.handleMarkdown)))
	mux.Handle("POST /api/v1/docx", s.recuperar(s.autenticar(s.handleDocx)))
	mux.Handle("POST /api/v1/odt", s.recuperar(s.autenticar(s.handleOdt)))
	mux.Handle("GET /api/v1/biblioteca", s.recuperar(s.autenticar(s.handleBibliotecaListar)))
	mux.Handle("POST /api/v1/biblioteca", s.recuperar(s.autenticar(s.handleBibliotecaGardar)))
	mux.Handle("DELETE /api/v1/biblioteca/{id}", s.recuperar(s.autenticar(s.handleBibliotecaEliminar)))
	// Internos (ver resultado.go, yang/docs/api-yang.md §11): mesmo socket
	// e token, non parte do catálogo público para módulos externos.
	mux.Handle("POST /api/v1/resultado", s.recuperar(s.autenticar(s.handlePublicarResultado)))
	mux.Handle("GET /api/v1/resultado", s.recuperar(s.autenticar(s.handleGetResultado)))
	mux.Handle("GET /api/v1/resultado/pdf", s.recuperar(s.autenticar(s.handleResultadoPDF)))
	mux.Handle("GET /api/v1/resultado/tex", s.recuperar(s.autenticar(s.handleResultadoTex)))
	mux.Handle("GET /api/v1/resultado/fonte", s.recuperar(s.autenticar(s.handleResultadoFonte)))
	return mux
}
