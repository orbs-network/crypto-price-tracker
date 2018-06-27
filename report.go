package main

import (
	"os"
	"path"
	"time"

	"github.com/kardianos/osext"
	"github.com/tealeg/xlsx"
)

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

func getReportFileName(startTime time.Time, endTime time.Time) (string, error) {
	fileName := startTime.Format(dateFormat)
	if startTime != endTime {
		fileName += "_" + endTime.Format(dateFormat)
	}
	fileName += ".xlsx"

	folderPath, err := osext.ExecutableFolder()
	if err != nil {
		return "", err
	}

	return path.Join(folderPath, fileName), nil
}

// DeleteReport deletes an existing report file.
func DeleteReport(startTime time.Time, endTime time.Time) error {
	fileName, err := getReportFileName(startTime, endTime)
	if err != nil {
		return err
	}

	err = os.Remove(fileName)
	if os.IsNotExist(err) {
		return nil
	} else {
		return err
	}
}

// NewReport create a new report file.
func NewReport(startTime time.Time, endTime time.Time) (*Report, error) {
	err := DeleteReport(startTime, endTime)
	if err != nil {
		return nil, err
	}

	return OpenReport(startTime, endTime)
}

// OpenReport opens or creates new report file.
func OpenReport(startTime time.Time, endTime time.Time) (*Report, error) {
	fileName, err := getReportFileName(startTime, endTime)
	if err != nil {
		return nil, err
	}

	file, err := xlsx.OpenFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			file = xlsx.NewFile()
		} else {
			return nil, err
		}
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
func (r *Report) AddCurrency(currency Currency) (*CurrencySheet, error) {
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

		cell := row.AddCell()
		cell.SetValue("Date")

		cell = row.AddCell()
		cell.SetValue("Open")

		cell = row.AddCell()
		cell.SetValue("High")

		cell = row.AddCell()
		cell.SetValue("Low")

		cell = row.AddCell()
		cell.SetValue("Close")

		cell = row.AddCell()
		cell.SetValue("Volume")

		cell = row.AddCell()
		cell.SetValue("Market Cap")

		cell = row.AddCell()
		cell.SetValue("Average")
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
func (s *CurrencySheet) AddData(data HistoricPriceData, average float64) error {
	defer s.Save()

	row := s.sheet.AddRow()

	cell := row.AddCell()
	cell.SetValue(data.date.Format(dateFormat))

	cell = row.AddCell()
	cell.SetValue(data.open)

	cell = row.AddCell()
	cell.SetValue(data.high)

	cell = row.AddCell()
	cell.SetValue(data.low)

	cell = row.AddCell()
	cell.SetValue(data.close)

	cell = row.AddCell()
	cell.SetValue(data.volume)

	cell = row.AddCell()
	cell.SetValue(data.marketCap)

	cell = row.AddCell()
	cell.SetValue(average)

	return nil
}
