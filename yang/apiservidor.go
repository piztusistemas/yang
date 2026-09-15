package main

import (
	"os"

	"matexe-wails/internal/api"
)

// iniciarAPI arrinca o servidor da API interna de Yang (yang/internal/api).
// SEMPRE arrinca — a diferenza da porta pública documentada en
// yang/docs/api-yang.md §3 (pensada para módulos externos, só ten sentido
// con Piztu detrás): a comunicación entre o editor e a xanela de resultado
// pasa por aquí agora (ver xanelaresultado.go, §11 do documento), así que a
// API ten que estar dispoñible mesmo executando Yang á man, sen Piztu.
//
// Cando hai contexto de Piztu (PIZTU_CONTEXTO, ver contexto.go), o socket
// vive no tmp_dir compartido — descubrible por módulos externos. Sen
// contexto (desenvolvemento, ou Yang executado á man), vive nun directorio
// temporal propio desta instancia, borrado en pecharAPI: segue funcionando
// para o uso interno, simplemente non hai nada público que descubrir.
func (a *App) iniciarAPI() {
	tmpDir, propio, err := tmpDirParaAPI()
	if err != nil {
		println("Yang: non se puido preparar un directorio para a API interna:", err.Error())
		return
	}

	servidor := api.Novo(&nucleoAPI{app: a}, tmpDir, VersionLocalYang())
	if err := servidor.Escoitar(); err != nil {
		println("Yang: non se puido arrincar a API interna:", err.Error())
		if propio {
			os.RemoveAll(tmpDir)
		}
		return
	}
	a.api = servidor
	a.apiTmpDir = tmpDir
	a.apiTmpDirPropio = propio
	a.clienteAPI = novoClienteAPIInterno(servidor.RutaSocket(), servidor.Token())
}

// pecharAPI para o servidor e borra o socket/ficheiro de descubrimento (e o
// directorio temporal propio, se se creou un). Chamado dende
// ServiceShutdown.
func (a *App) pecharAPI() {
	if a.api != nil {
		a.api.Pechar()
	}
	if a.apiTmpDirPropio && a.apiTmpDir != "" {
		os.RemoveAll(a.apiTmpDir)
	}
}

// tmpDirParaAPI devolve onde debe vivir o socket da API interna: o tmp_dir
// compartido de Piztu se hai contexto de lanzamento (propio=false, non hai
// que borralo — é de Piztu), ou un directorio temporal novo desta instancia
// (propio=true, hai que borralo en pecharAPI).
func tmpDirParaAPI() (dir string, propio bool, err error) {
	if ctx, ok := lerContextoLanzamento(); ok {
		return ctx.TmpDir, false, nil
	}
	dir, err = os.MkdirTemp("", "yang-api-*")
	return dir, true, err
}
