package main

import (
	"runtime"
	"strings"
	"testing"
)

// TestAnotarPaqueteQueFalta: da chea de liñas dun log de LaTeX hai que
// sacar o ficheiro que faltou, e SÓ cando é iso - un erro de sintaxe non
// pode deixar un botón de "instalar" pendurado.
func TestAnotarPaqueteQueFalta(t *testing.T) {
	defer limparPaqueteQueFalta()
	a := &App{}

	anotarPaqueteQueFalta("! LaTeX Error: File `tcolorbox.sty' not found.\nl.7 \\usepackage")
	info := a.PaqueteLatexQueFalta()
	if info.Ficheiro != "tcolorbox.sty" || info.Nome != "tcolorbox" {
		t.Fatalf("non se recoñeceu o paquete que falta: %+v", info)
	}

	// Unha clase de documento (.cls) conta igual.
	anotarPaqueteQueFalta("! LaTeX Error: File `beamer.cls' not found.")
	if info := a.PaqueteLatexQueFalta(); info.Nome != "beamer" {
		t.Errorf("esperaba beamer, obtiven %+v", info)
	}

	// Calquera outro erro non deixa nada pendente.
	anotarPaqueteQueFalta("! Undefined control sequence.\nl.4 \\foo")
	if info := a.PaqueteLatexQueFalta(); info.Ficheiro != "" {
		t.Errorf("un erro que non é de paquete non pode deixar nada pendente: %+v", info)
	}

	// E unha compilación correcta límpao.
	anotarPaqueteQueFalta("! LaTeX Error: File `tcolorbox.sty' not found.")
	limparPaqueteQueFalta()
	if info := a.PaqueteLatexQueFalta(); info.Ficheiro != "" {
		t.Errorf("tras limpar non pode quedar nada: %+v", info)
	}
}

// TestAnotarPaqueteQueFalta_Universal: o aviso "falta algo de LaTeX" ten que
// saltar tamén cando o que falta NON é un .sty/.cls - idiomas de babel
// (.ldf) e fontes (fontspec) - e aínda que a mensaxe do log veña cortada
// polo ancho de liña ("not foun\nd").
func TestAnotarPaqueteQueFalta_Universal(t *testing.T) {
	defer limparPaqueteQueFalta()
	a := &App{}

	// Caso real do usuario: lualatex + babel galego, que arrastra spanish.ldf.
	// O log parte "was not found" en dúas liñas.
	logBabel := "! Package babel Error: Unknown option 'spanish'. Either you misspelled it\n" +
		"(babel)                or the language definition file spanish.ldf was not foun\nd.\n" +
		"l.4357 \\ProcessOptions*"
	anotarPaqueteQueFalta(logBabel)
	info := a.PaqueteLatexQueFalta()
	if info.Ficheiro != "spanish.ldf" || info.Nome != "spanish" {
		t.Fatalf("babel spanish.ldf non recoñecido: %+v", info)
	}

	// Forma alternativa: "File `french.ldf' not found".
	anotarPaqueteQueFalta("! LaTeX Error: File `french.ldf' not found.")
	if info := a.PaqueteLatexQueFalta(); info.Nome != "french" {
		t.Errorf("esperaba french, obtiven %+v", info)
	}

	// Fonte que fontspec non atopa (plantilla con \setmainfont).
	anotarPaqueteQueFalta(`! Package fontspec Error: The font "Fira Sans" cannot be found.`)
	if info := a.PaqueteLatexQueFalta(); info.Ficheiro != "Fira Sans.font" || info.Nome != "Fira Sans" {
		t.Errorf("fonte non recoñecida: %+v", info)
	}

	// Un .def/.fd (ficheiro de soporte dun paquete) tamén conta.
	anotarPaqueteQueFalta("! LaTeX Error: File `t1qhv.fd' not found.")
	if info := a.PaqueteLatexQueFalta(); info.Nome != "t1qhv" {
		t.Errorf("esperaba t1qhv, obtiven %+v", info)
	}
}

// TestPaqueteDebianPara: a tradución "ficheiro que falta -> paquete apt"
// distingue por extensión.
func TestPaqueteDebianPara(t *testing.T) {
	casos := []struct{ ext, nome, want string }{
		{".ldf", "spanish", "texlive-lang-spanish"},
		{".ldf", "galician", "texlive-lang-european"},
		{".ldf", "portuguese", "texlive-lang-portuguese"},
		{".font", "Fira Sans", "texlive-fonts-extra"},
		{".sty", "pgfplots", "texlive-pictures"},
		{".sty", "un-paquete-raro", "texlive-latex-extra"},
	}
	for _, c := range casos {
		if got := paqueteDebianPara(c.ext, c.nome); got != c.want {
			t.Errorf("paqueteDebianPara(%q,%q) = %q, want %q", c.ext, c.nome, got, c.want)
		}
	}
}

// TestComandoInstalarPaqueteLatex comproba a forma do comando no sistema
// onde corre a proba (non se pode simular outro: depende de que binarios
// existan de verdade). O que se garante en TODOS os casos: ou hai comando,
// ou hai un motivo lexible - nunca as dúas cousas baleiras, que deixaría a
// UI sen nada que dicir.
func TestComandoInstalarPaqueteLatex(t *testing.T) {
	argv, descricion, motivo := comandoInstalarPaqueteLatex("tcolorbox.sty", "tcolorbox")
	if argv == nil {
		if strings.TrimSpace(motivo) == "" {
			t.Fatal("sen comando hai que dar un motivo que se poida amosar")
		}
		t.Logf("neste equipo non se pode instalar automaticamente: %s", motivo)
		return
	}
	if descricion == "" {
		t.Error("hai que amosar SEMPRE o comando antes de executalo")
	}
	// A descrición ten que parecer un comando de instalación real. NON se
	// esixe que conteña o nome de CTAN: en Debian «tcolorbox» resólvese á
	// colección texlive-latex-extra (Debian non reparte por paquete de CTAN).
	if !strings.Contains(descricion, "install") && !strings.Contains(descricion, "-S") {
		t.Errorf("a descrición non parece un comando de instalación: %q", descricion)
	}
	switch runtime.GOOS {
	case "darwin":
		if argv[0] != "sudo" || !strings.Contains(strings.Join(argv, " "), "tlmgr install tcolorbox") {
			t.Errorf("en macOS espérase sudo -A tlmgr install: %v", argv)
		}
	case "linux":
		if argv[0] != "pkexec" {
			t.Errorf("en Linux os permisos pídense con pkexec: %v", argv)
		}
	}
}

// TestPaqueteDebianPorColeccion: Debian non reparte TeX Live por paquete de
// CTAN, así que hai que traducir. O importante é que un nome descoñecido
// NON invente un paquete inexistente (faría fallar o apt-get enteiro) senón
// que caia na colección grande.
func TestPaqueteDebianPorColeccion(t *testing.T) {
	if paquetesDebian["pgfplots"] != "texlive-pictures" {
		t.Error("pgfplots vive en texlive-pictures")
	}
	if paquetesDebian["siunitx"] != "texlive-science" {
		t.Error("siunitx vive en texlive-science")
	}
	if _, hai := paquetesDebian["un-paquete-que-non-existe"]; hai {
		t.Error("a táboa non debería ter entradas inventadas")
	}
	if paqueteDebianPorDefecto != "texlive-latex-extra" {
		t.Errorf("o fallback ten que ser a colección grande, é %q", paqueteDebianPorDefecto)
	}
}
