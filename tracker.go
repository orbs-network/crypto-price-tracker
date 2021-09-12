package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	movingaverage "github.com/RobinUS2/golang-moving-average"
	"github.com/kardianos/osext"
	"github.com/urfave/cli"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"
)

const Version = "1.0.0"

// AverageDays is the number of days to take, when calculating the average price.
const AverageDays = 14

const configFileName = "config.json"
const dateFormat = "2006-01-02"
const priorityDateFormat = "02/01/06"
const bankOfIsraelDateFormat = "20060102"

const defaultStartFromDays = 31
const extraQueryDays = AverageDays + 1
const cmcQueryURL = "https://web-api.coinmarketcap.com/v1/cryptocurrency/ohlcv/historical"
const israeliBankURL = "https://www.boi.org.il/currency.xml"

var priorityEndpoint string
var priorityUsername string
var priorityPassword string
var daysBackToFetch string

func priorityLoadCurrencyApiEndpoint(currencySign string) string {

	return fmt.Sprintf(
		"%s/CURRENCIES('%s')/LOADCURRENCY_SUBFORM",
		priorityEndpoint,
		currencySign,
	)

}

var shekelUsdRatioCache = make(map[string]float64)

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
	priceData      *HistoricPriceData
	average        float64
	shekelUsdRatio float64
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

func getShekelConversionRatio(date time.Time) (shekelDollarRatio float64, err error) {

	formattedDate := date.Format(bankOfIsraelDateFormat)

	shekelUsdRatio, exists := shekelUsdRatioCache[formattedDate]

	if !exists {

		shekelUsdRatio, err = getFromIsraelBank(date, 20)
		if err != nil {
			return 0, err
		}

		shekelUsdRatioCache[formattedDate] = shekelUsdRatio

	}

	return shekelUsdRatio, nil

}

func getFromIsraelBank(date time.Time, retries int) (float64, error) {

	if retries < 1 {
		panic("Cannot fetch Shekel USD values from BankOfIsrael")
	}

	israelBankQueryURL, err := url.Parse(israeliBankURL)

	if err != nil {
		return 0, err
	}

	israelBankQuery := israelBankQueryURL.Query()

	israelBankQuery.Set("curr", "01")
	israelBankQuery.Set("rdate", date.Format(bankOfIsraelDateFormat))

	israelBankQueryURL.RawQuery = israelBankQuery.Encode()

	res, err := http.Get(israelBankQueryURL.String())
	if err != nil {
		return 0, err
	}

	if res.StatusCode != 200 {
		return 0, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	byteValue, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}

	var p struct {
		CURRENCY struct {
			RATE float64
			UNIT uint
		}
	}

	xml.Unmarshal(byteValue, &p)

	currency := p.CURRENCY

	if currency.UNIT > 1 {
		return 0, fmt.Errorf("getShekelConversionRatio: Wrong returned unit number %v", currency.UNIT)
	}

	if currency.RATE == 0 {

		return getFromIsraelBank(
			date.AddDate(0, 0, -1),
			retries-1,
		)

	}

	return currency.RATE, nil

}

