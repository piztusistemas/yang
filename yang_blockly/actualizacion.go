package main

// Autoactualización do propio Yang, mesmo mecanismo có de Piztu
// (ver piztu/internal/actualizacion/actualizacion.go, do que este
// ficheiro é unha adaptación directa — Yang non pode importalo porque vive
// noutro módulo Go, e ademais a lóxica é específica: outro repositorio,
// outro prefixo de asset, outro executable).
//
// A versión instalada lese do ficheiro VERSION embebido na raíz deste
// módulo. A versión publicada consúltase no repositorio compartido de
// módulos de Piztu, piztutao/modulos (https://github.com/piztutao/modulos)
// — Yang, coma calquera outro módulo, non ten repositorio propio en GitHub.
// Iso significa que "/releases/latest" NON serve: devolvería a última
// Release do repo enteiro, sen importar de que módulo sexa. En vez diso,
// cada publicación de Yang crea a súa propia Release co tag prefixado
// "yang-<versión>" (ex.: "yang-26.9" — GitHub esixe tags únicos por repo, e
// outros módulos publican coa mesma convención, "pancho-2.1" etc.), e
// ComprobarActualizacionYang percorre TODAS as Releases do repo (máis
// recentes primeiro, que é como as devolve a API) ata atopar a primeira que
// traia un asset "yang_<versión>.zip" adxunto — a versión remota parséase
// dese NOME DE ASSET, non do tag_name, precisamente para non depender de
// que o tag sexa "só" o número de versión. O .zip contén só o executable
// "yang" novo (todo o resto — frontend, idiomas — vai embebido dentro del
// con go:embed).
//
// A Release TAMÉN ten que publicar un asset "<nome-do-zip>.sha256" (formato
// "sha256sum": o hash en hex, opcionalmente seguido do nome do ficheiro) —
// AplicarActualizacionYangEnCaliente compróbao antes de instalar nada. Sen
// isto, quen puidese publicar unha Release podería facer executar calquera
// cousa, coas mesmas credenciais ca Yang, en cada equipo que actualizase.
import (
	"archive/zip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed VERSION
var versionYangFicheiro string

const repoYang = "piztutao/modulos"

// per_page=100: cobre folgadamente as Releases de todos os módulos do repo
// compartido; se algún día se superan as 100 Releases totais faría falta
// paxinar, pero non paga a pena a complexidade mentres non chegue ese caso.
const urlReleasesYang = "https://api.github.com/repos/" + repoYang + "/releases?per_page=100"
const timeoutComprobarYang = 5 * time.Second
const timeoutDescargaYang = 60 * time.Second

// EstadoActualizacionYang é o resultado de comprobar unha actualización de
// Yang — os nomes de campo van en minúscula inicial en JSON (camelCase) para
// encaixar coa convención xa usada polo resto de structs de app.go
// (Settings, InstallStatus...) que o frontend le directamente.
type EstadoActualizacionYang struct {
	Disponible    bool   `json:"disponible"`
	VersionLocal  string `json:"versionLocal"`
	VersionRemota string `json:"versionRemota"`
	URLDescarga   string `json:"urlDescarga"` // "" se non hai asset .zip descargable
	URLChecksum   string `json:"urlChecksum"` // "" se a Release non publica "<zip>.sha256"
	Notas         string `json:"notas"`       // descrición da Release (changelog)
}

func init() {
	application.RegisterEvent[EstadoActualizacionYang]("actualizacion_yang_cambiada")
}

type assetGitHubYang struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseGitHubYang struct {
	TagName string            `json:"tag_name"`
	Body    string            `json:"body"`
	Assets  []assetGitHubYang `json:"assets"`
}

// escollerAssetsYang busca, entre os assets dunha Release, o .zip do propio
// Yang e o seu .sha256 — o prefixo "yang_" vai cravado e non abonda con
// "calquera .zip": a Release podería levar outros adxuntos e collelo
// errado (ver o mesmo problema, xa sufrido en produción con Piztu, no
// commit "Arranxar autoactualización" de piztu).
func escollerAssetsYang(assets []assetGitHubYang) (urlZip, nomeZip, urlChecksum string) {
	for _, a := range assets {
		nome := strings.ToLower(a.Name)
		if strings.HasPrefix(nome, "yang_") && strings.HasSuffix(nome, ".zip") {
			urlZip = a.BrowserDownloadURL
			nomeZip = a.Name
			break
		}
	}
	if urlZip == "" {
		return "", "", ""
	}
	for _, a := range assets {
		if strings.EqualFold(a.Name, nomeZip+".sha256") {
			urlChecksum = a.BrowserDownloadURL
			break
		}
	}
	return urlZip, nomeZip, urlChecksum
}

// versionDesdeNomeZipYang extrae o número de versión do nome do asset
// ("yang_26.9.zip" → "26.9"). Parséase do nome do asset e non do tag_name da
// Release porque o tag agora vai prefixado co nome do módulo ("yang-26.9",
// ver cabeceira do ficheiro) para non colidir con tags doutros módulos no
// mesmo repo compartido.
func versionDesdeNomeZipYang(nomeZip string) string {
	nome := strings.ToLower(nomeZip)
	nome = strings.TrimPrefix(nome, "yang_")
	nome = strings.TrimSuffix(nome, ".zip")
	return strings.TrimSpace(nome)
}

// partesVersionYang e versionMaiorYang copian a comparación "semver-lite" de
// piztu/internal/modulos.VersionMaior — pequena e autónoma dabondo como
// para non merecer facer de Yang un consumidor de piztu/internal.
func partesVersionYang(v string) ([]int, bool) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if v == "" {
		return nil, false
	}
	anacos := strings.Split(v, ".")
	partes := make([]int, 0, len(anacos))
	for _, a := range anacos {
		n, err := strconv.Atoi(strings.TrimSpace(a))
		if err != nil {
			return nil, false
		}
		partes = append(partes, n)
	}
	return partes, true
}

