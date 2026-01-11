.PHONY: build start clean run json add list all watch remove

# Build the binary
build:
	go build -o quota-ag .

# Run the tool (alias for start)
start: build
	./quota-ag --all --watch 1m

# Run with JSON output
json: build
	./quota-ag --json

# Add a new account
add: build
	./quota-ag --add

# List all accounts
list: build
	./quota-ag --list

# Remove an account
remove: build
	./quota-ag --remove

# Show quota for all accounts
all: build
	./quota-ag --all

# Clean build artifacts
clean:
	rm -f quota-ag
