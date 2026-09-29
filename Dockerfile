# Etapa 1: Builder
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev git

WORKDIR /app

# Cache de dependências do Go
COPY go.mod go.sum ./
RUN go mod download

# Copia todo o código-fonte (templates, internal, etc.)
COPY . .

# Compila o binário estático/otimizado com CGO ativado para SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o doc-signer .

# Etapa 2: Runner de produção enxuto e seguro para rodar 24/7
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata sqlite-libs wget
ENV TZ=America/Sao_Paulo

WORKDIR /app

COPY --from=builder /app/doc-signer /app/doc-signer

RUN mkdir -p /app/data

EXPOSE 8080

# Healthcheck nativo para monitoramento 24/7 e auto-restart pelo Docker
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/livez > /dev/null || exit 1

CMD ["/app/doc-signer"]