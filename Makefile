.PHONY: run build frontend test test-ui package clean

run: frontend
	go run ./cmd/listodo

frontend/node_modules/.package-lock.json: frontend/package-lock.json frontend/package.json
	npm --prefix frontend ci --no-audit --no-fund

frontend: frontend/node_modules/.package-lock.json
	npm --prefix frontend run build

build: frontend
	mkdir -p build
	go build -trimpath -o build/listodo ./cmd/listodo

test: frontend
	go test ./...

test-ui: frontend
	npm --prefix frontend test

package: build
	sh scripts/package-macos.sh

clean:
	rm -rf build
