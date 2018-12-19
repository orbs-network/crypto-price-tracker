#!/bin/bash -e

go get github.com/urfave/cli
go get github.com/PuerkitoBio/goquery
go get github.com/RobinUS2/golang-moving-average
go get github.com/kardianos/osext
go get github.com/tealeg/xlsx

go build -o bin/crypto-price-tracker
