FROM golang:1.24.3 AS builder
WORKDIR /workspace
COPY go.mod go.sum /workspace/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -a -o manager main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER nobody
ENTRYPOINT ["/manager"]
