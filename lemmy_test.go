package main

import (
	"net/http"
	"strings"
	"testing"
)

var words = map[string]string{
	"et":        "et",
	"oratio":    "oratio",
	"conviciis": "convicium",
}

func TestLemmatizeWord(t *testing.T) {
	httpClient := &http.Client{}
	for base, lemmyd := range words {
		lemTest, _ := LemmatizeWord(httpClient, base)
		t.Logf("%s:%s:%s", base, lemTest, lemmyd)
		if lemTest != lemmyd {
			t.Error("Lemmatize Failed for word " + base)
		}
	}
}

func TestLemmatizeText(t *testing.T) {
	var in string
	var out []string
	mx := 10
	MAX_REQUESTS = &mx
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
			t.Errorf("Lemmatize Failed for word index %d", i)
		}
		i++
	}

	if i != len(out) {
		t.Error("Input and output counts do not match")
	}
}
