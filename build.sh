#!/bin/bash -e

go get github.com/urfave/cli
go get github.com/RobinUS2/golang-moving-average
go get github.com/kardianos/osext
go get github.com/tealeg/xlsx

go build -ldflags="-s -w" -o bin/crypto-price-tracker

echo 'cd -- "$(dirname -- "$BASH_SOURCE")"' > ./bin/run_priority_test.sh
echo './crypto-price-tracker --priorityEndpoint="https://p.priority-connect.online/odata/Priority/tabp00bl.ini/queent" --priorityPassword=OrbsAcc --priorityUsername=master' >> ./bin/run_priority_test.sh
chmod a+x ./bin/crypto-price-tracker
chmod a+x ./bin/run_priority_test.sh


