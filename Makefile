BINARY  := chefcal
GO      := go
GOFLAGS :=

.PHONY: all build test test-cover lint vet fmt clean docker run

all: build

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) .

test:
	$(GO) test ./...

test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w .

lint: vet fmt

clean:
	rm -f $(BINARY) coverage.out

docker:
	docker build -t $(BINARY) .

run: build
	./$(BINARY) -config config.yaml
