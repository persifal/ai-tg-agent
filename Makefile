BINARY_NAME=ai-tg-bot
DOCKER_IMAGE=ai-tg-bot-image

GOPATH=$(shell go env GOPATH)
COMMIT=$(shell git rev-parse --short HEAD)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

LDFLAGS=-ldflags "-X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}"

build:
	@echo "Building ${BINARY_NAME}..."
	go build ${LDFLAGS} -o ${BINARY_NAME} .

clean:
	@echo "Cleaning..."
	@rm -f ${BINARY_NAME}
	@go clean

docker-build:
	@echo "Building Docker image..."
	docker build -t ${DOCKER_IMAGE} .

docker-run:
	docker run ${DOCKER_IMAGE}

run: build
	./${BINARY_NAME}
