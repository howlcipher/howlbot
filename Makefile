.PHONY: all build test run

all: build

build:
	go build -o bin/howlbot main.go

test:
	go test ./...

run: build
	. .env && ./bin/howlbot
