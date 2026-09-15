package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App é o núcleo do instalador de Yang: expón métodos chamables desde o
// frontend (bindings, xerados por `wails build` en
// frontend/wailsjs/go/main/App.js) e emite eventos de progreso durante os
// pasos longos, mesmo patrón que installer/app.go (o instalador de Piztu).
type App struct {
	ctx context.Context
	tr  *Translator

	// permisoDenegado: true cando main() non puido reexecutarse como root
	// (pkexec non dispoñible ou cancelado) en Linux. O frontend, ao arrincar,
	// comproba PermisoDenegado() e amosa só a pantalla de instrucións.
	permisoDenegado bool
}

func NewApp() *App {
	return &App{tr: NewTranslator(idiomaFallback)}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ── Idioma ────────────────────────────────────────────────────────────────────

func (a *App) Idioma() string                { return a.tr.Idioma() }
func (a *App) SetIdioma(code string)         { a.tr.SetIdioma(code) }
func (a *App) Idiomas() []IdiomaInfo         { return idiomasDispo }
func (a *App) Traducions() map[string]string { return a.tr.All() }

// ── Estado xeral ──────────────────────────────────────────────────────────────

func (a *App) EsRoot() bool          { return esRoot() }
func (a *App) PermisoDenegado() bool { return a.permisoDenegado }
func (a *App) DestDir() string       { return destDir }
func (a *App) Version() string       { return strings.TrimSpace(versionInstalador) }

func (a *App) InstalacionDesc() string { return fmt.Sprintf(a.tr.T("inst.desc"), destDir) }
func (a *App) CompletadoDesc() string  { return fmt.Sprintf(a.tr.T("fin.desc"), destDir) }

// ── Paso "Instalación" ──────────────────────────────────────────────────────

// Instalar executa a instalación completa en segundo plano e emite cada
// liña por "inst_log"; ao rematar emite "inst_completo" con {ok, erro}.
func (a *App) Instalar() {
	go func() {
		emit := func(line string) {
			wruntime.EventsEmit(a.ctx, "inst_log", strings.Trim(line, "\n"))
		}
		if err := a.doInstall(emit); err != nil {
			erro := fmt.Sprintf(a.tr.T("inst.erro"), err.Error())
			emit(erro)
			wruntime.EventsEmit(a.ctx, "inst_completo", map[string]any{"ok": false, "erro": erro})
			return
		}
		emit(a.tr.T("inst.ok"))
		wruntime.EventsEmit(a.ctx, "inst_completo", map[string]any{"ok": true, "erro": ""})
	}()
}

func (a *App) doInstall(log func(string)) error {
	log(fmt.Sprintf(a.tr.T("inst.log.extraendo"), destDir))
	if err := extractFiles(log); err != nil {
		return fmt.Errorf("extraendo ficheiros: %w", err)
	}
	if err := fixOwnership(); err != nil {
		log("⚠️ non se puido axustar o propietario de " + destDir + ": " + err.Error())
	}
	log(a.tr.T("inst.log.extraidos"))

	log(a.tr.T("inst.log.deps"))
	if err := instalarDependencias(log); err != nil {
		// Non aborta a instalación: Yang segue funcionando (agás compilar
		// PDFs) e o propio botón "Instalar" en Opcións permite reintentalo.
		log(fmt.Sprintf(a.tr.T("inst.log.depsfail"), err.Error()))
	} else {
		log(a.tr.T("inst.log.depsok"))
	}

	if err := escribirIdiomaPorDefecto(a.tr.Idioma()); err != nil {
		log("⚠️ non se puido preconfigurar o idioma de Yang: " + err.Error())
	}

	// En Windows non existe o bit de execución (chmod aí só tocaría o
	// atributo de só-lectura, que xa está ben por defecto): nada que facer.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(destDir, nomeExecutable()), 0755); err != nil {
			log("⚠️ non se puido marcar yang coma executable: " + err.Error())
		}
	}

	// En macOS, deixar o bundle recén extraído en condicións de abrirse:
	// quitarlle a corentena de Gatekeeper e selar de novo a sinatura. Faise
	// aquí e non só en CrearAccesosDirectos porque sen isto a app podería
	// non arrincar, e ese botón é opcional (no-op fóra de macOS, ver
	// macos_darwin.go).
	if err := prepararBundleMacOS(log); err != nil {
		log("⚠️ " + err.Error())
	}

	return nil
}

// ── Paso "Completado" ────────────────────────────────────────────────────────

// CrearAccesosDirectos escribe a icona e os accesos directos (.desktop) do
// sistema e do escritorio.
func (a *App) CrearAccesosDirectos() error {
	return createShortcuts()
}

// ── Zona de perigo: desinstalación ──────────────────────────────────────────

// DesinstalarYang elimina Yang deste equipo. borrarDeps controla se ademais
// se desinstalan Maxima/LaTeX (paquetes de sistema, opcional). Emite
// progreso por "desinst_log" e remata con "desinst_completo" {ok, erro}.
func (a *App) DesinstalarYang(borrarDeps bool) {
	go func() {
		emit := func(line string) {
			wruntime.EventsEmit(a.ctx, "desinst_log", strings.Trim(line, "\n"))
		}
		if err := desinstalarYangCompleto(emit, borrarDeps); err != nil {
			erro := fmt.Sprintf(a.tr.T("desinst.erro"), err.Error())
			emit(erro)
			wruntime.EventsEmit(a.ctx, "desinst_completo", map[string]any{"ok": false, "erro": erro})
			return
		}
		emit(a.tr.T("desinst.ok"))
		wruntime.EventsEmit(a.ctx, "desinst_completo", map[string]any{"ok": true, "erro": ""})
	}()
}
