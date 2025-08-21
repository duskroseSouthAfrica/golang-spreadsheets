
// handlers.go
package main

import (
    "fmt"
    "log"
    "net/http"
    "strings"
    "time"
)

// GLOBAL in-memory storage for the last uploaded spreadsheet
var lastSpreadsheet Spreadsheet

const (
	MaxFileSize = 10 << 20 // 10MB
	MaxRows     = 10000
)

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if err := uploadTemplate.Execute(w, nil); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func displayHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(MaxFileSize); err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := strings.ToLower(header.Filename)
	if !strings.HasSuffix(filename, ".csv") &&
		!strings.HasSuffix(filename, ".xlsx") &&
		!strings.HasSuffix(filename, ".xls") {
		http.Error(w, "Invalid file type", http.StatusBadRequest)
		return
	}

	var data Spreadsheet
	data.FileName = header.Filename
	data.UploadTime = time.Now()
	data.FileSize = header.Size

	if strings.HasSuffix(filename, ".csv") {
		data, err = processCSV(file)
		if err != nil {
			http.Error(w, fmt.Sprintf("CSV error: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		data, err = processExcel(file)
		if err != nil {
			http.Error(w, fmt.Sprintf("Excel error: %v", err), http.StatusBadRequest)
			return
		}
	}

	if len(data.Rows) > MaxRows {
		http.Error(w, fmt.Sprintf("Too many rows (> %d)", MaxRows), http.StatusBadRequest)
		return
	}

	data.NumericCols = detectNumericColumns(data)
	if len(data.NumericCols) == 0 {
		http.Error(w, "No numeric columns found", http.StatusBadRequest)
		return
	}

	lastSpreadsheet = data

	displayData := DisplayData{
		Headers:     data.Headers,
		Rows:        data.Rows,
		NumericCols: data.NumericCols,
		FileName:    data.FileName,
		FileSize:    formatFileSize(data.FileSize),
		RowCount:    len(data.Rows),
	}

	if err := displayTemplate.Execute(w, displayData); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Failed to display data", http.StatusInternalServerError)
	}
}

func calculateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	cols := r.Form["cols"]
	op := r.FormValue("operation")

	if len(cols) == 0 || op == "" || len(lastSpreadsheet.Headers) == 0 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

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
		result, err := performCalculation(lastSpreadsheet, colIndex, op)
		if err != nil {
			continue
		}
		results = append(results, CalculationResult{Col: colName, Value: result})
	}

	if len(results) == 0 {
		http.Error(w, "No valid calculations", http.StatusBadRequest)
		return
	}

	page := ResultPage{
		Operation: strings.Title(op),
		Results:   results,
		FileName:  lastSpreadsheet.FileName,
		Timestamp: time.Now().Format("January 2, 2006 at 3:04 PM"),
	}

	if err := resultTemplate.Execute(w, page); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Failed to render results", http.StatusInternalServerError)
	}
}