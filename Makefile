MODEL_DIR := $(or $(gte_small_directory),$(CURDIR)/gte-small)

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

setup_benchmark:
	@echo "creating the tester venv and installing its requirements..."
	python3 -m venv benchmark/.venv
	benchmark/.venv/bin/pip install -r benchmark/requirements.txt
	@echo ""
	@echo "downloading thenlper/gte-small into $(MODEL_DIR)..."
	benchmark/.venv/bin/hf download thenlper/gte-small config.json vocab.txt tokenizer.json model.safetensors --local-dir "$(MODEL_DIR)"
	@echo ""
	@echo "linking the model into benchmark/gte-small..."
	@test -e benchmark/gte-small || ln -s "$(MODEL_DIR)" benchmark/gte-small
	@echo ""
	@echo "caching the python reference model..."
	cd benchmark && .venv/bin/python tester.py
	@echo ""

run_benchmark:
	@test -e benchmark/.venv -a -e benchmark/gte-small || { echo "run make setup_benchmark first"; exit 1; }
	@echo "generating reference vectors..."
	cd benchmark && HF_HUB_OFFLINE=1 .venv/bin/python tester.py
	@echo ""
	@echo "running benchmark..."
	cd benchmark && go run .
	@echo ""
