os:=$(shell uname | tr [:upper:] [:lower:])
bin := go-imgrect-$(os)
src := $(shell find . -type f -name '*.go')

.PHONY: build clean run deps

build: dist/$(bin)

deps:
	go get github.com/wieni/go-tls/simplehttp
	go get github.com/lazywei/go-opencv/opencv

dist/$(bin): $(src) | dist
	go build -o "dist/$(bin)"

dist/$(bin)_$(os): $(src) | dist
		go build -o "dist/$(bin)"

dist:
	mkdir dist

clean:
	rm -rf dist

run:
	go run *.go

