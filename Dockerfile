# syntax=docker/dockerfile:1

# ---- build stage -----------------------------------------------------------
FROM golang:1.26 AS build
WORKDIR /src

# Allow overriding the module proxy for restricted networks:
#   docker build --build-arg GOPROXY=direct .
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Build a fully static, cgo-free binary (modernc SQLite needs no cgo).
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/splitwise-quic .

# Pre-create the data dir so we can copy it with the right owner.
RUN mkdir -p /data

# ---- runtime stage ---------------------------------------------------------
# Distroless: no shell, no package manager, tiny attack surface. Runs as the
# non-root user (uid 65532) by default.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/splitwise-quic /usr/local/bin/splitwise-quic
COPY --from=build --chown=65532:65532 /data /data

# Persist the SQLite DB + uploaded receipts here.
VOLUME ["/data"]

# Same port serves TCP (HTTP/2 bootstrap) and UDP (HTTP/3 / QUIC).
EXPOSE 4433/tcp
EXPOSE 4433/udp

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/splitwise-quic"]
CMD ["-addr", ":4433", "-db", "/data/splitwise.db", "-uploads", "/data/uploads"]
