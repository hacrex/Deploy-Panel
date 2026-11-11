VERSION ?= 0.0.0-dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test vet fmt-check schema-check api-check proto-lint proto docker ci clean

# dpd needs CGO (mattn/go-sqlite3). dp-agent and dpctl do not.
build:
	CGO_ENABLED=1 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/dpd ./cmd/dpd
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/dp-agent ./cmd/dp-agent
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/dpctl ./cmd/dpctl

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "needs gofmt:"; echo "$$out"; exit 1; fi

# Applies every migration in order to in-memory SQLite and lints for portability.
schema-check:
	python3 scripts/schema_check.py

# Checks api/openapi.yaml, including that Site enums match the schema. Needs PyYAML.
api-check:
	python3 scripts/api_check.py

proto-lint:
	buf lint
	buf format -d --exit-code

# Generates Go code into gen/. See buf.gen.yaml for the plugins to install.
proto:
	buf generate

docker:
	docker build -f deploy/docker/Dockerfile.dpd -t deploypanel/dpd:dev .
	docker build -f deploy/docker/Dockerfile.dp-agent -t deploypanel/dp-agent:dev .

ci: fmt-check vet test schema-check api-check

clean:
	rm -rf bin
