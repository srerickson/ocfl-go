package main

import (
	"encoding/csv"
	"log"
	"os"
	"strings"
	"text/template"
)

// generates error objects based on ocfl spec

func main() {
	tpl := template.Must(template.ParseFiles(`errors_gen.go.tpl`))
	f, err := os.Open(`errors.csv`)
	if err != nil {
		log.Fatal(err)
	}
	reader := csv.NewReader(f)
	reader.Comma = '\t'
	records, err := reader.ReadAll()
	//some cleanup
	for i := range records {
		for j := range records[i] {
			records[i][j] = strings.Trim(records[i][j], `’‘ `)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	tpl.Execute(os.Stdout, records)
}
