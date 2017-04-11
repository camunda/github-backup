VERSION=0.1
BINARY_NAME=ghbackup

run:
	go run main.go

build:
	mkdir -p ./bin/
	go build -o $(BINARY_NAME) .
	mv $(BINARY_NAME) ./bin/

release:
	mkdir -p ./bin/ ./release/
	go build -o ./bin/$(BINARY_NAME) .
	tar czf ./release/$(BINARY_NAME)-$(VERSION).tar.gz ./bin/$(BACKUP_NAME)
	#mv $(BINARY_NAME) ./bin/
	#mv *tar.gz ./release/

test:
	go test . -v


clean:
	rm -rf ./bin/ ./release/ *.tar.gz ./repositories 20* $(BINARY_NAME)
