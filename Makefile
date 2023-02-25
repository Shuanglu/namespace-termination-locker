.PHONY: build
build:
		@echo "\nBuilding Go binaries..."
		GOOS=linux GOARCH=amd64 go build -ldflags "-w" -o output/namespace-termination-locker .

.PHONY: manifests

manifests:
	@echo "\nGenerates manifests..."
	scripts/gen-certs.sh