// processing.go
package main

import (
    "encoding/csv"
    "github.com/xuri/excelize/v2"
    "io"
    "strings"
    "strconv"
    "fmt"
)

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