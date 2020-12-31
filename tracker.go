package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	movingaverage "github.com/RobinUS2/golang-moving-average"
	"github.com/kardianos/osext"
	"github.com/urfave/cli"
)

// AverageDays is the number of days to take, when calculating the average price.
const AverageDays = 14

const configFileName = "config.json"
const dateFormat = "2006-01-02"
const queryDateFormat = "20060102"
const defaultStartFromDays = 31
const extraQueryDays = AverageDays + 1
const cmcQueryURL = "https://web-api.coinmarketcap.com/v1/cryptocurrency/ohlcv/historical"

// Currency is the specific cryptocurrency configuration in the config file.
type Currency struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
	CMC    string `json:"cmc"`
	CMCId  string `json:"cmc_id"`
}

// Currencies is the list of all currency configurations.
type Currencies struct {
	Currencies []Currency `json:"currencies"`
}

// HistoricPriceData is the cryptocurrency's historic price data at a specific point in time.
type HistoricPriceData struct {
	date      time.Time
	open      float64
	high      float64
	low       float64
	close     float64
	volume    float64
	marketCap int64
}

// FullHistoricPriceData includes HistoricPriceData as well as the last AverageDays const AverageDays = 14.
type FullHistoricPriceData struct {
	priceData *HistoricPriceData
	average   float64
}

func parseData(quotes []interface{}) []*HistoricPriceData {
	var data []*HistoricPriceData
	var err error
	const cmcDateFormat = "2006-01-02T15:04:05.000Z"

	for i := 0; i < len(quotes); i++ {
		aQuote := quotes[i].(map[string]interface{})
		p2 := aQuote["quote"].(map[string]interface{})
		currQuote := p2["USD"].(map[string]interface{})
		var dataElement HistoricPriceData
		dataElement.date, err = time.Parse(cmcDateFormat, currQuote["timestamp"].(string))
		if err != nil {
			log.Fatal(err)
		}
		dataElement.open = currQuote["open"].(float64)
		dataElement.high = currQuote["high"].(float64)
		dataElement.low = currQuote["low"].(float64)
		dataElement.close = currQuote["close"].(float64)
		dataElement.volume = currQuote["volume"].(float64)
		dataElement.marketCap = int64(currQuote["market_cap"].(float64))

		data = append(data, &dataElement)
	}

	return data
}

func getPriceData(currency *Currency, startTime *time.Time, endTime *time.Time) ([]*HistoricPriceData, error) {
	queryURL, err := url.Parse(cmcQueryURL)
	if err != nil {
		return nil, err
	}

	query := queryURL.Query()
	query.Set("id", currency.CMCId)
	query.Set("convert", "USD")
	query.Set("time_start", fmt.Sprintf("%d", startTime.Add(-extraQueryDays*24*time.Hour).Unix()))
	query.Set("time_end", fmt.Sprintf("%d", endTime.Unix()))
	queryURL.RawQuery = query.Encode()

	//Request the HTML page.
	res, err := http.Get(queryURL.String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	byteValue, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var p map[string]interface{}
	err = json.Unmarshal(byteValue, &p)
	if err != nil {
		return nil, err
	}

	p2 := p["data"].(map[string]interface{})
	quotes := p2["quotes"].([]interface{})

	data := parseData(quotes)

	if len(data) < AverageDays {
		return nil, fmt.Errorf("Not enough data points: %d", len(data))
	}

	return data, nil
}

func writePriceData(report *Report, currency *Currency, data []*HistoricPriceData) error {
	sheet, err := report.AddCurrency(currency)
	if err != nil {
		return err
	}

	var average float64
	ma := movingaverage.New(AverageDays)

	var fullPriceData []FullHistoricPriceData

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

		if e.close > 0 {
			ma.Add(e.close)
			j++
		}

		// If we have more than averageDays left, then it'd be possible to calculate the average.
		if j > AverageDays {
			average = ma.Avg()
		} else {
			average = 0
		}

		fullPriceData = append(fullPriceData, FullHistoricPriceData{priceData: e, average: average})
	}

	for i := len(fullPriceData) - 1; i >= 0; i-- {
		err := sheet.AddData(&fullPriceData[i])
		if err != nil {
			return err
		}
	}

	fmt.Println()

	return nil
}

func processCurrency(report *Report, currency *Currency, startTime *time.Time, endTime *time.Time) error {
	fmt.Println("Processing:", currency.Name)
	fmt.Println("Starting from:", startTime.Format(dateFormat))
	fmt.Println("Ending at:", endTime.Format(dateFormat))

	fmt.Println()

	data, err := getPriceData(currency, startTime, endTime)
	if err != nil {
		return err
	}

	err = writePriceData(report, currency, data)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	app := cli.NewApp()
	app.Name = "Cryptocurrencies Price Tracker"
	app.Usage = fmt.Sprintf("track the last %d days cryptocurrency's average price using CMC historic data web page scraper", AverageDays)
	app.Version = "0.9.0"

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

		var startTime time.Time
		rawStartTime := c.String("start")
		if rawStartTime == "now" {
			startTime = endTime.Add(-defaultStartFromDays * 24 * time.Hour)
		} else {
			var err error
			startTime, err = time.Parse(dateFormat, rawStartTime)
			if err != nil {
				return err
			}
		}

		folderPath, err := osext.ExecutableFolder()
		if err != nil {
			return err
		}

		jsonFile, err := os.Open(path.Join(folderPath, configFileName))
		if err != nil {
			jsonFile, err = os.Open(path.Join(configFileName))
			if err != nil {
				return err
			}
		}
		defer jsonFile.Close()

		byteValue, _ := ioutil.ReadAll(jsonFile)

		var currencies Currencies
		err = json.Unmarshal(byteValue, &currencies)
		if err != nil {
			return err
		}

		report, err := NewReport(&startTime, &endTime)
		if err != nil {
			return err
		}

		for _, currency := range currencies.Currencies {
			err := processCurrency(report, &currency, &startTime, &endTime)
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
