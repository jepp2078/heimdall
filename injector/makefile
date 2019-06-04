export GO111MODULE=on
export GOOS:=$(shell go env GOOS)
export GOARCH:=$(shell go env GOARCH)
export BINNAME:=jepp2078/heimdall-injector

all: build run

run:
	./bin/${BINNAME} --kubeconfig=/home/${USER}/.kube/config --git-credentials=kube-system/heimdall-git-credentials --configmap-cleanup=true

test:
	go test ./...

build-docker:
	docker run -it -e GOOS=${GOOS} -e GOARCH=${GOARCH} -v $(shell pwd):/${BINNAME} -w /${BINNAME} golang:1.12 make

build:
	go build -mod vendor -o bin/${BINNAME} .

clean:
	sudo rm bin/${BINNAME}