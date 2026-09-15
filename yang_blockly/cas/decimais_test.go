package cas

import (
	"os/exec"
	"strings"
	"testing"
)

// TestSetDecimaisRoundsFloatingResults is the regression test for "I_T =
// 3.555555555555556 = 32" showing up in a generated exam: any calculation
// that isn't an exact fraction propagates full machine precision into the
// TeX output unless SetDecimais rounds it first. 32/9 mirrors the report's
// own numbers (24V / 6.75Ω).
func TestSetDecimaisRoundsFloatingResults(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	m := NewMaxima(sess)
	defer m.Close()
	m.Format = FormatLatex

	if _, err := m.Send("linel:1024;display2d:false;simp:true;", false); err != nil {
		t.Fatal(err)
	}

	// Sen redondeo (Decimais=0, comportamento previo): confirma que o
	// problema reproduce sen o arranxo.
	raw, err := m.ToTeX("32.0/9", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "3.555555555555556") {
		t.Fatalf("esperaba ver a precisión completa sen SetDecimais, got: %q", raw)
	}

	// Con SetDecimais(2): a mesma expresión debe saír redondeada.
	if err := m.SetDecimais(2); err != nil {
		t.Fatal(err)
	}
	rounded, err := m.ToTeX("32.0/9", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rounded, "3.56") {
		t.Errorf("esperaba 3.56 con 2 decimais, got: %q", rounded)
	}
	if strings.Contains(rounded, "3.555555555555556") {
		t.Errorf("SetDecimais(2) non recortou a precisión completa: %q", rounded)
	}

	// Caso real do informe: unha ecuación completa "I_T = <valor>" - o
	// float ten que redondearse aínda que estea dentro doutra expresión
	// (op "=" con dous argumentos), non só cando é o resultado solto.
	eq, err := m.ToTeX("I_T=32.0/9", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(eq, "3.56") || strings.Contains(eq, "3.555555555555556") {
		t.Errorf("esperaba I_T=3.56 dentro da ecuación, got: %q", eq)
	}

	// Exacto (fracción, non float) debe quedar intacto.
	exact, err := m.ToTeX("32/9", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exact, "32") || !strings.Contains(exact, "9") {
		t.Errorf("esperaba a fracción exacta 32/9 sen tocar, got: %q", exact)
	}
}

// TestSetDecimaisDoesNotContaminateChainedCalculations proba que o
// redondeo é só de PRESENTACIÓN (dentro de ToTeX) e nunca se garda de
// volta na sesión Maxima: unha variable definida vía Send (o camiño que
// usa <HIDE>) mantén toda a súa precisión para cálculos posteriores, aínda
// que xa se estea mostrando redondeada. Se matexe_arredondar contaminase o
// valor gardado, b saíría 10.68 (3.56*3) en vez do correcto 10.67
// (32/3 = 10.666..., redondeado só ao mostralo).
func TestSetDecimaisDoesNotContaminateChainedCalculations(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	m := NewMaxima(sess)
	defer m.Close()
	m.Format = FormatLatex

	if _, err := m.Send("linel:1024;display2d:false;simp:true;", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetDecimais(2); err != nil {
		t.Fatal(err)
	}

	// a = 32/9 = 3.555555555555556 (defínese coa precisión completa, coma
	// faría un <HIDE> real).
	if _, err := m.Send("a: 32.0/9", true); err != nil {
		t.Fatal(err)
	}
	// A presentación de "a" redondéase...
	aShown, err := m.ToTeX("a", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(aShown, "3.56") {
		t.Fatalf("esperaba a mostrarse coma 3.56, got: %q", aShown)
	}

	// ...pero "b: a*3" ten que partir do valor completo de a (3.555555...),
	// non do 3.56 xa amosado - 32/3 = 10.666666666666666, redondeado a
	// 10.67. Se b saíse 10.68, o redondeo estaría contaminando os cálculos.
	if _, err := m.Send("b: a*3", true); err != nil {
		t.Fatal(err)
	}
	bShown, err := m.ToTeX("b", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bShown, "10.67") {
		t.Errorf("esperaba 10.67 (32/3 redondeado ao mostrar), got: %q", bShown)
	}
	if strings.Contains(bShown, "10.68") {
		t.Errorf("10.68 indicaría que b se calculou a partir do 3.56 xa redondeado, non do valor completo: %q", bShown)
	}
}

// TestMatexeDecimalForcesFloat is the regression test for "P = 230*5 =
// 23/20 kW" showing up in a physics worksheet: matexe_arredondar
// deliberately leaves exact fractions alone (see TestSetDecimaisRounds-
// FloatingResults's "32/9 sen tocar" case, needed for exercises like
// testdata/test.matex's "Simplificación de fraccións"), so an all-integer
// calculation like 230*5/1000 stays an exact rational unless something
// forces it to float. matexe_decimal(e) is that explicit escape hatch, and
// it composes with the existing N-decimal rounding for free.
func TestMatexeDecimalForcesFloat(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	m := NewMaxima(sess)
	defer m.Close()
	m.Format = FormatLatex

	if _, err := m.Send("linel:1024;display2d:false;simp:true;", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetDecimais(2); err != nil {
		t.Fatal(err)
	}

	// Sen matexe_decimal: exactamente o caso do informe, 230*5/1000 queda en
	// fracción exacta (23/20), non en 1.15.
	exact, err := m.ToTeX("230*5/1000", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exact, "23") || !strings.Contains(exact, "20") {
		t.Errorf("esperaba a fracción exacta 23/20 sen matexe_decimal, got: %q", exact)
	}
	if strings.Contains(exact, "1.15") {
		t.Errorf("non esperaba 1.15 sen matexe_decimal, got: %q", exact)
	}

	// Con matexe_decimal: o mesmo cálculo ten que saír en decimal (1.15),
	// coa mesma regra de redondeo (Decimais=2) xa aplicada por riba.
	decimal, err := m.ToTeX("matexe_decimal(230*5/1000)", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(decimal, "1.15") {
		t.Errorf("esperaba 1.15 con matexe_decimal, got: %q", decimal)
	}
	if strings.Contains(decimal, "23") && strings.Contains(decimal, "20") {
		t.Errorf("esperaba decimal, non a fracción 23/20, got: %q", decimal)
	}

	// Regresión do informe real: "<MAT>4</MAT>" (un enteiro, non unha
	// fracción) NON debe converterse en "4.0" - matexe_decimal só toca
	// fraccións exactas non enteiras, un enteiro xa está "en decimal" tal
	// como é.
	integer, err := m.ToTeX("matexe_decimal(4)", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(integer, "4") || strings.Contains(integer, "4.0") {
		t.Errorf("esperaba 4 sen .0, got: %q", integer)
	}

	// Sobrevive a kill(all) coma matexe_arredondar (ver
	// TestSetDecimaisSurvivesKillAll): app.go/latexdoc.go/markdowndoc.go
	// vólveno definir despois de cada reset entre variantes.
	if _, err := m.Send("reset();kill(all)", true); err != nil {
		t.Fatal(err)
	}
	if err := m.SetDecimais(2); err != nil {
		t.Fatal(err)
	}
	afterKill, err := m.ToTeX("matexe_decimal(230*5/1000)", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(afterKill, "matexe_decimal") {
		t.Fatalf("matexe_decimal apareceu sen avaliar despois de kill(all): %q", afterKill)
	}
	if !strings.Contains(afterKill, "1.15") {
		t.Errorf("esperaba 1.15 despois de kill(all), got: %q", afterKill)
	}
}

// TestForzarDecimalDefaultsToFloatWithOverride is the regression test for
// the "Amosar resultados en decimal" document-wide toggle (Settings.Decimal,
// settings.go): with ForzarDecimal on, an all-integer calculation that
// would normally stay an exact fraction (see TestMatexeDecimalForcesFloat)
// is now floated BY DEFAULT, without the teacher having to call
// matexe_decimal(...) on every line - but a specific exercise that still
// wants the exact fraction (ex. "Simplificación de fraccións") can opt out
// by wrapping it in matexe_fraccion(...).
func TestForzarDecimalDefaultsToFloatWithOverride(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	m := NewMaxima(sess)
	defer m.Close()
	m.Format = FormatLatex
	m.ForzarDecimal = true

	if _, err := m.Send("linel:1024;display2d:false;simp:true;", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetDecimais(2); err != nil {
		t.Fatal(err)
	}

	// Por defecto, en decimal (1.15), sen chamar matexe_decimal a man.
	autoDecimal, err := m.ToTeX("230*5/1000", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(autoDecimal, "1.15") {
		t.Errorf("esperaba 1.15 con ForzarDecimal, got: %q", autoDecimal)
	}
	if strings.Contains(autoDecimal, "23") && strings.Contains(autoDecimal, "20") {
		t.Errorf("esperaba decimal, non a fracción 23/20, got: %q", autoDecimal)
	}

	// matexe_fraccion(...) escapa do auto-wrap: fracción exacta, aínda con
	// ForzarDecimal activo.
	exact, err := m.ToTeX("matexe_fraccion(230*5/1000)", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exact, "23") || !strings.Contains(exact, "20") {
		t.Errorf("esperaba a fracción exacta 23/20 con matexe_fraccion, got: %q", exact)
	}
	if strings.Contains(exact, "1.15") {
		t.Errorf("matexe_fraccion non evitou o auto-wrap a decimal: %q", exact)
	}

	// Mesmo caso que TestMatexeDecimalForcesFloat pero co auto-wrap: un
	// resultado enteiro segue sendo "4", non "4.0", aínda con ForzarDecimal
	// activo - é xustamente o caso que motivou este arranxo ("<MAT>4</MAT>"
	// nun boletín, non ten sentido velo coma "4.0").
	integer, err := m.ToTeX("4", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(integer, "4") || strings.Contains(integer, "4.0") {
		t.Errorf("esperaba 4 sen .0 aínda con ForzarDecimal, got: %q", integer)
	}
}

// TestSetDecimaisSurvivesKillAll is the regression test for
// "matexe_arredondar" showing up literally, unevaluated, in a generated
// exam (ex. "I = VR = matexe_arredondar(5,2)"): app.go/latexdoc.go/
// markdowndoc.go all run "reset();kill(all)" once per variant inside their
// generation loop, and kill(all) wipes EVERY user definition - including
// whatever SetDecimais just defined. If SetDecimais is called only once
// before the loop (instead of again after each kill(all)), every ToTeX
// call for every variant hits a matexe_arredondar that no longer exists,
// and Maxima echoes the call back unevaluated instead of erroring.
func TestSetDecimaisSurvivesKillAll(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	m := NewMaxima(sess)
	defer m.Close()
	m.Format = FormatLatex

	if _, err := m.Send("linel:1024;display2d:false;simp:true;", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetDecimais(2); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Send("reset();kill(all)", true); err != nil {
		t.Fatal(err)
	}
	// O arranxo real: redefinir despois de cada kill(all), non só unha vez
	// antes do bucle de variantes.
	if err := m.SetDecimais(2); err != nil {
		t.Fatal(err)
	}

	out, err := m.ToTeX("I = 24/6.75", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "arredondar") {
		t.Fatalf("matexe_arredondar apareceu sen avaliar despois de kill(all): %q", out)
	}
	if !strings.Contains(out, "3.56") {
		t.Errorf("esperaba I=3.56 (24/6.75 redondeado), got: %q", out)
	}
}
