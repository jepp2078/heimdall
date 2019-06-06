export GO111MODULE=on
export GOOS:=$(shell go env GOOS)
export GOARCH:=$(shell go env GOARCH)
export BINNAME:=jepp2078/heimdall

all:
	(cd cli && make)
	(cd injector && make)
	(cd keys && make)
	(cd secrets && make)

run:
	(cd cli && make run)
	(cd injector && make run)
	(cd keys && make run)
	(cd secrets && make run)

test:
	(cd cli && make test)
	(cd injector && make test)
	(cd keys && make test)
	(cd secrets && make test)

build-docker:
	(cd cli && make build-docker)
	(cd injector && make build-docker)
	(cd keys && make build-docker)
	(cd secrets && make build-docker)

build:
	(cd cli && make build)
	(cd injector && make build)
	(cd keys && make build)
	(cd secrets && make build)

clean:
	(cd cli && make clean)
	(cd injector && make clean)
	(cd keys && make clean)
	(cd secrets && make clean)

generate:
	(cd api && protoc -I ./ heimdall-keys.proto --go_out=plugins=grpc:../generated)