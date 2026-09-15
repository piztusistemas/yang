package main

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// jsonia.go: tipos "tolerantes" para ler o JSON que devolve a IA.
//
// Problema real que resolven: o prompt pide unha cadea ("min": "6") pero os
// modelos escriben moitas veces o valor natural ("min": 6), ou parten un
// campo longo nunha lista ("resolucion": ["paso 1", "paso 2"]). Con campos
// `string` puros, encoding/json aborta O EXAME ENTEIRO cun
//
//	json: cannot unmarshal number into Go struct field VariableTaboa.variables.min of type string
//
// e pérdese todo o traballo (e o tempo) da chamada. Como o dato que manda a
// IA é perfectamente utilizable - só chega noutra forma -, o parser adáptase
// a el en vez de rexeitalo: só se dá erro cando o JSON está mal formado de
// verdade.

// textoIA é un `string` que acepta calquera escalar JSON (cadea, número,
// booleano, null) e tamén unha lista deles (únea con saltos de liña, que é o
// que fai a IA cando parte unha resolución en pasos). Un obxecto consérvase
// coma JSON compacto: non se perde información e o profesorado pode editalo,
// que é mellor ca tirar coa xeración enteira.
type textoIA string

func (t *textoIA) UnmarshalJSON(data []byte) error {
	s, err := textoDeJSON(data)
	if err != nil {
		return err
	}
	*t = textoIA(s)
	return nil
}

// String/Trim: azucre para os usos habituais, que sempre queren o string.
func (t textoIA) String() string { return string(t) }
func (t textoIA) Trim() string   { return strings.TrimSpace(string(t)) }

func textoDeJSON(data []byte) (string, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return "", nil
	}
	switch data[0] {
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return "", err
		}
		return s, nil
	case '[':
		var partes []json.RawMessage
		if err := json.Unmarshal(data, &partes); err != nil {
			return "", err
		}
		var out []string
		for _, p := range partes {
			s, err := textoDeJSON(p)
			if err != nil {
				return "", err
			}
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, "\n"), nil
	case '{':
		var buf bytes.Buffer
		if err := json.Compact(&buf, data); err != nil {
			return "", err
		}
		return buf.String(), nil
	default:
		// número (6, 6.5, 1e3) ou true/false: o literal xa É o texto.
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return "", err
		}
		return string(data), nil
	}
}

// repararEscapesJSON arranxa o erro máis habitual da IA ao devolver JSON
// que leva código LaTeX/TikZ dentro dun valor de texto: barras invertidas
// SEN escapar ("\draw", "\node", "\,", "\Omega") e saltos de liña crus
// dentro do string. encoding/json rexeita as dúas cousas ("invalid
// character 'd' in string escape code", "invalid character '\n' in string
// literal") e iso tumbaba a xeración enteira do exame (report real cun
// <TIKZ> nun exercicio de circuítos). Chámase SÓ cando o parse normal xa
// fallou (ver chamarIAJSON), así que nunca "arranxa" un JSON que xa era bo.
//
// Percorre o texto mantendo o estado dentro/fóra de string. Fóra de string
// non se toca nada. Dentro:
//   - "\" seguido dun escape JSON válido (" \ / b f n r t u) déixase igual;
//   - calquera outro "\" pásase a "\\";
//   - un salto de liña / retorno de carro / tabulador CRU pásase a "\n" /
//     "\r" / "\t".
func repararEscapesJSON(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	enCadea := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !enCadea {
			b.WriteByte(c)
			if c == '"' {
				enCadea = true
			}
			continue
		}
		switch c {
		case '"':
			b.WriteByte(c)
			enCadea = false
		case '\\':
			if i+1 < len(s) && esEscapeJSONValido(s[i+1]) {
				b.WriteByte(c)
				b.WriteByte(s[i+1])
				i++
			} else {
				b.WriteString(`\\`)
			}
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func esEscapeJSONValido(c byte) bool {
	switch c {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	}
	return false
}

// variablesIA acepta a lista de variables tal e como a manda a IA: o array
// esperado, un só obxecto sen array arredor, un mapa {"a": {...}, "k": {...}}
// (nese caso a clave é o nome, e ordénase alfabeticamente para que a saída
// sexa sempre a mesma) ou null.
type variablesIA []VariableTaboa

func (v *variablesIA) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*v = nil
		return nil
	}
	if data[0] == '{' {
		// Unha variable soa, sen o array arredor.
		var unha VariableTaboa
		if err := json.Unmarshal(data, &unha); err == nil && unha.Nome.Trim() != "" {
			*v = variablesIA{unha}
			return nil
		}
		// Mapa nome -> variable.
		var mapa map[string]VariableTaboa
		if err := json.Unmarshal(data, &mapa); err != nil {
			return err
		}
		nomes := make([]string, 0, len(mapa))
		for n := range mapa {
			nomes = append(nomes, n)
		}
		sort.Strings(nomes)
		out := make(variablesIA, 0, len(nomes))
		for _, n := range nomes {
			va := mapa[n]
			if va.Nome.Trim() == "" {
				va.Nome = textoIA(n)
			}
			out = append(out, va)
		}
		*v = out
		return nil
	}
	var arr []VariableTaboa
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	*v = arr
	return nil
}

// listaTextosIA é o mesmo criterio para un array de cadeas soltas (o guión do
// exame, ver IniciarExameTaboa): tolera números, obxectos e mesmo unha cadea
// soa sen array.
type listaTextosIA []string

func (l *listaTextosIA) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*l = nil
		return nil
	}
	if data[0] != '[' {
		s, err := textoDeJSON(data)
		if err != nil {
			return err
		}
		if s = strings.TrimSpace(s); s != "" {
			*l = listaTextosIA{s}
		} else {
			*l = nil
		}
		return nil
	}
	var partes []json.RawMessage
	if err := json.Unmarshal(data, &partes); err != nil {
		return err
	}
	out := make(listaTextosIA, 0, len(partes))
	for _, p := range partes {
		s, err := textoDeJSON(p)
		if err != nil {
			return err
		}
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	*l = out
	return nil
}

// booleanoIA é un `bool` que acepta tamén "true"/"false"/"si"/"non" coma
// cadea e 0/1 coma número - outra forma que a IA devolve a miúdo aínda que
// se lle pida un booleano.
type booleanoIA bool

func (b *booleanoIA) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*b = false
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true", "si", "sí", "yes", "1":
			*b = true
		default:
			*b = false
		}
		return nil
	}
	var v bool
	if err := json.Unmarshal(data, &v); err == nil {
		*b = booleanoIA(v)
		return nil
	}
	var n float64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*b = n != 0
	return nil
}