func getPriceData(currency *Currency) ([]*HistoricPriceData, error) {

	queryURL, err := url.Parse(cmcQueryURL)

	if err != nil {
		return nil, err
	}

	query := queryURL.Query()
	query.Set("id", currency.CMCId)
	query.Set("convert", "USD")
	query.Set("count", daysBackToFetch)
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

func writePriceData(report *Report, currency *Currency, data []*HistoricPriceData, report_forDelta *Report) error {

	sheet, err := report.AddCurrency(currency)

	sheet_forDelta, err := report_forDelta.AddCurrency(currency)

	if err != nil {
		return err
	}

	var average float64
	ma := movingaverage.New(AverageDays)

	var fullPriceData []FullHistoricPriceData

	for i, j := len(data)-1, 0; i >= 0; i-- {
		e := data[i]

		shekelUsdRatio, err := getShekelConversionRatio(e.date)
		if err != nil {
			return err
		}

		fmt.Println("Processing:", currency.Name, e.date.Format(dateFormat), "-",
			"open:", e.open, "USD",
			"high:", e.high, "USD",
			"low:", e.low, "USD",
			"close:", e.close, "USD",
			"volume:", e.volume,
			"market cap:", e.marketCap,
			"shekelUsdRatio:", shekelUsdRatio,
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

		fullPriceData = append(fullPriceData, FullHistoricPriceData{priceData: e, average: average, shekelUsdRatio: shekelUsdRatio})
	}

	for i := len(fullPriceData) - 1; i >= 0; i-- {

		priceData := &fullPriceData[i]

		exist := false

		// check if exist
		for _, row := range sheet.sheet.Rows {

			if row.Cells[0].Value == priceData.priceData.date.Format(dateFormat) {
				exist = true
				break
			}

		}

		if !exist {

			if len(priorityEndpoint) != 0 {

				dailyAverage := (priceData.priceData.open + priceData.priceData.close) / 2

				performImportToPriority(
					currency,
					priceData.shekelUsdRatio*dailyAverage,
					priceData.priceData.date,
				)

			}

			err := sheet.AddData(priceData)

			if err != nil {
				return err
			}

			err = sheet_forDelta.AddData(priceData)

			if err != nil {
				return err
			}

		} else {

			// todo: verify new row matches existing row
			fmt.Println(fmt.Sprintf("skipping existing row for date: %s", priceData.priceData.date))

		}

	}

	fmt.Println()

	return nil
}

func performImportToPriority(currency *Currency, exchangeRate float64, currencyDate time.Time) {

	fmt.Println(fmt.Sprintf(
		"Inserting to Priority {%s, %v, %s}...",
		currency.Name,
		exchangeRate,
		currencyDate,
	))

	data := map[string]interface{}{
		"EXCHANGE": exchangeRate,
		"CURDATE":  currencyDate,
	}

	json_data, err := json.Marshal(data)

	if err != nil {
		log.Fatal(err)
	}

	client := &http.Client{}

	req, _ := http.NewRequest(
		"POST",
		priorityLoadCurrencyApiEndpoint(currency.Symbol),
		bytes.NewBuffer(json_data),
	)

	usernameAndPassword := fmt.Sprintf(
		"%s:%s",
		priorityUsername,
		priorityPassword,
	)

	authorization := base64.StdEncoding.EncodeToString([]byte(usernameAndPassword, ))

	req.Header.Set(
		"Authorization",
		fmt.Sprintf("Basic %s", authorization),
	)

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)

	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode != 201 {

		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)

		fmt.Print(fmt.Sprintf(
			"Priority insert ERROR {%v}: ", resp.StatusCode))

		fmt.Println(res["FORM"].(map[string]interface{})["InterfaceErrors"].(map[string]interface{})["text"])

	} else {

		fmt.Println(fmt.Sprintf("Priority insert of Currency %s -> successful!", currency.Name))

	}

}

func processCurrency(report *Report, currency *Currency, report_forDelta *Report, ) error {

	fmt.Println("Processing:", currency.Name)
	fmt.Println("days count:", daysBackToFetch)

	fmt.Println()

	data, err := getPriceData(currency)

	if err != nil {
		return err
	}

	err = writePriceData(report, currency, data, report_forDelta)
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
			Name:  "priorityUsername",
			Value: "",
			Usage: "The username for priority API client",
		},
		cli.StringFlag{
			Name:  "priorityPassword",
			Value: "",
			Usage: "The password for priority API client",
		},
		cli.StringFlag{
			Name:  "priorityEndpoint",
			Value: "",
			Usage: "If set, we will try to export currencies into priority, this should be set to test or prod env endpoint uri",
		},
		cli.StringFlag{
			Name:  "daysBackToFetch",
			Value: "15",
			Usage: "Number of days to fetch from CMC, default is 15. must be above 15 for moving average calculations",
		},
	}

	app.Action = func(c *cli.Context) error {

		priorityUsername = c.String("priorityUsername")
		priorityPassword = c.String("priorityPassword")
		priorityEndpoint = c.String("priorityEndpoint")
		daysBackToFetch = c.String("daysBackToFetch")

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

		report, err := OpenReport("Crypto-HistoricalPrice.xlsx")

		if err != nil {
			return err
		}

		name, err := generateReportForDeltaFileName()

		if err != nil {
			return err
		}

		report_forDelta, err := OpenReport(
			name,
		)

		if err != nil {
			return err
		}

		for _, currency := range currencies.Currencies {
			err := processCurrency(report, &currency, report_forDelta)
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
