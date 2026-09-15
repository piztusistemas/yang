package main

import "testing"

// TestBibliotecaRoundTrip cobre o ciclo completo (gardar -> listar ->
// eliminar) contra unha configuración illada (configIllado, plantillas_test.go:
// XDG_CONFIG_HOME non abonda, os.UserConfigDir ignórao en macOS e a proba
// acababa escribindo na biblioteca REAL de quen a executa).
func TestBibliotecaRoundTrip(t *testing.T) {
	configIllado(t)
	a := &App{}

	if _, err := a.GardarNaBiblioteca("", "exame", "algo"); err == nil {
		t.Error("esperaba erro con nome baleiro")
	}
	if _, err := a.GardarNaBiblioteca("Cabeceira IES", "tipo-lixo", "algo"); err == nil {
		t.Error("esperaba erro con tipo descoñecido")
	}
	if _, err := a.GardarNaBiblioteca("Cabeceira IES", "bloque", ""); err == nil {
		t.Error("esperaba erro con contido baleiro")
	}

	item, err := a.GardarNaBiblioteca("Cabeceira IES", "bloque", "<p>Nome: ___</p>")
	if err != nil {
		t.Fatalf("gardar bloque: %v", err)
	}
	if item.ID == "" {
		t.Error("esperaba unha ID non baleira")
	}
	if _, err := a.GardarNaBiblioteca("Exame trimestral 1", "exame", "<EX>...</EX>\n"); err != nil {
		t.Fatalf("gardar exame: %v", err)
	}

	items, err := a.ListarBiblioteca()
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("esperaba 2 elementos, atopei %d", len(items))
	}

	if err := a.EliminarDaBiblioteca(item.ID); err != nil {
		t.Fatalf("eliminar: %v", err)
	}
	items, err = a.ListarBiblioteca()
	if err != nil {
		t.Fatalf("listar tras eliminar: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("esperaba 1 elemento tras eliminar, atopei %d", len(items))
	}
	if items[0].Nome != "Exame trimestral 1" {
		t.Errorf("quedou o elemento incorrecto: %+v", items[0])
	}

	// Eliminar unha ID xa inexistente non debe fallar (no-op silencioso).
	if err := a.EliminarDaBiblioteca(item.ID); err != nil {
		t.Errorf("eliminar ID xa borrada non debería fallar: %v", err)
	}
}

// TestBibliotecaPersistsAcrossReload comproba que un "reinicio" (chamada
// nova a ListarBiblioteca, sen estado en memoria) segue vendo o gardado
// antes - o mesmo bug de fondo có das credenciais de Gemini, pero para a
// biblioteca.
func TestBibliotecaPersistsAcrossReload(t *testing.T) {
	configIllado(t)
	a := &App{}
	if _, err := a.GardarNaBiblioteca("Cabeceira IES", "bloque", "<p>Nome: ___</p>"); err != nil {
		t.Fatalf("gardar: %v", err)
	}

	// Reinicio simulado: unha App nova, mesma configuración illada.
	b := &App{}
	items, err := b.ListarBiblioteca()
	if err != nil {
		t.Fatalf("listar tras reinicio: %v", err)
	}
	if len(items) != 1 || items[0].Nome != "Cabeceira IES" {
		t.Fatalf("non persistiu tras reinicio: %+v", items)
	}
}
