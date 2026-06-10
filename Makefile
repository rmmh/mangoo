ESBUILD := ./frontend/node_modules/.bin/esbuild
ESBUILD_FLAGS := --bundle --jsx=automatic --jsx-import-source=preact \
                 --loader:.tsx=tsx --loader:.ts=ts --target=es2020 \
                 --sourcemap

.PHONY: build frontend-build dev clean deps-check

build: frontend-build
	CGO_ENABLED=1 go build -o mangoo .

frontend-build: npm-install esbuild

npm-install:
	cd frontend && npm install --silent

esbuild:
	$(ESBUILD) $(ESBUILD_FLAGS) \
		frontend/src/main.tsx --outfile=frontend/dist/app.js

dev:
	$(ESBUILD) $(ESBUILD_FLAGS) --sourcemap \
		frontend/src/main.tsx --outfile=frontend/dist/app.js --watch &
	CGO_ENABLED=1 go run . --config mangoo.toml

deps-check:
	@pkg-config --exists libwebp || \
		(echo "ERROR: libwebp-dev not installed. Run: sudo apt install libwebp-dev"; exit 1)
	@echo "All system dependencies OK"

clean:
	rm -f mangoo frontend/dist/app.js
