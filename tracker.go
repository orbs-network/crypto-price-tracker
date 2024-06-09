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

const Version = "1.0.1"

// AverageDays is the number of days to take, when calculating the average price.
const AverageDays = 14

const configFileName = "config.json"
const dateFormat = "2006-01-02"
const priorityDateFormat = "02/01/06"
const bankOfIsraelDateFormat = "2006-01-02"

const defaultStartFromDays = 31
const extraQueryDays = AverageDays + 1
const cmcQueryURL = "https://api.coinmarketcap.com/data-api/v3/cryptocurrency/historical"
const cmcKey = "6c12ec7c-37c1-407f-80c9-c9402253034c"

// const israeliBankURL = "https://www.boi.org.il/currency.xml"
const israeliBankURL = "https://edge.boi.gov.il/FusionEdgeServer/sdmx/v2/data/dataflow/BOI.STATISTICS/EXR/1.0/RER_USD_ILS"

var priorityEndpoint string
var priorityUsername string
var priorityPassword string
var daysBackToFetch int64

type OBS struct {
	XMLName   xml.Name `xml:"Obs"`
	OBS_VALUE float64  `xml:"OBS_VALUE,attr"`
}
type Series struct {
	XMLName xml.Name `xml:"Series"`
	OBS     OBS      `xml:"Obs"`
}
type MDS struct {
	XMLName xml.Name `xml:"DataSet"`
	Series  Series   `xml:"Series"`
}
type SSD struct {
	XMLName xml.Name `message:"StructureSpecificData"`
	MDS     MDS      `message:"DataSet"`
}

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
		currQuote := aQuote["quote"].(map[string]interface{})
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
		dataElement.marketCap = int64(currQuote["marketCap"].(float64))

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

	//https: //edge.boi.gov.il/FusionEdgeServer/sdmx/v2/data/dataflow/BOI.STATISTICS/EXR/1.0/RER_USD_ILS?startperiod=2008-01-01&endperiod=2008-01-02

	formattedDate := date.Format(bankOfIsraelDateFormat)

	israelBankQuery.Set("startperiod", formattedDate)
	israelBankQuery.Set("endperiod", formattedDate)

	israelBankQueryURL.RawQuery = israelBankQuery.Encode()

	client := &http.Client{}
	s := israelBankQueryURL.String()
	req, _ := http.NewRequest("GET", s, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/103.0.0.0 Safari/537.36")
	res, _ := client.Do(req)

	if err != nil {
		return 0, err
	}

	if res == nil || res.StatusCode != 200 {

		return getFromIsraelBank(
			date.AddDate(0, 0, -1),
			retries-1,
		)

	}

	byteValue, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}

	var p SSD

	xml.Unmarshal(byteValue, &p)

	currency := p.MDS.Series.OBS

	return currency.OBS_VALUE, nil

}

func getPriceData(currency *Currency) ([]*HistoricPriceData, error) {

	endDate := time.Now().Unix()
	startDate := endDate - (60 * 60 * 24 * (daysBackToFetch + 1))

	url := fmt.Sprintf("%s?id=%s&convertId=2781&timeStart=%d&timeEnd=%d&interval=daily&limit=1000",
		cmcQueryURL,
		currency.CMCId,
		startDate,
		endDate)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
	req.Header.Set("accept-language", "en-US,en;q=0.9,he;q=0.8")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="105", "Not)A;Brand";v="8", "Chromium";v="105"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "none")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("upgrade-insecure-requests", "1")
	req.Header.Set("cookie", "_hjSessionUser_1060636=eyJpZCI6IjM0ZmQwYWZlLWM4ZTAtNTQ4Yy04OTgxLWJhYWQ0ZWRlMTk3YSIsImNyZWF0ZWQiOjE2NTMxNTIyNDQ3OTUsImV4aXN0aW5nIjpmYWxzZX0=; _ga=GA1.2.589864098.1653152245; _gcl_au=1.1.769449889.1664740502; _gid=GA1.2.141148982.1664740502; _tt_enable_cookie=1; _ttp=d60c58f5-5d31-48e9-b62d-a7b9aed1916a; _fbp=fb.1.1664742139841.150092240; sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%22180e78d616cc14-016c169b911da-34736704-1484784-180e78d616df66%22%2C%22first_id%22%3A%22%22%2C%22props%22%3A%7B%22%24latest_traffic_source_type%22%3A%22%E8%87%AA%E7%84%B6%E6%90%9C%E7%B4%A2%E6%B5%81%E9%87%8F%22%2C%22%24latest_search_keyword%22%3A%22%E6%9C%AA%E5%8F%96%E5%88%B0%E5%80%BC%22%2C%22%24latest_referrer%22%3A%22https%3A%2F%2Fwww.google.com%2F%22%7D%2C%22%24device_id%22%3A%22180e78d616cc14-016c169b911da-34736704-1484784-180e78d616df66%22%2C%22identities%22%3A%22eyIkaWRlbnRpdHlfYW5vbnltb3VzX2lkIjoiMTgwZTc4ZDYxNmNjMTQtMDE2YzE2OWI5MTFkYS0zNDczNjcwNC0xNDg0Nzg0LTE4MGU3OGQ2MTZkZjY2IiwiJGlkZW50aXR5X2Nvb2tpZV9pZCI6IjE4MzlhNWQwZTkxMTg5LTBhNzExN2NmMWUyMTRhOC0xYTUyNTYzNS0xNDg0Nzg0LTE4MzlhNWQwZTkyMWNhOSJ9%22%2C%22history_login_id%22%3A%7B%22name%22%3A%22%22%2C%22value%22%3A%22%22%7D%7D")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	var p map[string]interface{}
	err = json.Unmarshal(body, &p)
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

	authorization := base64.StdEncoding.EncodeToString([]byte(usernameAndPassword))

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

		m, ok := res["FORM"].(map[string]interface{})

		if !ok {
			fmt.Println(res)
		} else {

			fmt.Println(m["InterfaceErrors"].(map[string]interface{})["text"])

		}

	} else {

		fmt.Println(fmt.Sprintf("Priority insert of Currency %s -> successful!", currency.Name))

	}

}

func processCurrency(report *Report, currency *Currency, report_forDelta *Report) error {

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
		cli.IntFlag{
			Name:  "daysBackToFetch",
			Value: 15,
			Usage: "Number of days to fetch from CMC, default is 15. must be above 15 for moving average calculations",
		},
	}

	app.Action = func(c *cli.Context) error {

		priorityUsername = c.String("priorityUsername")
		priorityPassword = c.String("priorityPassword")
		priorityEndpoint = c.String("priorityEndpoint")
		daysBackToFetch = c.Int64("daysBackToFetch")

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
