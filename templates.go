// templates.go
package main

import (
	"html/template"
	"fmt"
)

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