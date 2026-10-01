.PHONY: check build clean format format-check

FORMAT_PYTHON = uv run --no-project --with-requirements requirements-format.txt

check:
	@test -z "$$(gofmt -l server)" || (gofmt -l server; exit 1)
	go test ./...
	go vet ./...

build:
	mkdir -p bin
	go build -trimpath -o bin/linha-server ./server/cmd/linha-server

format:
	gofmt -w server
	mvn -B spotless:apply
	$(FORMAT_PYTHON) python scripts/format-data.py
	$(FORMAT_PYTHON) black scripts
	$(FORMAT_PYTHON) sqlfluff format server/internal/postgres/*.sql

format-check:
	@test -z "$$(gofmt -l server)" || (gofmt -l server; exit 1)
	mvn -B spotless:check
	$(FORMAT_PYTHON) python scripts/format-data.py --check
	$(FORMAT_PYTHON) black --check scripts
	$(FORMAT_PYTHON) sqlfluff lint --rules layout server/internal/postgres/*.sql

clean:
	rm -f bin/linha-server
