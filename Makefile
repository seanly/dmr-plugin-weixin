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
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY)-windows-amd64.exe .
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINARY)-windows-arm64.exe .
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(LOGIN_BINARY)-linux-amd64 ./cmd/dmr-weixin-login
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(LOGIN_BINARY)-linux-arm64 ./cmd/dmr-weixin-login
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o $(LOGIN_BINARY)-darwin-amd64 ./cmd/dmr-weixin-login
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o $(LOGIN_BINARY)-darwin-arm64 ./cmd/dmr-weixin-login
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o $(LOGIN_BINARY)-windows-amd64.exe ./cmd/dmr-weixin-login
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o $(LOGIN_BINARY)-windows-arm64.exe ./cmd/dmr-weixin-login

install: build login-build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/
	cp $(LOGIN_BINARY) $(INSTALL_DIR)/
	@echo "Installed to $(INSTALL_DIR)/$(BINARY) and $(INSTALL_DIR)/$(LOGIN_BINARY)"

clean:
	rm -f $(BINARY) $(LOGIN_BINARY) \
		$(BINARY)-linux-amd64 $(BINARY)-linux-arm64 $(BINARY)-darwin-amd64 $(BINARY)-darwin-arm64 $(BINARY)-windows-amd64.exe $(BINARY)-windows-arm64.exe \
		$(LOGIN_BINARY)-linux-amd64 $(LOGIN_BINARY)-linux-arm64 $(LOGIN_BINARY)-darwin-amd64 $(LOGIN_BINARY)-darwin-arm64 $(LOGIN_BINARY)-windows-amd64.exe $(LOGIN_BINARY)-windows-arm64.exe
