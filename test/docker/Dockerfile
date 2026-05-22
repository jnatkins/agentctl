FROM golang:1.26-bookworm

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    openssh-client \
    openssh-server \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace/agentctl

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /usr/local/bin/agentctl ./cmd/agentctl

ENV GOCACHE=/tmp/go-cache
ENV GOMODCACHE=/go/pkg/mod

CMD ["agentctl", "--help"]