func versionMaiorYang(remota, local string) bool {
	r, okR := partesVersionYang(remota)
	if !okR {
		return false
	}
	l, okL := partesVersionYang(local)
	if !okL {
		l = []int{0}
	}
	for i := 0; i < len(r) || i < len(l); i++ {
		var rv, lv int
		if i < len(r) {
			rv = r[i]
		}
		if i < len(l) {
			lv = l[i]
		}
		if rv != lv {
			return rv > lv
		}
	}
	return false
}

// VersionLocalYang devolve a versión instalada de Yang.
func VersionLocalYang() string {
	return strings.TrimSpace(versionYangFicheiro)
}

// ComprobarActualizacionYang consulta as Releases de piztutao/modulos en
// GitHub, queda coa máis recente que traia un asset de Yang adxunto, e
// compáraa coa versión instalada. Nunca devolve erro cara ao chamador: se
// falla a rede, non hai releases publicadas (ou ningunha é de Yang), ou a
// resposta non é válida, devolve Disponible=false para non bloquear o
// arranque de Yang.
func ComprobarActualizacionYang() EstadoActualizacionYang {
	estado := EstadoActualizacionYang{VersionLocal: VersionLocalYang()}

	req, err := http.NewRequest(http.MethodGet, urlReleasesYang, nil)
	if err != nil {
		return estado
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	cliente := http.Client{Timeout: timeoutComprobarYang}
	resp, err := cliente.Do(req)
	if err != nil {
		return estado
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return estado
	}

	var rels []releaseGitHubYang
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return estado
	}

	// A API devolve as Releases máis recentes primeiro; o repo é compartido
	// con outros módulos, así que quedamos coa primeira que traia un asset
	// "yang_*.zip" — ver cabeceira do ficheiro.
	for _, rel := range rels {
		urlZip, nomeZip, urlChecksum := escollerAssetsYang(rel.Assets)
		if urlZip == "" {
			continue // Release doutro módulo (ou sen assets): seguinte
		}
		versionRemota := versionDesdeNomeZipYang(nomeZip)
		if versionRemota == "" {
			continue
		}
		estado.VersionRemota = versionRemota
		estado.URLDescarga = urlZip
		estado.URLChecksum = urlChecksum
		estado.Notas = rel.Body
		estado.Disponible = versionMaiorYang(versionRemota, estado.VersionLocal)
		return estado
	}
	return estado // ningunha Release do repo trae un asset de Yang
}

// nomeExecutableYang é o nome do ficheiro que ten que haber dentro do .zip
// da Release: o executable de Yang tal cal o produce "wails build".
func nomeExecutableYang() string {
	if runtime.GOOS == "windows" {
		return "yang.exe"
	}
	return "yang"
}

