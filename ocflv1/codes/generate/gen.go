package main

// This program generates error codes objects based on ocfl ocfl.
//
// from this directory:
// go run gen.go > ../codes.go

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"
)

//go:embed codes-ocflv1.0.csv
var codes1_0 string

//go:embed codes-ocflv1.1.csv
var codes1_1 string

var specs = map[string]string{
	"1.0": codes1_0,
	"1.1": codes1_1,
}

type SpecRef struct {
	Description string // code description
	URL         string // URL to spect
}

type CodeEntry struct {
	Num     string
	Comment string
	Specs   map[string]SpecRef
}

func main() {
	tpl := template.Must(template.ParseFiles(`codes_gen.go.tmpl`))
	codes := map[string]*CodeEntry{}

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
				codes[num] = &CodeEntry{
					Num:     num,
					Comment: comment,
					Specs:   map[string]SpecRef{},
				}
			}
			codes[num].Specs[specnum] = SpecRef{
				Description: desc,
				URL:         url,
			}
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	tpl.Execute(os.Stdout, codes)
}
