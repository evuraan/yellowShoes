#!/bin/bash 


mkdir ../bin

set -e 


GOOS=linux GOARCH=arm GOARM=7 go build  -o ../bin/yellowShoes-linux-armv7l 
GOOS=linux GOARCH=arm GOARM=6 go build  -o ../bin/yellowShoes-linux-armv6l 
GOOS=windows GOARCH=amd64 go build -o ../bin/yellowShoes-win-amd64.exe 
GOOS=windows GOARCH=386 go build -o ../bin/yellowShoes-win-386.exe 
GOOS=linux GOARCH=amd64 go build -o ../bin/yellowShoes-linux-amd64 
GOOS=linux GOARCH=386 go build -o ../bin/yellowShoes-linux-386 
