package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

func excelTimeString() string {
	d := time.Now()
	return fmt.Sprintf("%04d%02d%02d_%02d%02d%02d", d.Year(), int(d.Month()), d.Day(), d.Hour(), d.Minute(), d.Second())
}

func sanitizeSheetName(s string) string {
	re := regexp.MustCompile(`[*?:/\\\[\]]`)
	s = re.ReplaceAllString(s, " ")
	if len([]rune(s)) > 31 {
		s = string([]rune(s)[:31])
	}
	if s == "" {
		s = "Sheet"
	}
	return s
}

func buildExcel(ds *DataSet, lang string) ([]byte, string, error) {
	f := excelize.NewFile()
	defer f.Close()
	defaultSheet := f.GetSheetName(0)
	first := true

	typeName := map[string]string{}
	for _, p := range ds.TypeMap {
		typeName[p[0]] = p[1]
	}
	nameFor := func(k string) string {
		if v, ok := typeName[k]; ok && v != "" {
			return v
		}
		return k
	}

	customFont := i18nText(lang, "excel.customFont")
	headerKeys := []string{"time", "name", "type", "rank", "total", "pity", "remark"}
	headers := make([]string, 0, len(headerKeys))
	for _, k := range headerKeys {
		headers = append(headers, i18nText(lang, "excel.header."+k))
	}

	border := []excelize.Border{
		{Type: "top", Color: "ffc4c2bf", Style: 1},
		{Type: "left", Color: "ffc4c2bf", Style: 1},
		{Type: "bottom", Color: "ffc4c2bf", Style: 1},
		{Type: "right", Color: "ffc4c2bf", Style: 1},
	}
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Family: customFont, Color: "ff757575", Bold: true},
		Fill:   excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"ffdbd7d3"}},
		Border: border,
	})
	if err != nil {
		return nil, "", err
	}
	bodyStyles := map[string]int{}
	for rank, color := range map[string]string{"2": "ff8e8e8e", "3": "ffa256e1", "4": "ffbd6932"} {
		st, err := f.NewStyle(&excelize.Style{
			Font:   &excelize.Font{Family: customFont, Color: color, Bold: rank != "2"},
			Fill:   excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"ffebebeb"}},
			Border: border,
		})
		if err != nil {
			return nil, "", err
		}
		bodyStyles[rank] = st
	}

	for _, key := range orderKeys(ds.Result) {
		logs := ds.Result[key]
		sheet := sanitizeSheetName(nameFor(key))
		if first {
			f.SetSheetName(defaultSheet, sheet)
			first = false
		} else {
			idx, err := f.NewSheet(sheet)
			if err == nil {
				f.SetActiveSheet(idx)
			}
		}
		for i, h := range headers {
			col, _ := excelize.ColumnNumberToName(i + 1)
			_ = f.SetCellValue(sheet, fmt.Sprintf("%s1", col), h)
		}
		_ = f.SetCellStyle(sheet, "A1", "G1", headerStyle)
		widths := []float64{24, 14, 8, 8, 8, 8, 8}
		if !strings.Contains(ds.Lang, "zh-") {
			widths = []float64{24, 32, 16, 12, 12, 12, 8}
		}
		for i, w := range widths {
			col, _ := excelize.ColumnNumberToName(i + 1)
			_ = f.SetColWidth(sheet, col, col, w)
		}
		total := 0
		pity := 0
		for r, log := range logs {
			total++
			pity++
			rankName := i18nText(lang, "ui.data.star4")
			if log.RankType == "2" {
				rankName = i18nText(lang, "ui.data.star2")
			} else if log.RankType == "3" {
				rankName = i18nText(lang, "ui.data.star3")
			}
			row := r + 2
			_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), log.Time)
			_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), log.Name)
			_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), log.ItemType)
			_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), rankName)
			_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), total)
			_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", row), pity)
			style := bodyStyles[log.RankType]
			if style == 0 {
				style = bodyStyles["2"]
			}
			_ = f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("G%d", row), style)
			if log.RankType == "4" {
				pity = 0
			}
		}
		_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	}
	if first {
		f.SetSheetName(defaultSheet, "rawData")
	}
	raw := "rawData"
	if !first {
		idx, err := f.NewSheet(raw)
		if err == nil {
			f.SetActiveSheet(idx)
		}
	}
	rawHeaders := []string{"gacha_id", "gacha_type", "id", "item_id", "item_type", "lang", "name", "rank_type", "time", "uid"}
	for i, h := range rawHeaders {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetCellValue(raw, fmt.Sprintf("%s1", col), h)
	}
	rawRow := 2
	for _, key := range orderKeys(ds.Result) {
		for _, log := range ds.Result[key] {
			vals := []interface{}{log.GachaID, log.GachaType, log.ID, log.ItemID, log.ItemType, ds.Lang, log.Name, log.RankType, log.Time, ds.UID}
			for i, v := range vals {
				col, _ := excelize.ColumnNumberToName(i + 1)
				_ = f.SetCellValue(raw, fmt.Sprintf("%s%d", col, rawRow), v)
			}
			rawRow++
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", err
	}
	filename := i18nText(lang, "excel.filePrefix") + "_" + excelTimeString() + ".xlsx"
	return buf.Bytes(), filename, nil
}
