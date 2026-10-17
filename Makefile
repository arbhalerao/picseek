BIN := bin
TAGS := sqlite_fts5

.PHONY: gen build run lint fmt sync clean eval

gen:
	buf generate

build:
	go build -tags $(TAGS) -o $(BIN)/ ./cmd/...

run: build
	./scripts/dev.sh

sync:
	uv sync --all-packages

lint:
	buf lint
	golangci-lint run --build-tags $(TAGS) ./...
	uv run ruff check .
	uv run ruff format --check .

fmt:
	golangci-lint fmt ./...
	uv run ruff format .

eval: build
	uv run picseek-eval

clean:
	rm -rf $(BIN)
