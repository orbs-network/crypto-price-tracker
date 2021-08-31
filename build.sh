#!/bin/bash -e

go get github.com/urfave/cli
go get github.com/RobinUS2/golang-moving-average
go get github.com/kardianos/osext
go get github.com/tealeg/xlsx

go build -ldflags="-s -w" -o bin/crypto-price-tracker

echo './crypto-price-tracker --priorityUsername david --priorityPassword 1234 --priorityEndpoint "https://sandbox.priority-software.com/sbx5/odata/Priority/tabp3a1a.ini/cockpit"' > ./bin/run_priority_text.sh
chmod +x ./bin/run_priority_text.sh


