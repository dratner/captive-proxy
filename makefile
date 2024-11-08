.PHONY: install build run test clean

# Name of the executable
EXECUTABLE := captive-proxy

# Function to check for root privileges
define check_root
	@if [ "$$(id -u)" != "0" ]; then \
		echo "This operation requires root privileges. Please run with sudo."; \
		exit 1; \
	fi
endef

install:
	$(call check_root)
	@echo "Checking for libpcap-dev..."
	@if ! dpkg -s libpcap-dev >/dev/null 2>&1; then \
		echo "libpcap-dev not found. Installing..."; \
		apt-get update && apt-get install -y libpcap-dev; \
	else \
		echo "libpcap-dev is already installed."; \
	fi

build: test
	@echo "Building $(EXECUTABLE)..."
	@go fmt
	@go build -o $(EXECUTABLE)

test:
	@go mod tidy
	@echo "Running tests..."
	@go test -v ./...

run: build
	$(call check_root)
	@echo "Running $(EXECUTABLE) with root privileges..."
	./$(EXECUTABLE)

clean:
	@echo "Cleaning up..."
	@rm -f $(EXECUTABLE)
