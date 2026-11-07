build:
	CGO_ENABLED=0 go build -o mem ./main.go

lint:
	@golangci-lint run

tidy:
	@goimports -w .
	go mod tidy

test:
	go test -v -count=1 ./...
