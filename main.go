package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type Spreadsheet struct {
	Headers     []string
	Rows        [][]string
	NumericCols []int
	FileName    string
	UploadTime  time.Time
	FileSize    int64
}

type DisplayData struct {
	Headers     []string
	Rows        [][]string
	NumericCols []int
	FileName    string
	FileSize    string
	RowCount    int
}

type CalculationResult struct {
	Col   string
	Value float64
}

type ResultPage struct {
	Operation string
	Results   []CalculationResult
	FileName  string
	Timestamp string
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// Template functions
var templateFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"contains": func(slice []int, item int) bool {
		for _, s := range slice {
			if s == item {
				return true
			}
		}
		return false
	},
	"formatSize": func(size int64) string {
		const unit = 1024
		if size < unit {
			return fmt.Sprintf("%d B", size)
		}
		div, exp := int64(unit), 0
		for n := size / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
	},
	"formatNumber": func(f float64) string {
		return fmt.Sprintf("%.2f", f)
	},
}

var uploadTemplate = template.Must(template.New("upload.html").Funcs(templateFuncs).ParseFiles("upload.html"))
var displayTemplate = template.Must(template.New("display.html").Funcs(templateFuncs).ParseFiles("display.html"))
var resultTemplate = template.Must(template.New("results.html").Funcs(templateFuncs).ParseFiles("results.html"))

// GLOBAL in-memory storage for the last uploaded spreadsheet
var lastSpreadsheet Spreadsheet

const (
	MaxFileSize = 10 << 20 // 10MB
	MaxRows     = 10000
)

func main() {
	// Serve static CSS files correctly
	fs := http.FileServer(http.Dir("./"))
	http.Handle("/upload.css", http.StripPrefix("/", fs))
	http.Handle("/display.css", http.StripPrefix("/", fs))
	http.Handle("/results.css", http.StripPrefix("/", fs))

	// App endpoints
	http.HandleFunc("/", uploadHandler)
	http.HandleFunc("/display", displayHandler)
	http.HandleFunc("/calculate", calculateHandler)
	http.HandleFunc("/api/validate", validateFileHandler)
	http.HandleFunc("/health", healthHandler)

	fmt.Println("🚀 Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}


// Handlers

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

func validateFileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "Method not allowed"})
		return
	}
	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: map[string]string{"status": "File valid"}})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
	})
}

// CSV / Excel processing
func processCSV(file io.Reader) (Spreadsheet, error) {
	var data Spreadsheet
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return data, err
	}
	if len(rows) == 0 {
		return data, fmt.Errorf("empty CSV")
	}
	headers := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		h = strings.TrimSpace(h)
		if h == "" {
			h = fmt.Sprintf("Column_%d", i+1)
		}
		headers[i] = h
	}
	data.Headers = headers
	data.Rows = rows[1:]
	return data, nil
}

func processExcel(file io.Reader) (Spreadsheet, error) {
	var data Spreadsheet
	f, err := excelize.OpenReader(file)
	if err != nil {
		return data, err
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		return data, fmt.Errorf("no sheets")
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return data, err
	}
	if len(rows) == 0 {
		return data, fmt.Errorf("empty Excel")
	}
	headers := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		h = strings.TrimSpace(h)
		if h == "" {
			h = fmt.Sprintf("Column_%d", i+1)
		}
		headers[i] = h
	}
	data.Headers = headers
	data.Rows = rows[1:]
	return data, nil
}

// Helpers

func detectNumericColumns(data Spreadsheet) []int {
	var numericCols []int
	for col := range data.Headers {
		if isColumnNumeric(data, col) {
			numericCols = append(numericCols, col)
		}
	}
	return numericCols
}

func isColumnNumeric(data Spreadsheet, colIndex int) bool {
	numericCount := 0
	totalCount := 0
	for _, row := range data.Rows {
		if colIndex >= len(row) {
			continue
		}
		val := strings.TrimSpace(row[colIndex])
		if val == "" {
			continue
		}
		totalCount++
		if _, err := strconv.ParseFloat(val, 64); err == nil {
			numericCount++
		}
	}
	if totalCount == 0 {
		return false
	}
	return float64(numericCount)/float64(totalCount) >= 0.8
}

func performCalculation(data Spreadsheet, colIndex int, op string) (float64, error) {
	var values []float64
	for _, row := range data.Rows {
		if colIndex >= len(row) {
			continue
		}
		val := strings.TrimSpace(row[colIndex])
		if val == "" {
			continue
		}
		num, err := strconv.ParseFloat(val, 64)
		if err != nil {
			continue
		}
		values = append(values, num)
	}
	if len(values) == 0 {
		return 0, fmt.Errorf("no numeric values")
	}
	switch op {
	case "sum":
		return sum(values), nil
	case "average":
		return avg(values), nil
	case "median":
		return median(values), nil
	case "min":
		return min(values), nil
	case "max":
		return max(values), nil
	case "count":
		return float64(len(values)), nil
	case "std":
		return std(values), nil
	default:
		return 0, fmt.Errorf("unsupported operation")
	}
}

// Calculations
func sum(vals []float64) float64 { s := 0.0; for _, v := range vals { s += v }; return s }
func avg(vals []float64) float64 { return sum(vals) / float64(len(vals)) }
func median(vals []float64) float64 {
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-1-i; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}
func min(vals []float64) float64 { m := vals[0]; for _, v := range vals[1:] { if v < m { m = v } }; return m }
func max(vals []float64) float64 { m := vals[0]; for _, v := range vals[1:] { if v > m { m = v } }; return m }
func std(vals []float64) float64 {
	if len(vals) <= 1 { return 0 }
	mean := avg(vals)
	sumSq := 0.0
	for _, v := range vals { d := v - mean; sumSq += d * d }
	return math.Sqrt(sumSq / float64(len(vals)-1))
}

func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit { return fmt.Sprintf("%d B", size) }
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit { div *= unit; exp++ }
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
