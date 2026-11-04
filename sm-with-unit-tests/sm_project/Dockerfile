FROM golang:1.23-alpine AS build

WORKDIR /src

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY

ENV GOPROXY=https://proxy.golang.org
# Fall back to the source repository if the Go proxy fails.
# ENV GOPROXY=https://proxy.golang.org|direct

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go mod tidy

ARG TARGET=api

RUN CGO_ENABLED=0 go build \
    -trimpath \
    -o /app \
    ./cmd/${TARGET}

FROM alpine:3.21

RUN adduser -D app

USER app

COPY --from=build /app /app

ENTRYPOINT ["/app"]