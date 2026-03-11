package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// Format 输出格式枚举。
type Format string

const (
	// FormatTable 以对齐表格格式输出。
	FormatTable Format = "table"
	// FormatJSON 以 JSON 格式输出。
	FormatJSON Format = "json"
)

// Printer 统一的格式化输出器。
type Printer struct {
	Format Format
}

// PrintJSON 以带缩进的 JSON 格式输出任意数据到 stdout。
func (p *Printer) PrintJSON(data any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// PrintTable 以对齐表格格式输出数据到 stdout。
// headers 为列名，rows 为每行各列的字符串值。
func (p *Printer) PrintTable(headers []string, rows [][]string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// 输出表头
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// 输出数据行
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	return w.Flush()
}

// PrintTableToFile 以对齐表格格式输出数据到指定文件。
func (p *Printer) PrintTableToFile(path string, headers []string, rows [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("创建文件 %s 失败: %w", path, err)
	}
	defer f.Close()

	w := tabwriter.NewWriter(f, 0, 0, 2, ' ', 0)

	// 输出表头
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// 输出数据行
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	return w.Flush()
}
