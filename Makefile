BIN := bin/detour

.PHONY: build test run clean snapshot release-check

build:
	go build -o $(BIN) ./cmd/detour

test:
	go test ./...

run: build
	./$(BIN)

clean:
	rm -rf bin/ dist/

snapshot:
	goreleaser build --snapshot --clean

release-check:
	goreleaser check
