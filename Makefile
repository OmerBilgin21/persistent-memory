build:
	CGO_ENABLED=0 go build -o mem ./main.go

format:
	@golangci-lint fmt

lint:
	@golangci-lint fmt
	@golangci-lint run

tidy:
	go mod tidy

test:
	go test -v -count=1 ./...
