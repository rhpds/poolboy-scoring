BINARY   := poolboy-scoring
CMD      := ./cmd
COVER    := coverage.out

.PHONY: build run test cover lint clean

build:
	go build -o $(BINARY) $(CMD)

run:
	go run $(CMD)

test:
	go test ./... -v -count=1

cover:
	go test ./... -v -count=1 -coverprofile=$(COVER)
	go tool cover -func=$(COVER)

lint:
	go vet ./...

clean:
	rm -f $(BINARY) $(COVER)
