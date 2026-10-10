.PHONY: build run vet fmt clean

build:
	go build -o build/exht-proxy ./cmd/exht-proxy

run:
	go run ./cmd/exht-proxy

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal

clean:
	rm -rf build tmp
