package main

// This program generates error codes objects based on ocfl spec.
//
// from this directory:
// go run gen.go > ../codes.go

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/srerickson/ocfl/spec"
)

var specs = map[string]string{
	"1.0": "codes-ocflv1.0.csv",
	"1.1": "codes-ocflv1.1.csv",
}

type Spec struct {
	Description string // code description
	URL         string // URL to spect
}

type CodeInfo struct {
	Num     string
	Comment string
	Specs   map[spec.Num]Spec
}

func main() {
	tpl := template.Must(template.ParseFiles(`codes_gen.go.tmpl`))
	codes := map[string]*CodeInfo{}

	for specnum, file := range specs {
		f, err := os.Open(file)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		reader := csv.NewReader(f)
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
			comment := fmt.Sprintf("%s: %s", row[0], row[1])
			v := spec.MustParse(specnum)
			if code := codes[num]; code != nil {
				code.Specs[v] = Spec{
					Description: desc,
					URL:         url,
				}
				continue
			}
			codes[num] = &CodeInfo{
				Num:     num,
				Comment: comment,
				Specs: map[spec.Num]Spec{
					v: {
						Description: desc,
						URL:         url,
					},
				},
			}

		}
		if err != nil {
			log.Fatal(err)
		}
	}
	tpl.Execute(os.Stdout, codes)
}