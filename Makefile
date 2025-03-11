BINARY_NAME := vhicmd
SRC := ./...
CGO_ENABLED := 0
TEST_TEMPLATE_DIR := tests

BUILD_TIME := $(shell date -u +"%Y%m%d-%H%M%S")
LDFLAGS := -s -w -X github.com/jessegalley/vhicmd/cmd.buildTime=${BUILD_TIME}
BUILD_FLAGS := -ldflags="$(LDFLAGS)"
GODOC := $(HOME)/go/bin/godoc

.PHONY: all build clean docs

all: build

build:
	CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_FLAGS) -o bin/$(BINARY_NAME) main.go

clean:
	rm -f bin/$(BINARY_NAME)
	rm -f api/doc.go
	rm -rf docs

docs:
	@echo "Generating HTML documentation for api package..."
	@mkdir -p docs
	@go run tools/docgen/main.go
	@echo "Documentation written to docs/api.html"

test-unit:
	@echo "Running unit tests..."
	go test -v ./internal/template

test: test-unit
	@echo "All tests completed"
