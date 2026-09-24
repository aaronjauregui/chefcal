BINARY  := chefcal
GO      := go
GOFLAGS :=

.PHONY: all build test test-cover vet fmt fmt-check lint clean docker run

all: build

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) .

test:
	$(GO) test -race ./...

test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w .

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "Unformatted files:"; echo "$$out"; exit 1; fi

lint: vet fmt-check
	golangci-lint run ./...

clean:
	rm -f $(BINARY) coverage.out

docker:
	docker build -t $(BINARY) .

run: build
	./$(BINARY) -config config.yaml
