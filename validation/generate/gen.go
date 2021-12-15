package main

// This program generates error codes objects based on ocfl spec.
//
// from this directory:
// go run gen.go > ../codes.go

import (
	"encoding/csv"
	"log"
	"os"
	"strings"
	"text/template"
)

func main() {
	tpl := template.Must(template.ParseFiles(`codes_gen.go.tpl`))
	f, err := os.Open(`codes.csv`)
	if err != nil {
		log.Fatal(err)
	}
	reader := csv.NewReader(f)
	//reader.Comma = ','
	records, err := reader.ReadAll()
	//some cleanup
	for i := range records {
		for j := range records[i] {
			records[i][j] = strings.Trim(records[i][j], `'`)
			records[i][j] = strings.ReplaceAll(records[i][j], `"`, `\"`)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	tpl.Execute(os.Stdout, records)
}
