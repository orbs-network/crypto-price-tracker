package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tealeg/xlsx"

	"github.com/PuerkitoBio/goquery"
	"github.com/RobinUS2/golang-moving-average"
	"github.com/urfave/cli"
)

const configFileName = "config.json"
const dateFormat = "2006-01-02"
const queryDateFormat = "20060102"
const averageDays = 15
const extraQueryDays = averageDays + 1
const cmcQueryURL = "https://coinmarketcap.com/currencies/%s/historical-data/"

type Currency struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
	Cmc    string `json:"cmc"`
}

type Currencies struct {
	Currencies []Currency `json:"currencies"`
}

type historicPriceData struct {
	date      time.Time
	open      float64
	high      float64
	low       float64
	close     float64
	volume    int64
	marketCap int64
}

func parseData(doc *goquery.Document) []historicPriceData {
	var data []historicPriceData
	const selector = "#historical-data .table tbody tr"
	const td = "td"
	const cmcDateFormat = "Jan 2, 2006"

	// Find the historical data items.
	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		var err error
		var dataElement historicPriceData

		// For each item found, parse and get all the data
		nodes := s.Find(td).Map(func(_ int, e *goquery.Selection) string {
			return e.Text()
		})

		dataElement.date, err = time.Parse(cmcDateFormat, nodes[0])
		if err != nil {
			log.Fatal(err)
		}

		dataElement.open, err = strconv.ParseFloat(nodes[1], 64)
		if err != nil {
			log.Fatal(err)
		}

		dataElement.high, err = strconv.ParseFloat(nodes[2], 64)
		if err != nil {
			log.Fatal(err)
		}

		dataElement.low, err = strconv.ParseFloat(nodes[3], 64)
		if err != nil {
			log.Fatal(err)
		}

		dataElement.close, err = strconv.ParseFloat(nodes[4], 64)
		if err != nil {
			log.Fatal(err)
		}

		dataElement.volume, err = strconv.ParseInt(strings.Replace(nodes[5], ",", "", -1), 10, 64)
		if err != nil {
			log.Fatal(err)
		}

		marketCap := strings.Replace(nodes[6], ",", "", -1)
		if marketCap == "-" {
			dataElement.marketCap = 0.0
		} else {
			dataElement.marketCap, err = strconv.ParseInt(strings.Replace(nodes[6], ",", "", -1), 10, 64)
			if err != nil {
				log.Fatal(err)
			}
		}

		data = append(data, dataElement)
	})

	return data
}

func getPriceData(currency Currency, startTime time.Time, endTime time.Time) ([]historicPriceData, error) {
	queryURL, err := url.Parse(fmt.Sprintf(cmcQueryURL, currency.Cmc))
	if err != nil {
		return nil, err
	}

	query := queryURL.Query()
	query.Set("start", startTime.Add(-extraQueryDays*24*time.Hour).Format(queryDateFormat))
	query.Set("end", endTime.Format(queryDateFormat))
	queryURL.RawQuery = query.Encode()

	// Request the HTML page.
	res, err := http.Get(queryURL.String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	data := parseData(doc)

	if len(data) < averageDays {
		return nil, fmt.Errorf("Not enough data points: %d", len(data))
	}

	return data, nil
}

func writePriceData(fileName string, currency Currency, data []historicPriceData) error {
	err := writeHeaders(fileName, currency)
	if err != nil {
		return err
	}

	var average float64
	ma := movingaverage.New(averageDays)

	for i, j := len(data)-1, 0; i >= 0; i-- {
		e := data[i]
		fmt.Println("Processing:", currency.Name, e.date.Format(dateFormat), "-",
			"open:", e.open, "USD",
			"high:", e.high, "USD",
			"low:", e.low, "USD",
			"close:", e.close, "USD",
			"volume:", e.volume,
			"market cap:", e.marketCap,
		)

		// If we have more than averageDays left, then it'd be possible to calculate the average.
		if j+1 > averageDays {
			average = ma.Avg()
		} else {
			average = 0
		}

		// Don't record extra days.
		if j+1 > extraQueryDays {
			err := writeData(fileName, currency, e, average)
			if err != nil {
				return err
			}
		}

		if e.close > 0 {
			ma.Add(e.close)
			j++
		}
	}

	fmt.Println()

	return nil
}

func writeHeaders(fileName string, currency Currency) error {
	file, err := xlsx.OpenFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			file = xlsx.NewFile()
		} else {
			return err
		}
	}
	defer file.Save(fileName)

	sheet, err := file.AddSheet(currency.Name)
	if err != nil {
		return err
	}

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

	return nil
}

func writeData(fileName string, currency Currency, data historicPriceData, average float64) error {
	file, err := xlsx.OpenFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			file = xlsx.NewFile()
		} else {
			return err
		}
	}
	defer file.Save(fileName)

	sheet := file.Sheet[currency.Name]
	if sheet == nil {
		sheet, err = file.AddSheet(currency.Name)
		if err != nil {
			return err
		}
	}

	row := sheet.AddRow()

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

func processCurrency(fileName string, currency Currency, startTime time.Time, endTime time.Time) error {
	fmt.Println("Processing:", currency.Name)
	fmt.Println("Starting from:", startTime.Format(dateFormat))
	fmt.Println("Ending at:", endTime.Format(dateFormat))

	fmt.Println()

	data, err := getPriceData(currency, startTime, endTime)
	if err != nil {
		return err
	}

	err = writePriceData(fileName, currency, data)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	app := cli.NewApp()
	app.Name = "Cryptocurrencies Price Tracker"
	app.Usage = "track the last 15 days cryptocurrency's average price using CMC historic data web page scraper"
	app.Version = "0.2.0"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "start",
			Value: "now",
			Usage: `the start of the price calculation period (e.g., "2018-06-15"`,
		},
		cli.StringFlag{
			Name:  "end",
			Value: "now",
			Usage: `the end of the price calculation period (e.g., "2018-06-20"`,
		},
	}

	app.Action = func(c *cli.Context) error {
		var startTime time.Time
		rawStartTime := c.String("start")
		if rawStartTime == "now" {
			startTime = time.Now().Local()
		} else {
			var err error
			startTime, err = time.Parse(dateFormat, rawStartTime)
			if err != nil {
				return err
			}
		}

		var endTime time.Time
		rawEndTime := c.String("end")
		if rawEndTime == "now" {
			endTime = time.Now().Local()
		} else {
			var err error
			endTime, err = time.Parse(dateFormat, rawEndTime)
			if err != nil {
				return err
			}
		}

		jsonFile, err := os.Open(configFileName)
		if err != nil {
			return err
		}
		defer jsonFile.Close()

		byteValue, _ := ioutil.ReadAll(jsonFile)

		var currencies Currencies
		err = json.Unmarshal(byteValue, &currencies)
		if err != nil {
			return err
		}

		fileName := startTime.Format(dateFormat)
		if startTime != endTime {
			fileName += "_" + endTime.Format(dateFormat)
		}
		fileName += ".xlsx"

		os.Remove(fileName)

		for _, currency := range currencies.Currencies {
			err := processCurrency(fileName, currency, startTime, endTime)
			if err != nil {
				return err
			}
		}

		fmt.Println("Finished...")

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
