// calculations.go
package main

import (
    "strings"   
    "strconv"   
    "math"
	"fmt"     
)

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