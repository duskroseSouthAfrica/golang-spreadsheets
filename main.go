package main

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

type Spreadsheet struct {
	Headers     []string
	Rows        [][]string
	NumericCols []int
}

type DisplayData struct {
	Headers     []string
	Rows        [][]string
	NumericCols []int
}

type CalculationResult struct {
	Col   string
	Value float64
}

type ResultPage struct {
	Operation string
	Results   []CalculationResult
}

var uploadTemplate = template.Must(template.ParseFiles("upload.html"))
var displayTemplate = template.Must(template.ParseFiles("display.html"))
var resultTemplate = template.Must(template.ParseFiles("results.html"))

// GLOBAL in-memory storage for the last uploaded spreadsheet
var lastSpreadsheet Spreadsheet

func main() {
	http.HandleFunc("/", uploadHandler)
	http.HandleFunc("/display", displayHandler)
	http.HandleFunc("/calculate", calculateHandler)

	fmt.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	uploadTemplate.Execute(w, nil)
}

func displayHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	var data Spreadsheet

	if strings.HasSuffix(header.Filename, ".csv") {
		csvReader := csv.NewReader(file)
		rows, err := csvReader.ReadAll()
		if err != nil {
			http.Error(w, "Failed to read CSV", http.StatusBadRequest)
			return
		}
		if len(rows) == 0 {
			http.Error(w, "Empty CSV file", http.StatusBadRequest)
			return
		}
		data.Headers = rows[0]
		data.Rows = rows[1:]
	} else {
		f, err := excelize.OpenReader(file)
		if err != nil {
			http.Error(w, "Failed to read Excel file", http.StatusBadRequest)
			return
		}
		sheet := f.GetSheetName(0)
		rows, _ := f.GetRows(sheet)
		if len(rows) == 0 {
			http.Error(w, "Empty Excel file", http.StatusBadRequest)
			return
		}
		data.Headers = rows[0]
		data.Rows = rows[1:]
	}

	// Detect numeric columns
	for col := range data.Headers {
		isNumeric := true
		for _, row := range data.Rows {
			if col >= len(row) {
				continue
			}
			if _, err := strconv.ParseFloat(row[col], 64); err != nil {
				isNumeric = false
				break
			}
		}
		if isNumeric {
			data.NumericCols = append(data.NumericCols, col)
		}
	}

	// Save spreadsheet in global variable for calculation
	lastSpreadsheet = data

	displayTemplate.Execute(w, DisplayData(data))
}

func calculateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	r.ParseForm()
	cols := r.Form["cols"]
	op := r.FormValue("operation")

	var results []CalculationResult

	for _, colName := range cols {
		colIndex := -1
		for i, h := range lastSpreadsheet.Headers {
			if h == colName {
				colIndex = i
				break
			}
		}
		if colIndex == -1 {
			continue
		}

		sum := 0.0
		count := 0.0
		for _, row := range lastSpreadsheet.Rows {
			if colIndex >= len(row) {
				continue
			}
			val, err := strconv.ParseFloat(row[colIndex], 64)
			if err != nil {
				continue
			}
			sum += val
			count++
		}

		result := 0.0
		if op == "sum" {
			result = sum
		} else if op == "average" && count > 0 {
			result = sum / count
		}

		results = append(results, CalculationResult{
			Col:   colName,
			Value: result,
		})
	}

	err := resultTemplate.Execute(w, ResultPage{
		Operation: op,
		Results:   results,
	})
	if err != nil {
		http.Error(w, "Failed to render results", http.StatusInternalServerError)
		log.Println("Template execution error:", err)
	}
}