func descargarYang(url, destinoDir, nomeFicheiro string) (string, error) {
	if err := os.MkdirAll(destinoDir, 0o755); err != nil {
		return "", fmt.Errorf("non se puido crear %q: %w", destinoDir, err)
	}
	destino := filepath.Join(destinoDir, nomeFicheiro)

	cliente := http.Client{Timeout: timeoutDescargaYang}
	resp, err := cliente.Get(url)
	if err != nil {
		return "", fmt.Errorf("non se puido descargar a actualización: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("non se puido descargar a actualización: HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destino)
	if err != nil {
		return "", fmt.Errorf("non se puido crear %q: %w", destino, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("non se puido gardar a actualización: %w", err)
	}
	return destino, nil
}

func extraerExecutableYang(zipRuta, destDir string) (string, error) {
	zr, err := zip.OpenReader(zipRuta)
	if err != nil {
		return "", fmt.Errorf("o ficheiro descargado non é un zip válido: %w", err)
	}
	defer zr.Close()

	buscado := nomeExecutableYang()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || filepath.Base(f.Name) != buscado {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("non se puido abrir %q dentro do zip: %w", f.Name, err)
		}
		defer rc.Close()

		destino := filepath.Join(destDir, buscado)
		out, err := os.OpenFile(destino, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return "", fmt.Errorf("non se puido crear %q: %w", destino, err)
		}
		defer out.Close()
		if _, err := io.Copy(out, rc); err != nil {
			return "", fmt.Errorf("non se puido extraer %q: %w", f.Name, err)
		}
		return destino, nil
	}
	return "", fmt.Errorf("o zip da actualización non contén %q — revisa como o empaquetaches", buscado)
}

func lerHashEsperadoYang(r io.Reader) (string, error) {
	datos, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return "", fmt.Errorf("non se puido ler o .sha256: %w", err)
	}
	campos := strings.Fields(string(datos))
	if len(campos) == 0 {
		return "", fmt.Errorf("o ficheiro .sha256 da Release está baleiro")
	}
	return strings.ToLower(campos[0]), nil
}

