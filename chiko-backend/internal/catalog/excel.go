package catalog

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// templateColumns matches ТЗ раздел 9.3.
// Headers are in Russian for MVP. Step 5.1 (i18n) will localise these.
var templateColumns = []string{"Название", "Категория", "Цена", "Единица", "Остаток"}

// ExportTemplate generates a downloadable .xlsx template with one example row.
func ExportTemplate() ([]byte, error) {
	f := excelize.NewFile()
	sh := "Каталог"
	if err := f.SetSheetName("Sheet1", sh); err != nil {
		return nil, fmt.Errorf("excel.ExportTemplate: %w", err)
	}

	for col, header := range templateColumns {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sh, cell, header)
	}

	example := []any{"Пример товара", "Напитки", 12500.0, "шт", 100.0}
	for col, val := range example {
		cell, _ := excelize.CoordinatesToCellName(col+1, 2)
		f.SetCellValue(sh, cell, val)
	}

	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err == nil {
		last, _ := excelize.CoordinatesToCellName(len(templateColumns), 1)
		f.SetCellStyle(sh, "A1", last, style)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("excel.ExportTemplate write: %w", err)
	}
	return buf.Bytes(), nil
}

// ParseImportFile reads an .xlsx file (ТЗ раздел 9.3 template format).
// Returns the first previewLimit rows as preview, and all rows for import.
func ParseImportFile(r io.Reader, previewLimit int) (preview []ImportRow, all []ImportRow, warnings []string, err error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("excel: не удалось открыть файл: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, nil, fmt.Errorf("excel: файл не содержит листов")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("excel.GetRows: %w", err)
	}
	if len(rows) < 2 {
		return nil, nil, nil, fmt.Errorf("excel: файл пуст или содержит только заголовок")
	}

	idx := detectColumns(rows[0])

	for i, row := range rows[1:] {
		ir, warn := parseRow(row, idx, i+2)
		if warn != "" {
			warnings = append(warnings, warn)
		}
		if ir == nil {
			continue
		}
		all = append(all, *ir)
		if len(preview) < previewLimit {
			preview = append(preview, *ir)
		}
	}
	return preview, all, warnings, nil
}

// ExportCatalog exports products to an .xlsx file.
func ExportCatalog(products []Product, categories []Category) ([]byte, error) {
	catNames := make(map[uuid.UUID]string, len(categories))
	for _, c := range categories {
		catNames[c.ID] = c.Name
	}

	f := excelize.NewFile()
	sh := "Каталог"
	f.SetSheetName("Sheet1", sh)

	for col, h := range templateColumns {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sh, cell, h)
	}

	for i, p := range products {
		row := i + 2
		catName := ""
		if p.CategoryID != nil {
			catName = catNames[*p.CategoryID]
		}
		vals := []any{p.Name, catName, p.Price, p.Unit, p.StockQty}
		for col, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			f.SetCellValue(sh, cell, v)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("excel.ExportCatalog: %w", err)
	}
	return buf.Bytes(), nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

type colIdx struct{ name, cat, price, unit, stock int }

func detectColumns(header []string) colIdx {
	idx := colIdx{name: -1, cat: -1, price: -1, unit: -1, stock: -1}
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "название", "name", "наименование", "товар":
			idx.name = i
		case "категория", "category":
			idx.cat = i
		case "цена", "price", "стоимость":
			idx.price = i
		case "единица", "unit", "ед", "ед.изм", "ед. изм.":
			idx.unit = i
		case "остаток", "stock", "количество", "кол-во":
			idx.stock = i
		}
	}
	return idx
}

func parseRow(row []string, idx colIdx, rowNum int) (*ImportRow, string) {
	get := func(i int) string {
		if i < 0 || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	name := get(idx.name)
	if name == "" {
		return nil, fmt.Sprintf("строка %d: пустое название, пропущена", rowNum)
	}

	price := parseFloat(get(idx.price))
	stock := parseFloat(get(idx.stock))

	unit := get(idx.unit)
	if unit == "" {
		unit = "шт"
	}

	return &ImportRow{
		Name:     name,
		Category: get(idx.cat),
		Price:    price,
		Unit:     unit,
		StockQty: stock,
	}, ""
}

func parseFloat(s string) float64 {
	// Replace comma decimal separator (common in Russian locale Excel).
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, " ", "") // strip thousands separator
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func colLetter(n int) string {
	buf := &bytes.Buffer{}
	for n > 0 {
		n--
		buf.WriteByte(byte('A' + n%26))
		n /= 26
	}
	b := buf.Bytes()
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}
