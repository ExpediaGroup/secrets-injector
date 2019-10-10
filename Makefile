INJECTOR_TAG?=secrets-injector
SECRET_TAG?=secrets-init
all: build docker-push helm-install
build: go-build docker-injector-build docker-secret-build
docker-injector-build:
	docker build -t $(INJECTOR_TAG) .
docker-secret-build:
	docker build -t $(SECRET_TAG) ./examples/secrets-init
go-build:
	docker run --rm -v $(shell pwd):/usr/src/secrets-injector --workdir /usr/src/secrets-injector -e CGO_ENABLED=0 -e GOPATH=/usr -e GOOS=linux -e GOARCH=amd64 -e GO111MODULE=on golang:1.12 go build -a -tags netgo -ldflags '-w' -o secrets-injector *.go
docker-push: 
	docker push $(INJECTOR_TAG) && docker push $(SECRET_TAG)
helm-install:
	helm upgrade --install --namespace default secrets helm/secrets-injector --set image.name=$(INJECTOR_TAG) --set secretImage.name=$(SECRET_TAG) --values helm/secrets-injector/overrides.yaml
