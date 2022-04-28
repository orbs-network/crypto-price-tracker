#!/bin/bash -e

go get github.com/urfave/cli
go get github.com/RobinUS2/golang-moving-average
go get github.com/kardianos/osext
go get github.com/tealeg/xlsx

go build -ldflags="-s -w" -o bin/crypto-price-tracker

echo 'cd -- "$(dirname -- "$BASH_SOURCE")"' > ./bin/run_priority_test.sh
echo './crypto-price-tracker --priorityUsername david --priorityPassword 1234 --priorityEndpoint "https://sandbox.priority-software.com/sbx5/odata/Priority/tabp3a1a.ini/cockpit"' >> ./bin/run_priority_test.sh
chmod a+x ./bin/crypto-price-tracker
chmod a+x ./bin/run_priority_test.sh


