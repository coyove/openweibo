// Package jiebago is the Golang implemention of [Jieba](https://github.com/fxsjy/jieba), Python Chinese text segmentation module.
package jiebago

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/coyove/iis/common/jiebago/dictionary"
	"github.com/coyove/iis/common/jiebago/util"
)

var (
	reEng         = regexp.MustCompile(`[[:alnum:]]`)
	reHanCutAll   = regexp.MustCompile(`(\p{Han}+)`)
	reSkipCutAll  = regexp.MustCompile(`[^[:alnum:]+#\n]`)
	reHanDefault  = regexp.MustCompile(`([\p{Han}+[:alnum:]+#&\._]+)`)
	reSkipDefault = regexp.MustCompile(`(\r\n|\s)`)
	defaultSeg    Segmenter
)

func LoadDictionary() {
	fn, err := os.OpenFile(filepath.Join(os.TempDir(), "jieba"), os.O_CREATE|os.O_RDWR, 0777)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}
	}

	fi, _ := fn.Stat()
	if fi.Size() > 0 {
		fmt.Println("Jieba dict is presented", fn.Name())
		fn.Close()
		if err := defaultSeg.LoadDictionary(fn.Name()); err != nil {
			panic(err)
		}
		return
	}

	req, _ := http.NewRequest("GET", "https://f002.backblazeb2.com/file/fweibomedia/dict.txt", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()

	fmt.Println("Downloading Jieba Dict")
	n, _ := io.Copy(fn, resp.Body)
	fmt.Println("Downloaded Jieba Dict:", n)
	fn.Close()

	if err := defaultSeg.LoadDictionary(fn.Name()); err != nil {
		panic(err)
	}
}

// Segmenter is a Chinese words segmentation struct.
type Segmenter struct {
	dict *Dictionary
}

// Frequency returns a word's frequency and existence
func (seg *Segmenter) Frequency(word string) (float64, bool) {
	return seg.dict.Frequency(word)
}

// AddWord adds a new word with frequency to dictionary
func (seg *Segmenter) AddWord(word string, frequency float64) {
	seg.dict.AddToken(dictionary.NewToken(word, frequency, ""))
}

// DeleteWord removes a word from dictionary
func (seg *Segmenter) DeleteWord(word string) {
	seg.dict.AddToken(dictionary.NewToken(word, 0.0, ""))
}

// LoadDictionary loads dictionary from given file name. Everytime
// LoadDictionary is called, previously loaded dictionary will be cleard.
func (seg *Segmenter) LoadDictionary(fileName string) error {
	seg.dict = &Dictionary{freqMap: make(map[string]float64)}
	return seg.dict.loadDictionary(fileName)
}

// LoadUserDictionary loads a user specified dictionary, it must be called
// after LoadDictionary, and it will not clear any previous loaded dictionary,
// instead it will override exist entries.
func (seg *Segmenter) LoadUserDictionary(fileName string) error {
	return seg.dict.loadDictionary(fileName)
}

func (seg *Segmenter) dag(runes []rune) map[int][]int {
	dag := make(map[int][]int)
	n := len(runes)
	var frag []rune
	var i int
	for k := 0; k < n; k++ {
		dag[k] = make([]int, 0)
		i = k
		frag = runes[k : k+1]
		for {
			freq, ok := seg.dict.Frequency(string(frag))
			if !ok {
				break
			}
			if freq > 0.0 {
				dag[k] = append(dag[k], i)
			}
			i++
			if i >= n {
				break
			}
			frag = runes[k : i+1]
		}
		if len(dag[k]) == 0 {
			dag[k] = append(dag[k], k)
		}
	}
	return dag
}

type route struct {
	frequency float64
	index     int
}

func (seg *Segmenter) cutAll(sentence string) []string {
	result := []string{}
	runes := []rune(sentence)
	dag := seg.dag(runes)
	start := -1
	ks := make([]int, len(dag))
	for k := range dag {
		ks[k] = k
	}
	var l []int
	for k := range ks {
		l = dag[k]
		if len(l) == 1 && k > start {
			result = append(result, string(runes[k:l[0]+1]))
			start = l[0]
			continue
		}
		for _, j := range l {
			if j > k {
				result = append(result, string(runes[k:j+1]))
				start = j
			}
		}
	}
	return result
}

// Cut cuts a sentence into words using full mode.
// Full mode gets all the possible words from the sentence.
// Fast but not accurate.
func (seg *Segmenter) Cut(sentence string, n int) []string {
	result := map[string]int{}
	for _, block := range util.RegexpSplit(reHanCutAll, sentence, -1) {
		if len(block) == 0 {
			continue
		}
		if reHanCutAll.MatchString(block) {
			for _, t := range seg.cutAll(block) {
				if len(t) > 3 {
					result[t]++
				}
			}
			continue
		}
		for _, subBlock := range reSkipCutAll.Split(block, -1) {
			subBlock = strings.TrimSpace(subBlock)
			subBlock = strings.ReplaceAll(subBlock, "\n", "")
			if len(subBlock) > 3 {
				result[subBlock]++
			}
		}
	}
	sorts := [8][]string{}
	for k, v := range result {
		if v > 7 {
			v = 7
		}
		sorts[v] = append(sorts[v], k)
	}
	final := make([]string, 0, len(result))
	for i := 7; i > 0; i-- {
		sort.Slice(sorts[i], func(ii, jj int) bool { return sorts[i][ii] > sorts[i][jj] })
		final = append(final, sorts[i]...)
		if len(final) >= n {
			final = final[:n]
			break
		}
	}
	return final
}

func Cut(sentence string, n int) []string {
	return defaultSeg.Cut(sentence, n)
}
