.PHONY: build login-build install clean tidy cross-build test

BINARY := dmr-plugin-weixin
LOGIN_BINARY := dmr-weixin-login
INSTALL_DIR := $(HOME)/.dmr/plugins

build: tidy
	go build -o $(BINARY) .

login-build: tidy
	go build -o $(LOGIN_BINARY) ./cmd/dmr-weixin-login

test:
	go test ./...

tidy:
	go mod tidy

cross-build: tidy
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY)-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINARY)-darwin-arm64 .

install: build login-build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/
	cp $(LOGIN_BINARY) $(INSTALL_DIR)/
	@echo "Installed to $(INSTALL_DIR)/$(BINARY) and $(INSTALL_DIR)/$(LOGIN_BINARY)"

clean:
	rm -f $(BINARY) $(LOGIN_BINARY) $(BINARY)-linux-amd64 $(BINARY)-linux-arm64 $(BINARY)-darwin-amd64 $(BINARY)-darwin-arm64
