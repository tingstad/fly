ARG GO_VERSION=1.23
FROM golang:${GO_VERSION}-alpine AS build

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /build

COPY go.* *.go ./
COPY templates/ ./templates/

RUN go generate ./... && go test ./... && go build -o main .

FROM alpine:3.18.2

WORKDIR /dist

COPY --from=build /build/main ./

ENTRYPOINT ["/dist/main"]

