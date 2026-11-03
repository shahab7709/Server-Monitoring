FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGET=api
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath -o /app ./cmd/${TARGET}
FROM alpine:3.21
RUN adduser -D app
USER app
COPY --from=build /app /app
ENTRYPOINT ["/app"]
