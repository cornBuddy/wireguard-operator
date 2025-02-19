FROM golang:1.23 AS builder
ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH
ENV CGO_ENABLED=0
WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY private/ private/

RUN go build \
    -a -v \
    -gcflags=all="-l -B" \
    -ldflags="-w -s" \
    -o wireguard-operator main.go

FROM scratch
COPY --from=builder /workspace/wireguard-operator .
USER 65534:65534
ENTRYPOINT ["/wireguard-operator"]
