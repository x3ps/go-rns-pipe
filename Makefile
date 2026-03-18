.PHONY: test test-root test-examples lint build build-tcp build-udp e2e e2e-tcp e2e-udp

test: test-root test-examples

test-root:
	go test ./...
	go test -race ./...

test-examples:
	cd examples/tcp && go test ./... && go test -race ./...
	cd examples/udp && go test ./... && go test -race ./...

lint:
	go vet ./...
	golangci-lint run
	cd examples/tcp && go vet ./... && golangci-lint run
	cd examples/udp && go vet ./... && golangci-lint run

build: build-tcp build-udp

build-tcp:
	cd examples/tcp && go build -o rns-tcp-iface .

build-udp:
	cd examples/udp && go build -o rns-udp-iface .

e2e: e2e-tcp e2e-udp

e2e-tcp:
	docker compose -f examples/tcp/e2e/e2e/docker-compose.yml up --build --abort-on-container-exit --exit-code-from test-runner; \
	EXIT=$$?; \
	docker compose -f examples/tcp/e2e/e2e/docker-compose.yml down --volumes --remove-orphans; \
	exit $$EXIT

e2e-udp:
	docker compose -f examples/udp/e2e/e2e/docker-compose.yml up --build --abort-on-container-exit --exit-code-from test-runner; \
	EXIT=$$?; \
	docker compose -f examples/udp/e2e/e2e/docker-compose.yml down --volumes --remove-orphans; \
	exit $$EXIT
