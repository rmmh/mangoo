ESBUILD := ./frontend/node_modules/.bin/esbuild

VERSION := v$(shell git rev-list --count HEAD)-$(shell git rev-parse --short HEAD)$(shell test -n "$$(git status --porcelain -uno)" && echo -dirty)

ESBUILD_FLAGS := --bundle --jsx=automatic --jsx-import-source=preact \
                 --loader:.tsx=tsx --loader:.ts=ts --target=es2020 \
                 --sourcemap --define:__APP_VERSION__='"$(VERSION)"'

.PHONY: build frontend-build clean deps-check

build: frontend-build
	CGO_ENABLED=1 go build -o mangoo .

frontend-build: npm-install esbuild

npm-install:
	cd frontend && npm install --silent

esbuild:
	$(ESBUILD) $(ESBUILD_FLAGS) \
		frontend/src/main.tsx --outfile=frontend/dist/app.js

deps-check:
	@pkg-config --exists libwebp || \
		(echo "ERROR: libwebp-dev not installed. Run: sudo apt install libwebp-dev"; exit 1)
	@echo "All system dependencies OK"

clean:
	rm -f mangoo frontend/dist/app.js
