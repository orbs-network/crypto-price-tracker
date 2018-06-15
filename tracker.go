package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/urfave/cli"
)

const dateFormat = "2006-01-02"
const daysAgo = 15

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

		startTime := targetTime.Add(-daysAgo * 24 * time.Hour)

		fmt.Println("Getting price information from:", startTime.Format(dateFormat))

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
