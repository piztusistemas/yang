// Package cas implements an interactive session with the Maxima CAS,
// porting the logic found in matexe.2013's casprocess.pas / maxima.pas.
package cas

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// sentinel is sent after every real command so we can detect where its
// output ends. Mirrors id_end_read := 'id_fin_de_lectura' in maxima.pas.
const sentinel = "id_fin_de_lectura"

// Session drives an interactive Maxima subprocess over stdin/stdout,
// equivalent to TCasProcess in casprocess.pas.
type Session struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu     sync.Mutex
	buf    strings.Builder // accumulates raw output between reads, like lastRead
	closed bool

	// timeout, when > 0, bounds how long rawSend waits for a single reply
	// before treating Maxima as wedged - see SetTimeout.
	timeout time.Duration
}

// SetTimeout bounds how long a single rawSend waits for Maxima to answer
// before giving up on it as wedged (infinite loop, or a genuinely dead
// process): rawSend kills the session automatically and returns an error
// instead of blocking forever. d <= 0 disables the bound (the original
// wait-forever behaviour). Doesn't affect a read already in flight.
func (s *Session) SetTimeout(d time.Duration) {
	s.mu.Lock()
	s.timeout = d
	s.mu.Unlock()
}

// Open starts the Maxima binary (path e.g. "/usr/bin/maxima") and waits for
// it to be ready to accept input.
func Open(maximaPath string) (*Session, error) {
	cmd := exec.Command(maximaPath, "-q")
	// ver tempenv_windows.go: en Windows, substitúe TEMP/TMP por unha ruta
	// garantidamente ASCII antes de arrincar Maxima - se non, Maxima falla
	// en escribir o seu propio script .gnuplot de traballo cando o usuario
	// de Windows ten un "ñ"/"á"/... no seu nome (moi común en Galicia). En
	// calquera outra plataforma, maximaEnv() devolve os.Environ() sen tocar.
	cmd.Env = maximaEnv()
	// ver execwindow_windows.go: sen isto, cada Xerar abriría unha xanela de
	// consola negra visible (Maxima é un proceso de consola en Windows,
	// e Yang non ten consola propia da que herdar unha).
	ocultarConsola(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout // poStderrToOutPut equivalent

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	s := &Session{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
	}

	// Drain the initial banner up to (and including) the first "(%iN)"
	// prompt, so it doesn't bleed into the first real command's output.
	if _, err := s.readUntil("(%i"); err != nil {
		return nil, fmt.Errorf("waiting for maxima startup prompt: %w", err)
	}
	return s, nil
}

// rawSend writes text to Maxima, appends the sentinel command, and reads
// back everything up to (and including) the following "(%iM)" prompt. This
// mirrors TCasProcess.send()/read().
//
// When a timeout is set (SetTimeout), the read runs in its own goroutine so
// a Maxima that never answers (infinite loop, or a wedged/dead process -
// readUntil's ReadByte has no deadline of its own) doesn't block the caller
// forever: past the deadline, rawSend kills the session itself - same
// mechanism the manual "Reiniciar Maxima" button already used via Kill() -
// which unblocks the goroutine's pending read (closed pipe) and returns a
// clear error instead. The goroutine result is discarded via the buffered
// channel so it doesn't leak waiting for a receiver.
func (s *Session) rawSend(text string) (string, error) {
	if s.closed {
		return "", fmt.Errorf("session is closed")
	}
	if _, err := io.WriteString(s.stdin, text+"\n"); err != nil {
		return "", err
	}
	if _, err := io.WriteString(s.stdin, sentinel+";\n"); err != nil {
		return "", err
	}

	s.mu.Lock()
	timeout := s.timeout
	s.mu.Unlock()
	if timeout <= 0 {
		return s.readUntil(sentinel)
	}

	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := s.readUntil(sentinel)
		ch <- result{out, err}
	}()
	select {
	case r := <-ch:
		return r.out, r.err
	case <-time.After(timeout):
		s.Kill()
		return "", fmt.Errorf("Maxima non respondeu en %s - a sesión reiniciouse automaticamente, podes tentalo de novo", timeout)
	}
}

// readUntil accumulates bytes from stdout until the running buffer contains
// marker, then keeps reading through the closing ')' of whatever prompt
// follows (e.g. "(%i9)") so that trailing fragment isn't left sitting in
// the stream to corrupt the start of the *next* read.
func (s *Session) readUntil(marker string) (string, error) {
	var out strings.Builder
	for {
		b, err := s.stdout.ReadByte()
		if err != nil {
			return out.String(), err
		}
		out.WriteByte(b)
		if strings.Contains(out.String(), marker) {
			break
		}
	}
	for {
		b, err := s.stdout.ReadByte()
		if err != nil {
			return out.String(), err
		}
		out.WriteByte(b)
		if b == ')' {
			return out.String(), nil
		}
	}
}

// Close terminates the Maxima session, mirroring TCasProcess.close().
func (s *Session) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	io.WriteString(s.stdin, "quit();\n")
	s.stdin.Close()
	return s.cmd.Wait()
}

// Kill forcibly terminates the underlying Maxima process, for recovering a
// session that's wedged inside rawSend's blocking ReadByte loop (readUntil
// has no timeout, so a Maxima stuck on a runaway computation - or just
// dead-locked - hangs that read forever). Close() can't be used for this: it
// writes "quit();\n" and waits for the process to exit on its own, which
// blocks exactly as long as the hang it's meant to recover from. Marking
// s.closed first means a later Close() (deferred by the caller) becomes a
// no-op instead of trying the same graceful shutdown again.
func (s *Session) Kill() error {
	s.closed = true
	if s.cmd.Process == nil {
		return nil
	}
	return s.cmd.Process.Kill()
}
