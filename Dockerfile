# stage: build ---------------------------------------------------------

FROM golang:1.22-alpine as build

RUN apk add --no-cache gcc musl-dev linux-headers

WORKDIR /go/src/github.com/flashbots/bproxy

COPY go.* ./
RUN go mod download

COPY . .

ARG SOURCE_DATE_EPOCH=0
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -buildid=" \
    -o bin/bproxy \
    github.com/flashbots/bproxy/cmd

# stage: run -----------------------------------------------------------

FROM alpine

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /go/src/github.com/flashbots/bproxy/bin/bproxy ./bproxy

ENTRYPOINT ["/app/bproxy"]
