ARG GO_VERSION=1.23
FROM golang:${GO_VERSION}-alpine AS build

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /build

COPY go.* *.go ./
COPY templates/ ./templates/

RUN go generate ./... && go test ./... && go build -o main .

FROM gcc:14.2.0 AS invaders
WORKDIR /build
COPY ninvaders/* ./
RUN make clean && make

FROM alpine:3.18.2

# For ninvaders (util-linux for `script` command):
RUN apk add --no-cache \
    ncurses \
    libc6-compat \
    util-linux \
    && ln -s /usr/lib/libncursesw.so.6 /usr/lib/libncurses.so.6 \
    && ln -s /usr/lib/libncursesw.so.6 /usr/lib/libtinfo.so.6

WORKDIR /dist

COPY --from=build /build/main ./
COPY --from=invaders /build/nInvaders ./ninvaders

COPY <<"EOF" run.sh
    /dist/main
EOF

ENTRYPOINT ["sh", "run.sh"]
