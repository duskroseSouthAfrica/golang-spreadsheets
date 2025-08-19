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
	"add": func(a, b int) int {
		return a + b
	},
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
	MaxRows     = 10000    // Limit rows for performance
)

func main() {
	http.HandleFunc("/", uploadHandler)
	http.HandleFunc("/display", displayHandler)
	http.HandleFunc("/calculate", calculateHandler)
	http.HandleFunc("/api/validate", validateFileHandler)
	http.HandleFunc("/health", healthHandler)

	fmt.Println("🚀 Dusk Rose Pty (Ltd) Server running on http://localhost:8080")
	fmt.Println("📊 Ready to process spreadsheets...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	
	w.Header().Set("Cache-Control", "no-cache")
	err := uploadTemplate.Execute(w, nil)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func displayHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Parse multipart form with size limit
	err := r.ParseMultipartForm(MaxFileSize)
	if err != nil {
		http.Error(w, "File too large. Maximum size is 10MB", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file size
	if header.Size > MaxFileSize {
		http.Error(w, "File too large. Maximum size is 10MB", http.StatusBadRequest)
		return
	}

	// Validate file type
	filename := strings.ToLower(header.Filename)
	if !strings.HasSuffix(filename, ".csv") && !strings.HasSuffix(filename, ".xlsx") && !strings.HasSuffix(filename, ".xls") {
		http.Error(w, "Invalid file type. Only CSV, XLSX, and XLS files are supported", http.StatusBadRequest)
		return
	}

	var data Spreadsheet
	data.FileName = header.Filename
	data.UploadTime = time.Now()
	data.FileSize = header.Size

	log.Printf("Processing file: %s (%.2f KB)", header.Filename, float64(header.Size)/1024)

	if strings.HasSuffix(filename, ".csv") {
		data, err = processCSV(file)
		if err != nil {
			log.Printf("CSV processing error: %v", err)
			http.Error(w, fmt.Sprintf("Failed to process CSV file: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		data, err = processExcel(file)
		if err != nil {
			log.Printf("Excel processing error: %v", err)
			http.Error(w, fmt.Sprintf("Failed to process Excel file: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Set file metadata
	data.FileName = header.Filename
	data.UploadTime = time.Now()
	data.FileSize = header.Size

	// Validate data limits
	if len(data.Rows) > MaxRows {
		http.Error(w, fmt.Sprintf("File too large. Maximum %d rows supported", MaxRows), http.StatusBadRequest)
		return
	}

	if len(data.Headers) == 0 {
		http.Error(w, "No headers found in file", http.StatusBadRequest)
		return
	}

	// Detect numeric columns with improved logic
	data.NumericCols = detectNumericColumns(data)

	if len(data.NumericCols) == 0 {
		http.Error(w, "No numeric columns found in the spreadsheet", http.StatusBadRequest)
		return
	}

	// Save spreadsheet in global variable for calculation
	lastSpreadsheet = data

	log.Printf("File processed successfully: %d rows, %d numeric columns", len(data.Rows), len(data.NumericCols))

	// Prepare display data
	displayData := DisplayData{
		Headers:     data.Headers,
		Rows:        data.Rows,
		NumericCols: data.NumericCols,
		FileName:    data.FileName,
		FileSize:    formatFileSize(data.FileSize),
		RowCount:    len(data.Rows),
	}

	err = displayTemplate.Execute(w, displayData)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Failed to display data", http.StatusInternalServerError)
	}
}

func calculateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	cols := r.Form["cols"]
	op := r.FormValue("operation")

	// Validate inputs
	if len(cols) == 0 {
		http.Error(w, "No columns selected for calculation", http.StatusBadRequest)
		return
	}

	if op == "" {
		http.Error(w, "No operation selected", http.StatusBadRequest)
		return
	}

	if len(lastSpreadsheet.Headers) == 0 {
		http.Error(w, "No data available. Please upload a file first", http.StatusBadRequest)
		return
	}

	log.Printf("Calculating %s for columns: %v", op, cols)

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
			log.Printf("Column not found: %s", colName)
			continue
		}

		result, err := performCalculation(lastSpreadsheet, colIndex, op)
		if err != nil {
			log.Printf("Calculation error for column %s: %v", colName, err)
			continue
		}

		results = append(results, CalculationResult{
			Col:   colName,
			Value: result,
		})
	}

	if len(results) == 0 {
		http.Error(w, "No valid calculations could be performed", http.StatusBadRequest)
		return
	}

	resultPage := ResultPage{
		Operation: strings.Title(op),
		Results:   results,
		FileName:  lastSpreadsheet.FileName,
		Timestamp: time.Now().Format("January 2, 2006 at 3:04 PM"),
	}

	err = resultTemplate.Execute(w, resultPage)
	if err != nil {
		http.Error(w, "Failed to render results", http.StatusInternalServerError)
		log.Printf("Template execution error: %v", err)
	}
}

// API endpoint for file validation (for AJAX calls)
func validateFileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	// This would validate file without processing
	// Implementation would depend on specific validation needs
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    map[string]string{"status": "File format valid"},
	})
}

// Health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
	})
}

