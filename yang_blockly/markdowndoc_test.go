package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMarkdownConverterHR comproba que <hr> se traduce a "---" (regra
// horizontal estándar en Markdown) no canto de caer no ramo "default" de
// renderElement, que o marcaría coma etiqueta sen soporte e dispararía o
// aviso "etiqueta HTML sen soporte en Markdown" na UI.
func TestMarkdownConverterHR(t *testing.T) {
	nodes, err := parseFragment("<p>Antes</p><hr><p>Despois</p>")
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	conv := newMarkdownConverter("", t.TempDir())
	var out string
	for _, n := range nodes {
		out += conv.render(n)
	}
	if conv.unknownTags["hr"] {
		t.Error("<hr> non debería quedar marcado coma etiqueta sen soporte")
	}
	if !strings.Contains(out, "---") {
		t.Errorf("esperaba unha regra horizontal (---) na saída, obtiven: %q", out)
	}
}

// TestMarkdownConverterLinkExternal comproba que <A href="https://..."> se
// traduce á sintaxe [texto](url) de Markdown, mesmo criterio ca a versión
// LaTeX (ver TestLatexConverterRendersLinkExternal) - antes desta
// funcionalidade "a" caía no ramo que só conserva os fillos (agrupado con
// span/font), perdendo o href por completo.
func TestMarkdownConverterLinkExternal(t *testing.T) {
	nodes, err := parseFragment(`ver <A href="https://example.org/guia.pdf">a guía</A>`)
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	conv := newMarkdownConverter("", t.TempDir())
	var out string
	for _, n := range nodes {
		out += conv.render(n)
	}
	want := "[a guía](https://example.org/guia.pdf)"
	if !strings.Contains(out, want) {
		t.Errorf("esperaba %q, obtiven: %q", want, out)
	}
	if conv.unknownTags["a"] {
		t.Error("<a> non debería quedar marcado coma etiqueta sen soporte")
	}
}

// TestMarkdownConverterLinkLocal comproba unha ligazón LOCAL (un PDF subido
// dende o explorador, ver explorer.go SubirFicheiroCartafol): cópiase a
// buildDir/attachments (mesmo criterio ca renderImg con images/) e a
// ligazón apunta a ese camiño relativo.
func TestMarkdownConverterLinkLocal(t *testing.T) {
	baseDir := t.TempDir()
	buildDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "modelo.pdf"), []byte("%PDF-fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	nodes, err := parseFragment(`<A href="modelo.pdf">o modelo</A>`)
	if err != nil {
		t.Fatalf("parseFragment: %v", err)
	}
	conv := newMarkdownConverter(baseDir, buildDir)
	var out string
	for _, n := range nodes {
		out += conv.render(n)
	}
	want := "[o modelo](attachments/modelo.pdf)"
	if !strings.Contains(out, want) {
		t.Errorf("esperaba %q, obtiven: %q", want, out)
	}
	if _, err := os.Stat(filepath.Join(buildDir, "attachments", "modelo.pdf")); err != nil {
		t.Errorf("esperaba modelo.pdf copiado a buildDir/attachments: %v", err)
	}
}

// TestMarkdownConverterSmall: as mesmas etiquetas que agora renderiza o
// conversor LaTeX (ver TestLatexConverterRendersSmall) non poden quedar
// marcadas coma "sen soporte" tampouco aquí. Markdown non ten tamaño de
// letra, así que <small>/<big> só conservan o texto; <code> e
// <blockquote> si teñen sintaxe propia.
func TestMarkdownConverterSmall(t *testing.T) {
	casos := []struct{ html, agardado string }{
		{"<small>2 puntos</small>", "2 puntos"},
		{"<big>título</big>", "título"},
		{"<code>plot2d</code>", "`plot2d`"},
		{"<blockquote>citado</blockquote>", "> citado"},
	}
	for _, c := range casos {
		nodes, err := parseFragment(c.html)
		if err != nil {
			t.Fatalf("parseFragment: %v", err)
		}
		conv := newMarkdownConverter("", t.TempDir())
		var out string
		for _, n := range nodes {
			out += conv.render(n)
		}
		if !strings.Contains(out, c.agardado) {
			t.Errorf("%s -> %q, esperaba que contivese %q", c.html, out, c.agardado)
		}
		if len(conv.unknownTags) > 0 {
			t.Errorf("%s marcou etiquetas sen soporte: %v", c.html, conv.unknownTags)
		}
	}
}
