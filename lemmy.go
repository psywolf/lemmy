package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"text/scanner"
	"time"

	"github.com/alecthomas/kingpin"
)

var (
	urlBase    = "http://www.perseus.tufts.edu/hopper/xmlmorph?lang=la&lookup="
	input      = kingpin.Arg("input", "File or Folder containing latin text to be lemmatized").Required().File()
	outputPath = kingpin.Arg("output", "File or Folder to output the lemmatized text").Required().String()
)

func main() {
	kingpin.Version("0.0.1")
	kingpin.Parse()

	defer input.Close()

	inputInfo, err := input.Stat()
	if err != nil {
		panic(err)
	}

	outputInfo, err := os.Stat(*outputPath)
	if !os.IsNotExist(err) && err != nil {
		panic(err)
	}

	if !os.IsNotExist(err) && outputInfo.IsDir() != inputInfo.IsDir() {
		fmt.Fprintf(os.Stderr, "ERROR: Output and Input must both be either files or folders\n")
		return
	}

	if !inputInfo.IsDir() {
		LemmatizeFile(*input, *outputPath)
	} else {
		fileInfos, err := input.Readdir(0)
		if err != nil {
			panic(err)
		}
		if _, err := os.Stat(*outputPath); os.IsNotExist(err) {
			os.Mkdir(*outputPath, 0777)
		}
		for _, fi := range fileInfos {
			f, err := os.Open(filepath.Join(input.Name(), fi.Name()))
			defer f.Close()
			if err != nil {
				panic(err)
			}

			LemmatizeFile(f, filepath.Join(*outputPath, fi.Name()))
		}
	}

}

func LemmatizeFile(inputFile *os.File, outputPath string) {
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {

		fmt.Printf("WARNING: ")
		for {
			fmt.Printf("File '%s' already exists. Overwrite? (y/n): ", outputPath)
			var yn string
			fmt.Scanf("%s\n", &yn)
			if yn == "y" || yn == "Y" {
				break
			} else if yn == "n" || yn == "N" {
				return
			}
			fmt.Println("Invalid Input.")
		}
	}
	outFile, err := os.Create(outputPath)
	defer outFile.Close()
	if err != nil {
		panic(err)
	}
	lr := NewLemmaReader(inputFile)
	fmt.Printf("Lemmatizing '%s' into '%s'", inputFile.Name(), outputPath)
	ticker := time.NewTicker(time.Millisecond * 500)
	go func() {
		for _ = range ticker.C {
			fmt.Print(".")
		}
	}()
	for w, done := lr.Read(); !done; w, done = lr.Read() {
		outFile.WriteString(w + " ")
	}
	outFile.WriteString("\n")
	ticker.Stop()
	fmt.Println()
}

func LemmatizeText(f io.Reader) *LemmaReader {
	return NewLemmaReader(f)
}

type LemmaReader struct {
	s scanner.Scanner
}

func NewLemmaReader(f io.Reader) *LemmaReader {
	lr := &LemmaReader{}
	lr.s.Init(f)
	return lr
}

func (l *LemmaReader) Read() (string, bool) {
	done := false
	lemmyd := ""

	for lemmyd == "" && !done {
		if l.s.Scan() == scanner.EOF {
			done = true
		}
		lemmyd = LemmatizeWord(l.s.TokenText())
	}

	return lemmyd, done
}

func LemmatizeWord(word string) string {
	resp, err := http.Get(urlBase + word)
	if err != nil {
		panic(err)

	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	lemXml := Analyses{}
	err = xml.Unmarshal(body, &lemXml)
	if err != nil {
		panic(err)
	}

	return lemXml.Analysis.Lemma
}

type Analysis struct {
	XMLName xml.Name `xml:"analysis"`
	Lemma   string   `xml:"lemma"`
}

type Analyses struct {
	XMLName  xml.Name `xml:"analyses"`
	Analysis Analysis
}
