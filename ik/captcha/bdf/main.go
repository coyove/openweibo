package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
)

func main() {
	p, _ := ioutil.ReadFile("RobotoMono-Bold-20.bdf")
	lines := bytes.Split(p, []byte("\n"))

	buf := bytes.Buffer{}

	for i := 0; i < len(lines); {
		line := (string(lines[i]))
		if strings.HasPrefix(line, "STARTCHAR ") {
			ch := line[10]
			if ch >= 'A' && ch <= 'Z' && len(line) == 11 {
				j := i + 1
				for ; j < len(lines); j++ {
					if strings.HasPrefix(string(lines[j]), "BBX ") {
						break
					}
				}
				bbx := strings.Split(string(lines[j][4:]), " ")
				buf.WriteString(fmt.Sprint("// ", string(ch), bbx, "\n"))

				for ; j < len(lines); j++ {
					if strings.HasPrefix(string(lines[j]), "BITMAP") {
						j++
						break
					}
				}

				buf.WriteString("{\n")
				bits := []string{}
				minp0, mins0 := 100, 100

				for ; j < len(lines); j++ {
					if strings.HasPrefix(string(lines[j]), "ENDCHAR") {
						break
					}

					x := string(lines[j])
					v, _ := strconv.ParseInt(x, 16, 64)
					z := fmt.Sprintf("%019b", v)
					bits = append(bits, z)

					for i := 0; i < len(z); i++ {
						if z[i] != '0' {
							if i < minp0 {
								minp0 = i
							}
							break
						}
					}

					for i := 0; i < len(z); i++ {
						if z[len(z)-1-i] != '0' {
							if i < mins0 {
								mins0 = i
							}
							break
						}
					}
				}

				if 19-mins0-minp0 <= 11 {
					// do nothing
					diff := 11 - (19 - mins0 - minp0)
					diffs := diff * mins0 / (mins0 + minp0)
					mins0 -= diffs
					minp0 -= (diff - diffs)
				}

				for _, b := range bits {
					b = b[minp0 : len(b)-mins0]
					for _, c := range b {
						buf.WriteString(string(c) + ",")
					}
					buf.WriteString("\n")
				}

				buf.WriteString("},\n")
				i = j
			}
		}
		i++
	}

	fmt.Println(buf.String())
}
