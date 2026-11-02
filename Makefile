license:
	curl -sL https://liam.sh/-/gh/g/license-header.sh | bash -s

docs-prepare:
	cd docs && bun install

docs-debug: docs-prepare
	cd docs && bun run dev

docs-build: docs-prepare
	cd docs && bun run build

docs-preview: docs-build
	cd docs && bun run preview

# Records each example's tour to docs/public/demos/<example>.webp, for the docs pages. Needs ffmpeg.
demos:
	cd _examples/ && STEEP_EXPORT_MODE=on go test -count 1 ./...

fetch:
	go mod tidy

up:
	go get -u ./... && go mod tidy
	cd _examples/ && go get -u ./... && go mod tidy
	cd _integration/ && go get -u ./... && go mod tidy
	cd docs && bunx npm-check-updates -u && bun install

test:
	GORACE="exitcode=1 halt_on_error=1" go test -v -race -timeout 3m -count 3 -cpu 1,4 ./...
	cd _examples/ && GORACE="exitcode=1 halt_on_error=1" go test -v -race -timeout 3m -count 3 -cpu 1,4 ./...
	cd _integration/ && GORACE="exitcode=1 halt_on_error=1" go test -v -race -timeout 3m -count 3 -cpu 1,4 ./...
