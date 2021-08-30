package main

import (
	"fmt"
	"github.com/tealeg/xlsx"
	"os"
	"path"
	"strconv"
	"time"
)

// Header is the column meta-data.
type Header struct {
	Name string
	Wide bool
}

var headers = [...]Header{
	{Name: "Date"},
	{Name: "Open"},
	{Name: "High"},
	{Name: "Low"},
	{Name: "Close"},
	{Name: "Volume", Wide: true},
	{Name: "Market Cap", Wide: true},
	{Name: "Daily Average"},
	{Name: fmt.Sprintf("%d Days Average", AverageDays)},
	{Name: "Year"},
	{Name: "Month"},
	{Name: "Day"},
	{Name: "Average USD"},
	{Name: "Dollar rate"},
	{Name: "Date"},
	{Name: "Average ILS"},
	{Name: "Importer version"},
}

const longNumFormat = "#,##0"
const longColumnWidth = 20

// Report is is a high level structure providing price report management.
type Report struct {
	file     *xlsx.File
	fileName string
}

// CurrencySheet is a high level structure providing an access to a specific cryptocurrency report.
type CurrencySheet struct {
	sheet  *xlsx.Sheet
	report *Report
}

func generateReportForDeltaFileName(startTime *time.Time, endTime *time.Time) (string, error) {
	fileName := startTime.Format(dateFormat)
	if startTime != endTime {
		fileName += "_" + endTime.Format(dateFormat)
	}
	fileName += ".xlsx"

	folderPath, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return path.Join(folderPath, fileName), nil
}

// OpenReport opens or creates new report file.
func OpenReport(fileName string) (*Report, error) {

	file, err := xlsx.OpenFile(fileName)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		file = xlsx.NewFile()
	}

	defer file.Save(fileName)

	return &Report{
		file:     file,
		fileName: fileName,
	}, nil
}

// Save saves the report to the disk.
func (r *Report) Save() error {
	return r.file.Save(r.fileName)
}

// AddCurrency adds a new currency sheet to the report.
func (r *Report) AddCurrency(currency *Currency) (*CurrencySheet, error) {
	defer r.Save()

	var err error
	newSheet := false
	sheet := r.file.Sheet[currency.Name]
	if sheet == nil {
		sheet, err = r.file.AddSheet(currency.Name)
		if err != nil {
			return nil, err
		}
		newSheet = true
	}

	if newSheet {
		row := sheet.AddRow()

		for i, header := range headers {

			cell := row.AddCell()
			cell.SetValue(header.Name)

			if header.Wide {
				err := sheet.SetColWidth(i, i, longColumnWidth)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return &CurrencySheet{
		sheet:  sheet,
		report: r,
	}, nil
}

// Save saves the currency sheet.
func (s *CurrencySheet) Save() error {
	return s.report.Save()
}

// AddData adds a new data entry to the report.
func (s *CurrencySheet) AddData(data *FullHistoricPriceData) error {
	defer s.Save()

	row := s.sheet.AddRow()

	cell := row.AddCell()
	cell.SetValue(data.priceData.date.Format(dateFormat))

	cell = row.AddCell()
	cell.SetValue(data.priceData.open)

	cell = row.AddCell()
	cell.SetValue(data.priceData.high)

	cell = row.AddCell()
	cell.SetValue(data.priceData.low)

	cell = row.AddCell()
	cell.SetValue(data.priceData.close)

	cell = row.AddCell()
	cell.SetValue(data.priceData.volume)
	cell.NumFmt = longNumFormat

	cell = row.AddCell()
	cell.SetValue(data.priceData.marketCap)
	cell.NumFmt = longNumFormat

	dailyAverage := (data.priceData.open + data.priceData.close) / 2

	cell = row.AddCell()
	cell.SetValue(dailyAverage)

	cell = row.AddCell()
	cell.SetValue(data.average)

	cell = row.AddCell()
	cell.SetValue(dateIntToString(data.priceData.date.Day()))

	cell = row.AddCell()
	cell.SetValue(dateIntToString(int(data.priceData.date.Month())))

	cell = row.AddCell()
	cell.SetValue(dateIntToString(data.priceData.date.Year()))

	cell = row.AddCell()
	cell.SetValue(dailyAverage)

	cell = row.AddCell()
	cell.SetValue(data.shekelUsdRatio)

	cell = row.AddCell()
	cell.SetValue(data.priceData.date.Format(priorityDateFormat))
	cell = row.AddCell()
	cell.SetFloat(data.shekelUsdRatio * dailyAverage)

	cell = row.AddCell()
	cell.SetValue(Version)

	return nil
}

func dateIntToString(value int) string {

	stringValue := strconv.Itoa(value)

	if len(stringValue) == 1 {
		return "0" + stringValue
	}

	return stringValue

}
