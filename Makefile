.PHONY: check build clean format format-check frontend frontend-check observability-check

VERSION ?= dev
REVISION ?= unknown

FORMAT_PYTHON = uv run --no-project --with-requirements requirements-format.txt

check:
	@test -z "$$(gofmt -l server)" || (gofmt -l server; exit 1)
	go test ./...
	go vet ./...

frontend:
	npm --prefix web ci
	npm --prefix web run build

frontend-check:
	npm --prefix web ci
	npm --prefix web run check
	npm --prefix web run build
	npm --prefix web test

build: frontend
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$(VERSION) -X main.revision=$(REVISION)" -o bin/linha-server ./server/cmd/linha-server

format:
	npm --prefix web run format
	gofmt -w server
	mvn -B spotless:apply
	$(FORMAT_PYTHON) python scripts/format-data.py
	$(FORMAT_PYTHON) black scripts
	$(FORMAT_PYTHON) sqlfluff format server/internal/postgres/*.sql

format-check:
	npm --prefix web run check
	@test -z "$$(gofmt -l server)" || (gofmt -l server; exit 1)
	mvn -B spotless:check
	$(FORMAT_PYTHON) python scripts/format-data.py --check
	$(FORMAT_PYTHON) black --check scripts
	$(FORMAT_PYTHON) sqlfluff lint --rules layout server/internal/postgres/*.sql

clean:
	rm -f bin/linha-server

observability-check:
	$(FORMAT_PYTHON) python scripts/check-observability.py
