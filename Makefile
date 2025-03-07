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

test-integration: build
	@echo "Running integration tests for validation command..."
	@echo "Validating bash template..."
	./bin/$(BINARY_NAME) validate $(TEST_TEMPLATE_DIR)/mock-setup.sh --ci-data 'hostname:test,username:admin,ssh_key:testkey,packages:nginx,app_type:web,environment:test'
	@echo "Validating yaml template..."
	./bin/$(BINARY_NAME) validate $(TEST_TEMPLATE_DIR)/mock-setup.yaml --ci-data 'hostname:test,username:admin,ssh_key:testkey,extra_packages:curl,server_name:test.com,environment:test'
	@echo "Testing for expected failures (should produce errors)..."
	@(./bin/$(BINARY_NAME) validate $(TEST_TEMPLATE_DIR)/mock-setup.sh --ci-data 'hostname:test' || echo "Test passed: Expected failure due to missing variables") && \
	(./bin/$(BINARY_NAME) validate $(TEST_TEMPLATE_DIR)/mock-setup.sh --ci-data 'hostname:test,username:admin,ssh_key:testkey,packages:nginx,app_type:web,environment:test,extra:value' || echo "Test passed: Expected failure due to unused variables")

test: test-unit test-integration
	@echo "All tests completed"
