.PHONY: all
all: check test coverage check-licenses build

# Programs to build, each in its own directory under cmd/, e.g.
# cmd/example/main.go. Add more programs space-separated.
CMDS = example
BINARIES = $(addprefix out/,$(CMDS))

.PHONY: build
build: $(BINARIES)

# Binaries are built as packages, not as main.go files, so that they carry the
# version Go embeds from the git tag and commit (see internal/buildinfo.go).
# They are .PHONY because that version is not a file prerequisite: a binary
# built before a commit or a tag would be kept, still naming the old one. The
# build cache makes the rebuild cheap.
.PHONY: $(BINARIES)
$(BINARIES): out/%:
	go build -o $@ ./cmd/$*

.PHONY: check
check: tidy-check vet lint

# Fails when go.mod or go.sum is not tidy, without changing them. `make tidy`
# tidies them.
.PHONY: tidy-check
tidy-check:
	go mod tidy -diff

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: vet
vet:
	go vet ./...

# The golangci-lint version CI runs. Keep equal to the version in
# .github/workflows/golangci-lint.yml.
GOLANGCI_LINT_VERSION = 2.15.0

.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint version 2>/dev/null | grep -q " $(GOLANGCI_LINT_VERSION) " || \
			echo "note: golangci-lint is not $(GOLANGCI_LINT_VERSION), the version CI runs"; \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed; skipping (CI enforces it)"; \
	fi

# Scans for known vulnerabilities that the code reaches, as the govulncheck
# workflow does. Not part of `check` or `all`: the vulnerability database
# changes without a commit, so this can fail on code nobody touched.
.PHONY: vulncheck
vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# pre-commit runs from a Python virtual environment. It is .venv, not venv,
# because the go tool ignores directories whose names start with a dot:
# pre-commit ships a Go template file, so with a venv/ inside the module,
# ./... would include a package from it.
PRE_COMMIT_VERSION = 4.7.0

.venv/bin/pre-commit:
	python3 -m venv .venv
	.venv/bin/pip install pre-commit==$(PRE_COMMIT_VERSION)

.PHONY: pre-commit-install
pre-commit-install: .venv/bin/pre-commit
	.venv/bin/pre-commit install

.PHONY: pre-commit
pre-commit: .venv/bin/pre-commit
	.venv/bin/pre-commit run --all-files

# Spell check of code, comments and docs, with the settings in .codespellrc
# that the pre-commit hook also reads. Keep the version equal to the hook rev.
CODESPELL_VERSION = 2.4.3

.venv/bin/codespell:
	python3 -m venv .venv
	.venv/bin/pip install codespell==$(CODESPELL_VERSION)

.PHONY: codespell
codespell: .venv/bin/codespell
	.venv/bin/codespell

.PHONY: test
test:
	go test ./...

.PHONY: coverage
coverage:
	go test -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out -o coverage.txt
	@tail -1 coverage.txt

.PHONY: clean
clean:
	rm -rf out coverage.out coverage.html coverage.txt

# go install builds the packages too, so the installed programs report the
# version, and it installs to $GOBIN, or else the bin directory of `go env GOPATH`.
.PHONY: install
install:
	go install $(addprefix ./cmd/,$(CMDS))

.PHONY: update
update:
	go get -t -u ./...

# wwhrd reads the licenses of the packages in a vendor directory, and without one it checks nothing and passes.
# So the Go files, go.mod, go.sum and .wwhrd.yml are copied to a temporary directory, the dependencies are
# vendored there, and wwhrd checks that copy, which leaves no vendor directory in the repository.
.PHONY: check-licenses
check-licenses:
	@if ! command -v wwhrd >/dev/null 2>&1; then \
		echo "wwhrd not installed; skipping license check (install: go install github.com/frapposelli/wwhrd@latest)"; \
	else \
		tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
		git ls-files -z -c -o --exclude-standard '*.go' go.mod go.sum .wwhrd.yml | xargs -0 tar cf - | tar xf - -C "$$tmp" && \
		cd "$$tmp" && go mod vendor && wwhrd -q check; \
	fi
