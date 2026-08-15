# syntax=docker/dockerfile:1
# Imagem única para os três serviços Go (telephony-gw | core-api | notifier).
# Build: docker build -f infra/docker/Dockerfile.go --build-arg SERVICE=core-api .
# Contexto: raiz do repositório. Sem HEALTHCHECK aqui — o healthcheck vive no
# docker-compose.yml (distroless não tem shell; o compose usa /healthprobe).

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION} AS build
ARG SERVICE
WORKDIR /src

# Dependency layer cache: go.mod/go.sum change far less often than code.
COPY go/go.mod go/go.sum ./
RUN go mod download

COPY go/ ./

# Static build (CGO off) so the binary runs on distroless/static.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/service ./cmd/${SERVICE}

# Tiny static HTTP probe: distroless has no shell/wget, so compose healthchecks
# exec this binary ("/healthprobe <url>"). Any response < 500 means healthy.
COPY <<'EOF' /probe/main.go
// Command healthprobe is a minimal HTTP health probe for distroless images.
package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	url := "http://localhost:8080/healthz"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		os.Exit(1)
	}
}
EOF
RUN cd /probe && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/healthprobe ./main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
COPY --from=build /out/healthprobe /healthprobe
USER nonroot:nonroot
ENTRYPOINT ["/service"]
