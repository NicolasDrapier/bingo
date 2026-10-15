.PHONY: run test build clean

run: build
	./bin/bingo

test:
	go test ./...

build:
	go build -buildvcs=false -o bin/bingo ./cmd/server

clean:
	rm -rf bin
