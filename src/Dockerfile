# Build the wireguard-operator binary
FROM golang:1.23 AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY private/ private/

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
	go build -v -o wireguard-operator main.go

# Result container
FROM scratch
WORKDIR /
COPY --from=builder /workspace/wireguard-operator .
USER 65532:65532
ENTRYPOINT ["/wireguard-operator"]
