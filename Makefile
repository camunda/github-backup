BINARY_NAME=ghbackup

run:
	go run main.go

build:
	go build -o $(BINARY_NAME) .

test:
	go test . -v

clean:
	rm -rf ./repositories
	rm -rf 20*
	rm $(BINARY_NAME)

