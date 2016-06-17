os := $(shell uname | tr [:upper:] [:lower:])
bin := go-imgrect-$(os)
src := $(shell find . -type f -name '*.go')
assets := $(shell find asset/assets -type f)

.PHONY: build clean run deps 

build: dist/$(bin)

asset/asset.go: $(assets)
	go-bindata -pkg asset -nocompress -o asset/asset.go -prefix asset asset/assets/

deps:
	go get github.com/wieni/go-tls/simplehttp
	go get github.com/lazywei/go-opencv
	go get github.com/golang/freetype
	go get github.com/jteeuwen/go-bindata/...

dist/$(bin): $(src) asset/asset.go | dist
	go build -o "dist/$(bin)"

dist:
	mkdir dist

clean:
	rm -rf dist
	rm asset/asset.go

run: asset/asset.go
	go run *.go

