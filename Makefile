BINARY := gooxi-exporter
SUBNET ?= 192.168.0.0/24

.PHONY: all deps tidy build fmt vet test cover run discovery clean help

all: fmt vet build ## Format, vet, and build

deps: ## Download module dependencies
	go mod download

tidy: ## Add missing / remove unused module requirements
	go mod tidy

build: ## Build the exporter binary
	CGO_ENABLED=0 go build -trimpath -o $(BINARY) .

fmt: ## Format Go source files
	gofmt -w .

vet: ## Run static analysis
	go vet ./...

test: ## Run all tests (with race detector)
	go test -race ./...

cover: ## Run tests with a coverage report
	go test -coverprofile=.coverage ./...

run: ## Run the exporter (e.g. go run . --config.file=config.example.yml)
	go run .

discovery: ## Discover Gooxi BMCs in a subnet (override: SUBNET=10.0.1.0/24)
	./scripts/discover.sh "$(SUBNET)"

clean: ## Remove build artifacts
	rm -f $(BINARY) .coverage

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "%-8s %s\n", $$1, $$2}'
