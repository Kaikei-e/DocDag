BINARY := docdag
PKGS := ./...

.PHONY: build test vet fmt fmt-check check clean

build:
	go build -o $(BINARY) .

test:
	go test -race -shuffle=on -count=1 $(PKGS)

vet:
	go vet $(PKGS)

fmt:
	gofmt -w .

fmt-check:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then \
		echo "gofmt needed:"; echo "$$out"; exit 1; \
	fi

check: fmt-check vet test

clean:
	rm -f $(BINARY)