// comprobarChecksumYang descarga urlChecksum e verifica que coincide co
// SHA-256 real de ficheiroRuta. Devolve erro (e non deixa continuar) se
// falta, non se pode descargar, ou non coincide — mellor cancelar a
// actualización ca instalar algo sen verificar.
func comprobarChecksumYang(ficheiroRuta, urlChecksum string) error {
	if urlChecksum == "" {
		return fmt.Errorf("esta Release non publica un .sha256 do .zip — non se pode verificar a súa integridade, así que se cancela a actualización por seguridade")
	}

	cliente := http.Client{Timeout: timeoutComprobarYang}
	resp, err := cliente.Get(urlChecksum)
	if err != nil {
		return fmt.Errorf("non se puido descargar o .sha256 da actualización: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("non se puido descargar o .sha256 da actualización: HTTP %d", resp.StatusCode)
	}
	esperado, err := lerHashEsperadoYang(resp.Body)
	if err != nil {
		return err
	}

	f, err := os.Open(ficheiroRuta)
	if err != nil {
		return fmt.Errorf("non se puido abrir %q para verificalo: %w", ficheiroRuta, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("non se puido calcular o SHA-256 de %q: %w", ficheiroRuta, err)
	}
	obtido := hex.EncodeToString(h.Sum(nil))

	if obtido != esperado {
		return fmt.Errorf("o SHA-256 do .zip descargado non coincide co publicado na Release — podería estar adulterado; cancélase a actualización")
	}
	return nil
}

func copiarFicheiroYang(orixe, destino string) error {
	in, err := os.Open(orixe)
	if err != nil {
		return fmt.Errorf("non se puido abrir %q: %w", orixe, err)
	}
	defer in.Close()
	out, err := os.OpenFile(destino, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("non se puido crear %q: %w", destino, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("non se puido copiar a %q: %w", destino, err)
	}
	return nil
}

// actualizarVersionModuloJSON reescribe (best-effort) o campo "version" do
// modulo.json que vive canda o executable, para que ⚙ Aula → Módulos de
// Piztu reflicta a nova versión sen agardar ao próximo escaneo do
// catálogo. Non é fatal se falla (modulo.json podería non estar presente en
// execucións soltas de Yang fóra do sistema de módulos): AplicarEnCaliente
// xa fixo o importante (substituír o executable) antes de chamar aquí.
func actualizarVersionModuloJSON(dirModulo, versionNova string) {
	ruta := filepath.Join(dirModulo, "modulo.json")
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return
	}
	var m map[string]any
	if err := json.Unmarshal(datos, &m); err != nil {
		return
	}
	m["version"] = versionNova
	novosDatos, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(ruta, novosDatos, 0o644)
}

// AplicarActualizacionYangEnCaliente descarga o .zip da Release en `urlZip`,
// verifica o seu SHA-256 contra o publicado en `urlChecksum`, extrae dentro
// del o executable de Yang e substitúeo polo que está en execución agora
// mesmo — sen pechar Yang nin descomprimir nada á man (← botón "Actualizar"
// no diálogo de información). Non reinicia por si mesma: quen chama decide
// cando (ver ReiniciarYang).
//
// Se falta o checksum, non se pode descargar, ou non coincide, non se toca
// nada — mellor cancelar a actualización ca instalar un binario sen
// verificar (ver comprobarChecksumYang).
//
// A substitución escribe primeiro o binario novo canda o vello (mesmo
// cartafol → mesmo sistema de ficheiros) e só ao final fai un os.Rename
// atómico enriba del: se algo falla a metade, o binario orixinal non se
// toca. Antes gárdase tamén unha copia coma "<executable>.anterior".
func AplicarActualizacionYangEnCaliente(urlZip, urlChecksum, versionNova string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("a actualización automática aínda non está soportada en Windows")
	}

	exeActual, err := os.Executable()
	if err != nil {
		return fmt.Errorf("non se puido localizar o executable actual: %w", err)
	}
	exeActual, err = filepath.EvalSymlinks(exeActual)
	if err != nil {
		return fmt.Errorf("non se puido resolver o executable actual: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "yang-actualizacion-")
	if err != nil {
		return fmt.Errorf("non se puido crear un cartafol temporal: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	zipRuta, err := descargarYang(urlZip, tmpDir, "actualizacion.zip")
	if err != nil {
		return err
	}
	if err := comprobarChecksumYang(zipRuta, urlChecksum); err != nil {
		return err
	}
	novoExe, err := extraerExecutableYang(zipRuta, tmpDir)
	if err != nil {
		return err
	}

	swapTmp := exeActual + ".novo"
	defer os.Remove(swapTmp) // non-op se xa se renomeou
	if err := copiarFicheiroYang(novoExe, swapTmp); err != nil {
		return err
	}

	_ = copiarFicheiroYang(exeActual, exeActual+".anterior") // copia de reserva; non crítico se falla

	if err := os.Rename(swapTmp, exeActual); err != nil {
		return fmt.Errorf("non se puido instalar o binario novo: %w", err)
	}

	// En macOS substituír o executable rompe a sinatura do .app enteiro e a
	// app deixaría de abrir - hai que volver asinala (no-op noutras
	// plataformas, ver bundle_darwin.go). Se non se pode, desfacer: mellor
	// quedar coa versión anterior funcionando ca cunha instalación que o
	// sistema xa non deixa arrincar.
	if err := resinarBundleMacOS(exeActual); err != nil {
		if errRestaurar := copiarFicheiroYang(exeActual+".anterior", exeActual); errRestaurar != nil {
			return fmt.Errorf("%w (e ademais non se puido restaurar a versión anterior: %v)", err, errRestaurar)
		}
		_ = resinarBundleMacOS(exeActual) // best-effort: devolver o bundle ao estado de antes
		return fmt.Errorf("actualización desfeita, segues na versión anterior: %w", err)
	}

	if versionNova != "" {
		actualizarVersionModuloJSON(filepath.Dir(exeActual), versionNova)
	}
	return nil
}

// ReiniciarYang arrinca unha nova copia de Yang (xa a versión instalada por
// AplicarActualizacionYangEnCaliente) coma proceso independente, para que
// quen chame poida pechar esta xanela decontado.
func ReiniciarYang() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("non se puido localizar o executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("non se puido resolver o executable: %w", err)
	}

	// En macOS hai que relanzar o .app, non o binario de dentro (ver
	// bundle_darwin.go); noutras plataformas isto é exec.Command(exe).
	cmd := comandoRelanzar(exe)
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("non se puido reiniciar Yang: %w", err)
	}
	return nil
}
