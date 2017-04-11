API_JSON=$(printf '{"tag_name": "v%s","target_commitish": "master","name": "v%s","body": "Release of version %s","draft": false,"prerelease": false}' $VERSION $VERSION $VERSION)
curl --data "$API_JSON" https://api.github.com/repos/:owner/:repository/releases?access_token=:access_token

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

