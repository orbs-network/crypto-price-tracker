package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/RobinUS2/golang-moving-average"
	"github.com/tealeg/xlsx"
	"github.com/urfave/cli"
)

const dateFormat = "2006-01-02"
const queryDateFormat = "20060102"
const lastDaysAverageDurationDays = 15
const cmcQueryURL = "https://coinmarketcap.com/currencies/%s/historical-data/"

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

		dataElement.marketCap, err = strconv.ParseInt(strings.Replace(nodes[6], ",", "", -1), 10, 64)
		if err != nil {
			log.Fatal(err)
		}

		data = append(data, dataElement)
	})

	return data
}

func getPriceData(startTime time.Time, endTime time.Time, currency string) ([]historicPriceData, error) {
	queryURL, err := url.Parse(fmt.Sprintf(cmcQueryURL, currency))
	if err != nil {
		return nil, err
	}

	query := queryURL.Query()
	query.Set("start", startTime.Format(queryDateFormat))
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

	if len(data) < lastDaysAverageDurationDays {
		return nil, fmt.Errorf("Not enough data point: %d", len(data))
	}

	return data, nil
}

func writePriceData(fileName string, currency string, data []historicPriceData) error {
	err := writeHeaders(fileName, currency)
	if err != nil {
		return err
	}

	var average float64
	ma := movingaverage.New(lastDaysAverageDurationDays)

	for i, j := len(data)-1, 0; i >= 0; i-- {
		e := data[i]
		fmt.Println("Processing", currency, e.date.Format(dateFormat), "-",
			"open:", e.open, "USD",
			"high:", e.high, "USD",
			"low:", e.low, "USD",
			"close:", e.close, "USD",
			"volume:", e.volume,
			"market cap:", e.marketCap,
		)

		// If we have more than lastDaysAverageDurationDays left, then it'd be possible to calculate the average.
		if j+1 > lastDaysAverageDurationDays {
			average = ma.Avg()
		} else {
			average = 0
		}

		j++
		ma.Add(e.close)

		err := writeData(fileName, currency, e, average)
		if err != nil {
			return err
		}
	}

	fmt.Println()

	return nil
}

func writeHeaders(fileName string, currency string) error {
	file := xlsx.NewFile()
	defer file.Save(fileName)

	sheet, err := file.AddSheet(currency)
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

func writeData(fileName string, currency string, data historicPriceData, average float64) error {
	file, err := xlsx.OpenFile(fileName)
	if err != nil {
		return err
	}

	defer file.Save(fileName)

	sheet := file.Sheet[currency]

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
		cli.StringFlag{
			Name:  "currency",
			Value: "bitcoin",
			Usage: `the name of the cryptocurrency to query"`,
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

		fmt.Println("Starting from:", startTime.Format(dateFormat))
		fmt.Println("Ending at:", endTime.Format(dateFormat))

		currency := c.String("currency")

		fmt.Println("Processing:", currency)
		fmt.Println()

		data, err := getPriceData(startTime, endTime, currency)
		if err != nil {
			return err
		}

		fileName := startTime.Format(dateFormat)
		if startTime != endTime {
			fileName += "_" + endTime.Format(dateFormat)
		}
		fileName += ".xlsx"

		err = writePriceData(fileName, currency, data)
		if err != nil {
			return err
		}

		fmt.Println("Finished...")

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
