// types.go
package main

import "time"

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