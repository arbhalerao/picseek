BIN := bin

.PHONY: gen build run lint fmt sync clean

gen:
	buf generate

build:
	go build -o $(BIN)/ ./cmd/...

run: build
	$(BIN)/seekd

sync:
	uv sync --all-packages

lint:
	buf lint
	golangci-lint run ./...
	uv run ruff check .
	uv run ruff format --check .

fmt:
	golangci-lint fmt ./...
	uv run ruff format .

clean:
	rm -rf $(BIN)
