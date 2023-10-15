FROM golang:1.21.3 as builder
WORKDIR /workspace
COPY go.mod go.sum /workspace/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -a -o manager main.go

FROM alpine:3.18.2
RUN apk update && apk add --no-cache ca-certificates
WORKDIR /
COPY --from=builder /workspace/manager .
USER nobody
ENTRYPOINT ["/manager"]
