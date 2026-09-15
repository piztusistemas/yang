package cas

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestSessionTimeoutRecoversFromWedgedMaxima is the regression test for the
// "Yang hangs forever if Maxima never answers" case (infinite loop in a
// <MAT>/<EVAL>, or a genuinely dead process): SetTimeout bounds a single
// rawSend, and once it fires the session must (a) return promptly with a
// clear error instead of blocking, and (b) actually be dead afterwards -
// same as if the user had pressed "Reiniciar Maxima" manually - so the next
// call fails fast instead of hanging again.
func TestSessionTimeoutRecoversFromWedgedMaxima(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	sess.SetTimeout(2 * time.Second)

	start := time.Now()
	// Bucle infinito: mantén Maxima ocupado en CPU sen producir NINGUNHA
	// saída, exactamente o escenario que readUntil non pode detectar por si
	// só (non hai deadline no ReadByte subxacente).
	_, err = sess.rawSend("block([n:0], while true do n:n+1);")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("esperaba un erro de timeout, a chamada devolveu éxito")
	}
	if !strings.Contains(err.Error(), "non respondeu") {
		t.Errorf("mensaxe de erro pouco clara: %v", err)
	}
	if elapsed > 4*time.Second {
		t.Errorf("tardou %s en recuperarse, esperaba ~2s (timeout) + marxe", elapsed)
	}

	// A sesión ten que quedar realmente morta despois (Kill() automático) -
	// unha chamada posterior falla rápido, non queda colgada outra vez.
	start2 := time.Now()
	_, err2 := sess.rawSend("1+1;")
	if err2 == nil {
		t.Error("esperaba erro nunha sesión xa matada polo timeout")
	}
	if time.Since(start2) > time.Second {
		t.Error("a chamada posterior á sesión matada debería fallar ao instante, non bloquear")
	}
}

// TestSessionTimeoutDoesNotInterfereWithNormalCalls confirms a generous
// timeout doesn't false-positive on an ordinary (fast) calculation.
func TestSessionTimeoutDoesNotInterfereWithNormalCalls(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	sess.SetTimeout(30 * time.Second)

	out, err := sess.rawSend("1+1;")
	if err != nil {
		t.Fatalf("chamada normal fallou cun timeout xeneroso: %v", err)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("esperaba ver \"2\" na resposta, got: %q", out)
	}
}
