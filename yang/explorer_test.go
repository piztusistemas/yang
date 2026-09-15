package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListarCartafolOrdenaCartafolesPrimeiroLogoAlfabeticoSenDistinguirMaiusculas(t *testing.T) {
	dir := t.TempDir()
	for _, nome := range []string{"b.matex", "A.matex", ".gitignore", "zeta", "Alfa"} {
		full := filepath.Join(dir, nome)
		if nome == "zeta" || nome == "Alfa" {
			if err := os.Mkdir(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := &App{}
	entradas, err := a.ListarCartafol(dir)
	if err != nil {
		t.Fatal(err)
	}
	var nomes []string
	for _, e := range entradas {
		nomes = append(nomes, e.Name)
	}
	want := []string{"Alfa", "zeta", ".gitignore", "A.matex", "b.matex"}
	if len(nomes) != len(want) {
		t.Fatalf("got %v, want %v", nomes, want)
	}
	for i := range want {
		if nomes[i] != want[i] {
			t.Fatalf("got %v, want %v", nomes, want)
		}
	}
	// Cartafoles marcados IsDir, ficheiros non.
	for _, e := range entradas {
		wantDir := e.Name == "Alfa" || e.Name == "zeta"
		if e.IsDir != wantDir {
			t.Errorf("%s: IsDir=%v, want %v", e.Name, e.IsDir, wantDir)
		}
		if e.Path != filepath.Join(dir, e.Name) {
			t.Errorf("%s: Path=%q, want %q", e.Name, e.Path, filepath.Join(dir, e.Name))
		}
	}
}

func TestNovoCartafolRenomearEliminar(t *testing.T) {
	dir := t.TempDir()
	a := &App{}

	sub, err := a.NovoCartafol(dir, "exames")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(sub); err != nil || !info.IsDir() {
		t.Fatalf("NovoCartafol non creou o directorio: %v", err)
	}

	ficheiro := filepath.Join(sub, "orixinal.matex")
	if err := os.WriteFile(ficheiro, []byte("<EX></EX>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	novaRuta, err := a.RenomearFicheiro(ficheiro, "renomeado.matex")
	if err != nil {
		t.Fatal(err)
	}
	if novaRuta != filepath.Join(sub, "renomeado.matex") {
		t.Fatalf("got %q", novaRuta)
	}
	if _, err := os.Stat(novaRuta); err != nil {
		t.Fatalf("o ficheiro renomeado non existe: %v", err)
	}
	if _, err := os.Stat(ficheiro); !os.IsNotExist(err) {
		t.Fatalf("o ficheiro orixinal aínda existe tralo renomear")
	}

	if err := a.EliminarFicheiro(sub); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Fatalf("o cartafol aínda existe tralo eliminar")
	}
}

func TestRenomearFicheiroRexeitaNomesConBarra(t *testing.T) {
	dir := t.TempDir()
	ficheiro := filepath.Join(dir, "a.matex")
	if err := os.WriteFile(ficheiro, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	for _, malo := range []string{"../fora.matex", "sub/nome.matex", "", ".", ".."} {
		if _, err := a.RenomearFicheiro(ficheiro, malo); err == nil {
			t.Errorf("RenomearFicheiro(%q) debería fallar", malo)
		}
	}
	// O ficheiro orixinal segue intacto tras os intentos rexeitados.
	if _, err := os.Stat(ficheiro); err != nil {
		t.Fatalf("o ficheiro orixinal desapareceu: %v", err)
	}
}
