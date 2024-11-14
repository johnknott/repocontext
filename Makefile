.PHONY: build run clean

build:
	go build -o bin/repocontext cmd/repocontext/main.go

run: build
	./bin/repocontext $(ARGS)

clean:
	rm -rf bin/
	go clean

install: build
	cp bin/repocontext $(GOPATH)/bin/
