FROM debian:bookworm-slim

ARG DEBIAN_FRONTEND=noninteractive
ENV GO_VERSION=1.26.0

RUN apt-get update && apt-get install -y --no-install-recommends \
    git wget ca-certificates postgresql-client && \
    rm -rf /var/lib/apt/lists/*

RUN wget -q https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -O /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz
ENV GOROOT="/usr/local/go"
ENV PATH="${GOROOT}/bin:${PATH}"

# Public repo — no access token required.
WORKDIR /ignis
RUN git clone https://github.com/THD-Spatial-AI/ignis.git .

RUN go mod tidy \
    && go build -o bin/app      cmd/app/main.go \
    && go build -o bin/build_db cmd/build_db/main.go

EXPOSE 8090

ENTRYPOINT ["/ignis/bin/app"]
