# Etapa 1: Builder
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev git

WORKDIR /app

# Copia os arquivos de código
COPY . .

# Atualiza e baixa as dependências exatas gerando o go.sum correto
RUN go mod tidy && go mod download

# Compila com CGO ativado para SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o doc-signer .

# Etapa 2: Runner final enxuto
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata sqlite-libs
ENV TZ=America/Sao_Paulo

WORKDIR /app

COPY --from=builder /app/doc-signer /app/doc-signer

RUN mkdir -p /app/data

EXPOSE 8080

CMD ["/app/doc-signer"]