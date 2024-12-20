//go:build ignore

package main

// This program generates error code for ocfl validatoin errors

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"
)

const filename = "codes_gen.go"

//go:embed codes-ocflv1.0.csv
var codes1_0 string

//go:embed codes-ocflv1.1.csv
var codes1_1 string

var specs = map[string]string{
	"1.0": codes1_0,
	"1.1": codes1_1,
}

type specRef struct {
	Description string // code description
	URL         string // URL to spect
}

type codeEntry struct {
	Num     string
	Comment string
	Specs   map[string]specRef
}

func main() {
	tpl := template.Must(template.ParseFiles(`codes_gen.go.tmpl`))
	codes := map[string]*codeEntry{}

	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	for specnum, raw := range specs {
		reader := csv.NewReader(strings.NewReader(raw))
		rows, err := reader.ReadAll()
		for _, row := range rows {
			// cleanup
			for i := range row {
				row[i] = strings.Trim(row[i], `'`)
				row[i] = strings.ReplaceAll(row[i], `"`, `\"`)
			}
			num := row[0]
			desc := row[1]
			url := row[2]
			comment := fmt.Sprintf("%s: %s", num, desc)
			if codes[num] == nil {
				codes[num] = &codeEntry{
					Num:     num,
					Comment: comment,
					Specs:   map[string]specRef{},
				}
			}
			codes[num].Specs[specnum] = specRef{
				Description: desc,
				URL:         url,
			}
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	tpl.Execute(f, codes)
}
