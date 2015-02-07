package main

import (
	"strings"
	"testing"
)

var words = map[string]string{
	"et":        "et",
	"oratio":    "oratio",
	"conviciis": "convicium",
}

func TestLemmatizeWord(t *testing.T) {
	for base, lemmyd := range words {
		lemTest := LemmatizeWord(base)
		t.Logf("%s:%s:%s", base, lemTest, lemmyd)
		if lemTest != lemmyd {
			t.Error("Lemmatize Failed for word " + base)
		}
	}
}

func TestLemmatizeText(t *testing.T) {
	var in string
	var out []string
	for base, lemmyd := range words {
		in += base + " "
		out = append(out, lemmyd)
	}
	t.Log(in)
	lr := NewLemmaReader(strings.NewReader(in))

	i := 0
	for w, done := lr.Read(); !done; w, done = lr.Read() {
		t.Logf("%s:%s", w, out[i])
		if w != out[i] {
			t.Error("Lemmatize Failed for word index " + string(i))
		}
		i++
	}

	if i != len(out) {
		t.Error("Input and output counts do not match")
	}
}
