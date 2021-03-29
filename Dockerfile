# Build the manager binary
FROM golang:1.13 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY vault/ vault/

# Build
RUN CGO_ENABLED=0 GO111MODULE=on go build -a -o manager main.go

FROM alpine:3.13.0
RUN apk update && apk add --no-cache ca-certificates
WORKDIR /
COPY --from=builder /workspace/manager .
USER nobody

ENTRYPOINT ["/manager"]
