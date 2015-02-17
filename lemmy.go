package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"text/scanner"
	"time"

	"github.com/psywolf/autocache"
	"gopkg.in/alecthomas/kingpin.v1"
)

var (
	urlBase      = "http://www.perseus.tufts.edu/hopper/xmlmorph?lang=la&lookup="
	MAX_REQUESTS = kingpin.Flag("requestCount", "The max number of concurrant requests that lemmy should send to www.perseus.tufts.edu (More isn't always better)").Default("10").Short('r').Int()
	CACHE_SIZE   = kingpin.Flag("cacheSize", "The max number of lemmatized words to cache").Default("10000").Short('c').Int()
	verbose      = kingpin.Flag("verbose", "Print extra debugging information").Short('v').Bool()
	input        = kingpin.Arg("input", "File or Folder containing latin text to be lemmatized").Required().File()
	outputPath   = kingpin.Arg("output", "File or Folder to output the lemmatized text").Required().String()
)

func main() {
	kingpin.Version("1.0.3")
	kingpin.Parse()

	defer input.Close()

	if *MAX_REQUESTS < 1 {
		fmt.Fprintf(os.Stderr, "ERROR: requestCount of %d is invalid.  Must be greater than 0.\n", *MAX_REQUESTS)
		os.Exit(1)
	}

	if *CACHE_SIZE < 0 {
		fmt.Fprintf(os.Stderr, "ERROR: cacheSize of %d is invalid.  Must be at least 0.\n", *CACHE_SIZE)
		os.Exit(1)
	}

	rand.Seed(time.Now().UnixNano())

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
		os.Exit(1)
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
	i := 1
	for w, done := lr.Read(); !done; w, done = lr.Read() {
		outFile.WriteString(w + " ")
		if i%50 == 0 {
			fmt.Print(".")
		}
		i++
	}
	outFile.WriteString("\n")
	fmt.Println()
}

func LemmatizeText(f io.Reader) *LemmaReader {
	return NewLemmaReader(f)
}

type LemmaReader struct {
	outChan chan *postLemMsg
	cache   *autocache.Cache
	s       scanner.Scanner
}

func NewLemmaReader(f io.Reader) *LemmaReader {
	l := &LemmaReader{outChan: make(chan *postLemMsg)}

	inChan := make(chan *preLemMsg)
	l.s.Init(f)
	//change the mode to only look for words and numbers
	//the default mode ignores go style comments
	//and chokes on unmatched quotes or backticks
	l.s.Mode = scanner.ScanIdents

	httpClient := &http.Client{}
	l.cache = autocache.New(*CACHE_SIZE, func(word string) (string, error) {
		lemmyd, err := LemmatizeWord(httpClient, word)
		for err != nil {
			if *verbose {
				fmt.Fprintf(os.Stderr, "\nError on word '%s'\ntype is '%T'\nerr is '%s'\nRETRYING\n", word, err, err)
			}
			time.Sleep(time.Duration(500+rand.Intn(500)) * time.Millisecond)
			lemmyd, err = LemmatizeWord(httpClient, word)
		}
		return lemmyd, nil
	})

	go l.populateInChan(inChan)
	go l.processInChan(inChan)
	return l
}

func (l *LemmaReader) populateInChan(inChan chan *preLemMsg) {
	waitOn := make(chan struct{})

	if *MAX_REQUESTS > 1 {
		//kick off the first word
		go func(waitOn chan struct{}) {
			waitOn <- struct{}{}
		}(waitOn)
	}

	var proceed chan struct{}
	for l.s.Scan() != scanner.EOF {
		token := l.s.TokenText()
		if token != "" {
			if *verbose {
				fmt.Printf("token is '%s'\n", token)
			}
			proceed = make(chan struct{})
			inChan <- &preLemMsg{token, waitOn, proceed}
			waitOn = proceed
		}
	}
	close(inChan)
	//drain the last waitOn channel so the final goroutine doesn't block on it

	if *MAX_REQUESTS > 1 {
		<-waitOn
	}
}

func (l *LemmaReader) processInChan(inChan chan *preLemMsg) {

	doneChan := make(chan struct{})

	//create MAX_REQUESTS worker goroutines
	for i := 0; i < *MAX_REQUESTS; i++ {
		go func(inChan chan *preLemMsg, doneChan chan struct{}, i int) {
			for msg := range inChan {
				if *verbose {
					fmt.Printf("thread #%d word '%s': get from cache\n", i, msg.word)
				}
				lemmyd, err := l.cache.Get(msg.word)
				if err != nil {
					panic(err)
				}
				if *MAX_REQUESTS > 1 {
					if *verbose {
						fmt.Printf("thread #%d word '%s': waiting to output '%s'\n", i, msg.word, lemmyd)
					}
					<-msg.waitOn
				}
				if lemmyd != "" {
					l.outChan <- &postLemMsg{lemmyd, false}
				}
				if *MAX_REQUESTS > 1 {
					if *verbose {
						fmt.Printf("thread #%d word '%s': signaling next thread to proceed\n", i, msg.word)
					}
					msg.proceed <- struct{}{}
				}

				if *verbose {
					fmt.Printf("thread #%d word '%s': exiting thread\n", i, msg.word)
				}
			}
			doneChan <- struct{}{}
		}(inChan, doneChan, i)
	}

	//wait for all goroutines to complete
	for i := 0; i < *MAX_REQUESTS; i++ {
		<-doneChan
	}

	l.outChan <- &postLemMsg{"", true}
}

func (l *LemmaReader) Read() (string, bool) {
	msg := <-l.outChan
	return msg.word, msg.done
}

type preLemMsg struct {
	word    string
	waitOn  chan struct{}
	proceed chan struct{}
}

type postLemMsg struct {
	word string
	done bool
}

func LemmatizeWord(httpClient *http.Client, word string) (string, error) {
	if *verbose {
		fmt.Printf("lemmatizing word '%s'\n", word)
	}
	req, err := http.NewRequest("GET", urlBase+url.QueryEscape(word), nil)

	if err != nil {
		return "", err
	}

	req.Close = true

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("StatusCode %d when trying to lemmatize word '%s' (expected 200)", resp.StatusCode, word)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	lemXml := Analyses{}
	err = xml.Unmarshal(body, &lemXml)
	if err != nil {
		if *verbose {
			fmt.Fprintf(os.Stderr, "\n%s\n", body)
		}
		return "", err
	}
	if len(lemXml.Analysis) == 0 {
		return "", nil
	}
	return lemXml.Analysis[0].Lemma, nil
}

type Analysis struct {
	Lemma string `xml:"lemma"`
}

type Analyses struct {
	XMLName  xml.Name   `xml:"analyses"`
	Analysis []Analysis `xml:"analysis"`
}