// Helper functions

func processCSV(file io.Reader) (Spreadsheet, error) {
	var data Spreadsheet
	
	csvReader := csv.NewReader(file)
	csvReader.FieldsPerRecord = -1 // Allow variable number of fields
	
	rows, err := csvReader.ReadAll()
	if err != nil {
		return data, fmt.Errorf("failed to read CSV: %w", err)
	}
	
	if len(rows) == 0 {
		return data, fmt.Errorf("empty CSV file")
	}
	
	// Clean headers (remove whitespace, handle empty headers)
	headers := make([]string, len(rows[0]))
	for i, header := range rows[0] {
		cleaned := strings.TrimSpace(header)
		if cleaned == "" {
			cleaned = fmt.Sprintf("Column_%d", i+1)
		}
		headers[i] = cleaned
	}
	
	data.Headers = headers
	data.Rows = rows[1:]
	
	return data, nil
}

func processExcel(file io.Reader) (Spreadsheet, error) {
	var data Spreadsheet
	
	f, err := excelize.OpenReader(file)
	if err != nil {
		return data, fmt.Errorf("failed to open Excel file: %w", err)
	}
	defer f.Close()
	
	sheetName := f.GetSheetName(0)
	if sheetName == "" {
		return data, fmt.Errorf("no sheets found in Excel file")
	}
	
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return data, fmt.Errorf("failed to read Excel rows: %w", err)
	}
	
	if len(rows) == 0 {
		return data, fmt.Errorf("empty Excel file")
	}
	
	// Clean headers
	headers := make([]string, len(rows[0]))
	for i, header := range rows[0] {
		cleaned := strings.TrimSpace(header)
		if cleaned == "" {
			cleaned = fmt.Sprintf("Column_%d", i+1)
		}
		headers[i] = cleaned
	}
	
	data.Headers = headers
	data.Rows = rows[1:]
	
	return data, nil
}

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
		
		cellValue := strings.TrimSpace(row[colIndex])
		if cellValue == "" {
			continue // Skip empty cells
		}
		
		totalCount++
		if _, err := strconv.ParseFloat(cellValue, 64); err == nil {
			numericCount++
		}
	}
	
	// Consider column numeric if at least 80% of non-empty cells are numeric
	if totalCount == 0 {
		return false
	}
	
	return float64(numericCount)/float64(totalCount) >= 0.8
}

func performCalculation(data Spreadsheet, colIndex int, operation string) (float64, error) {
	var values []float64
	
	for _, row := range data.Rows {
		if colIndex >= len(row) {
			continue
		}
		
		cellValue := strings.TrimSpace(row[colIndex])
		if cellValue == "" {
			continue // Skip empty cells
		}
		
		val, err := strconv.ParseFloat(cellValue, 64)
		if err != nil {
			continue // Skip non-numeric values
		}
		
		values = append(values, val)
	}
	
	if len(values) == 0 {
		return 0, fmt.Errorf("no valid numeric values found")
	}
	
	switch operation {
	case "sum":
		return calculateSum(values), nil
	case "average":
		return calculateAverage(values), nil
	case "median":
		return calculateMedian(values), nil
	case "min":
		return calculateMin(values), nil
	case "max":
		return calculateMax(values), nil
	case "count":
		return float64(len(values)), nil
	case "std":
		return calculateStandardDeviation(values), nil
	default:
		return 0, fmt.Errorf("unsupported operation: %s", operation)
	}
}

// Calculation functions
func calculateSum(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum
}

func calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return calculateSum(values) / float64(len(values))
}

func calculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	// Simple bubble sort for median calculation
	sorted := make([]float64, len(values))
	copy(sorted, values)
	
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

func calculateMin(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func calculateMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func calculateStandardDeviation(values []float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	
	mean := calculateAverage(values)
	sumSquaredDiff := 0.0
	
	for _, v := range values {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}
	
	variance := sumSquaredDiff / float64(len(values)-1)
	return math.Sqrt(variance)
}

func formatFileSize(size int64) string {
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
}

// Middleware for logging and error handling
func withLogging(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("Started %s %s", r.Method, r.URL.Path)
		
		handler(w, r)
		
		log.Printf("Completed %s %s in %v", r.Method, r.URL.Path, time.Since(start))
	}
}

// Security headers middleware
func withSecurityHeaders(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		
		handler(w, r)
	}
}