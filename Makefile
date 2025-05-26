BINARY_NAME := vhicmd
SRC := ./...
CGO_ENABLED := 0

BUILD_TIME := $(shell date -u +"%Y%m%d-%H%M%S")
LDFLAGS := -s -w -X github.com/jessegalley/vhicmd/cmd.buildTime=${BUILD_TIME}
BUILD_FLAGS := -ldflags="$(LDFLAGS)"

.PHONY: all build clean

all: build

build:
	CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_FLAGS) -o bin/$(BINARY_NAME) main.go

clean:
	rm -f bin/$(BINARY_NAME)
	rm -f api/doc.go
	rm -rf docs
