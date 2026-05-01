BIN := bin/detour

.PHONY: build test run clean

build:
	go build -o $(BIN) ./cmd/detour

test:
	go test ./...

run: build
	./$(BIN)

clean:
	rm -rf bin/
