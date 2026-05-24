.PHONY: build login-build install install-all install-policy clean tidy cross-build test bump-dmr help

BINARY := dmr-plugin-weixin
LOGIN_BINARY := dmr-weixin-login
BINDIR := bin
CMD := ./cmd/dmr-plugin-weixin
LOGIN_CMD := ./cmd/dmr-weixin-login
INSTALL_DIR := $(HOME)/.dmr/plugins
POLICY_DIR := $(HOME)/.dmr/etc/policies

# Sibling checkout for bump-dmr; override: make bump-dmr DMR_DIR=/path/to/dmr
DMR_DIR ?= ../dmr

help:
	@echo "Targets:"
	@echo "  build          - tidy + compile to bin/$(BINARY)"
	@echo "  login-build    - tidy + compile helper to bin/$(LOGIN_BINARY)"
	@echo "  install        - plugin + login helper binaries -> $(INSTALL_DIR)/"
	@echo "  install-policy - copy policies/weixin.rego -> $(POLICY_DIR)/ (chmod 700 dir, 600 file for DMR opa_policy reload)"
	@echo "  install-all    - install + install-policy"
	@echo "  clean tidy cross-build test bump-dmr"
	@echo ""
	@echo "  bump-dmr       - pin go.mod github.com/seanly/dmr to DMR_DIR HEAD ($(DMR_DIR))"

build: tidy
	@mkdir -p $(BINDIR)
	go build -o $(BINDIR)/$(BINARY) $(CMD)

login-build: tidy
	@mkdir -p $(BINDIR)
	go build -o $(BINDIR)/$(LOGIN_BINARY) $(LOGIN_CMD)

cross-build: tidy
	@mkdir -p $(BINDIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINDIR)/$(BINARY)-linux-amd64 $(CMD)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINDIR)/$(BINARY)-linux-arm64 $(CMD)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINDIR)/$(BINARY)-darwin-amd64 $(CMD)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINDIR)/$(BINARY)-darwin-arm64 $(CMD)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINDIR)/$(BINARY)-windows-amd64.exe $(CMD)
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINDIR)/$(BINARY)-windows-arm64.exe $(CMD)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINDIR)/$(LOGIN_BINARY)-linux-amd64 $(LOGIN_CMD)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINDIR)/$(LOGIN_BINARY)-linux-arm64 $(LOGIN_CMD)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINDIR)/$(LOGIN_BINARY)-darwin-amd64 $(LOGIN_CMD)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINDIR)/$(LOGIN_BINARY)-darwin-arm64 $(LOGIN_CMD)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINDIR)/$(LOGIN_BINARY)-windows-amd64.exe $(LOGIN_CMD)
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o $(BINDIR)/$(LOGIN_BINARY)-windows-arm64.exe $(LOGIN_CMD)

tidy:
	go mod tidy

bump-dmr:
	@test -d "$(DMR_DIR)/.git" || (echo "dmr repo not found at $(DMR_DIR); set DMR_DIR=..." && exit 1)
	@echo "=> github.com/seanly/dmr $$(git -C '$(DMR_DIR)' rev-parse --short HEAD)"
	GOPRIVATE=github.com/seanly/dmr GONOSUMDB=github.com/seanly/dmr GOPROXY=direct go get github.com/seanly/dmr@$$(git -C '$(DMR_DIR)' rev-parse HEAD)
	go mod tidy
	@grep 'github.com/seanly/dmr' go.mod || true

install: build login-build
	mkdir -p $(INSTALL_DIR)
	cp $(BINDIR)/$(BINARY) $(INSTALL_DIR)/
	cp $(BINDIR)/$(LOGIN_BINARY) $(INSTALL_DIR)/

install-all: install install-policy

install-policy:
	mkdir -p $(POLICY_DIR)
	chmod 700 $(POLICY_DIR)
	cp policies/weixin.rego $(POLICY_DIR)/
	chmod 600 $(POLICY_DIR)/weixin.rego

clean:
	rm -rf $(BINDIR)

test:
	go test ./...
