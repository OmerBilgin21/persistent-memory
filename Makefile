build:
	@echo "building..."
	CGO_ENABLED=0 go build -o mem ./main.go
	@echo ""

check: build
	@echo "formatting..."
	@golangci-lint fmt
	@echo ""
	@echo "linting..."
	@golangci-lint run || true
	@echo ""
	@echo "mod tidy..."
	@go mod tidy
	@echo ""
	@echo "running tests..."
	@go test -v -count=1 ./...
