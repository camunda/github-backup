VERSION=0.1.2
BINARY_NAME=ghbackup

build:
	mkdir -p ./bin/
	go build -o $(BINARY_NAME) .
	mv $(BINARY_NAME) ./bin/

run:
	go run main.go

release:
	mkdir -p bin/
	go build -o bin/$(BINARY_NAME) .
	cp config.yml bin/
	tar czf $(BINARY_NAME)-$(VERSION).tar.gz bin/
	mkdir -p release/ 
	mv *.tar.gz release/

test:
	go test . -v


clean:
	rm -rf ./bin/ ./release/ *.tar.gz ./repositories 20* $(BINARY_NAME)
