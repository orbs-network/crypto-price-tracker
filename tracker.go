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
	"github.com/urfave/cli"
)

const dateFormat = "2006-01-02"
const queryDateFormat = "20060102"
const lastDaysAverageDurationDays = 15
const queryDurationDays = 30
const cmcQueryURL = "https://coinmarketcap.com/currencies/%s/historical-data/"

type priceData struct {
	date    time.Time
	volume  int64
	close   float64
	average float64
}

type historicData struct {
	date      time.Time
	open      float64
	high      float64
	low       float64
	close     float64
	volume    int64
	marketCap int64
}

func parseData(doc *goquery.Document) []historicData {
	var data []historicData
	const selector = "#historical-data .table tbody tr"
	const td = "td"
	const cmcDateFormat = "Jan 2, 2006"

	// Find the historical data items.
	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		var err error
		var dataElement historicData

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

func getPriceData(targetTime time.Time, currency string) priceData {
	startTime := targetTime.Add(-queryDurationDays * 24 * time.Hour)

	fmt.Println("Getting price information for:", currency)
	fmt.Println("Starting from:", startTime.Format(dateFormat))

	queryURL, err := url.Parse(fmt.Sprintf(cmcQueryURL, currency))
	if err != nil {
		log.Fatal(err)
	}

	query := queryURL.Query()
	query.Set("start", startTime.Format(queryDateFormat))
	query.Set("end", targetTime.Format(queryDateFormat))
	queryURL.RawQuery = query.Encode()

	// Request the HTML page.
	res, err := http.Get(queryURL.String())
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	data := parseData(doc)

	for _, e := range data {
		fmt.Println("Processing", e.date.Format(dateFormat), "-",
			"open:", e.open, "USD",
			"high:", e.high, "USD",
			"low:", e.low, "USD",
			"close:", e.close, "USD",
			"volume:", e.volume,
			"market cap:", e.marketCap,
		)
	}

	var fullData priceData
	fullData.date = data[0].date
	fullData.volume = data[0].volume
	fullData.close = data[0].close
	fullData.average = 0

	fmt.Println()

	for _, e := range data[0:lastDaysAverageDurationDays] {
		fmt.Println("Including", e.date.Format(dateFormat), "-",
			"close:", e.close, "USD",
			"to the price calculation",
		)

		fullData.average += e.close
	}

	fullData.average /= lastDaysAverageDurationDays

	return fullData
}

func main() {
	app := cli.NewApp()
	app.Name = "Cryptocurrencies Price Tracker"
	app.Usage = "track the last 15 days cryptocurrency's average price using CMC historic data web page scraper"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "date",
			Value: "now",
			Usage: `the price calculating date (e.g., "2018-06-15"`,
		},
		cli.StringFlag{
			Name:  "currency",
			Value: "bitcoin",
			Usage: `the name of the cryptocurrency to query"`,
		},
	}

	app.Action = func(c *cli.Context) error {
		var targetTime time.Time
		date := c.String("date")
		if date == "now" {
			targetTime = time.Now().Local()
		} else {
			var err error
			targetTime, err = time.Parse(dateFormat, date)
			if err != nil {
				return err
			}
		}

		fmt.Println("Target time is:", targetTime.Format(dateFormat))

		currency := c.String("currency")
		data := getPriceData(targetTime, currency)

		fmt.Println("****************************************")
		fmt.Println("Result for", currency, "at", data.date.Format(dateFormat), "-",
			"volume:", data.volume,
			"close:", data.close, "USD",
			lastDaysAverageDurationDays, "days average:", data.average, "USD",
		)

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
