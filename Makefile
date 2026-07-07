BIN := sub-subscribe

.PHONY: build test clean

build:
	CGO_ENABLED=0 go build -o $(BIN) -ldflags="-s -w" .

test:
	go vet ./...
	go test ./...

clean:
	rm -f $(BIN)
