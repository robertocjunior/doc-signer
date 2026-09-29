# Doc Signer

Serviço corporativo de confirmação e assinatura eletrônica de leitura de documentos integrado ao **Microsoft Entra ID (Azure AD)**, banco de dados SQLite com WAL e disparo de recibos via SMTP Office 365.

---

## 🏗️ Arquitetura e Estrutura de Diretórios

O projeto segue as convenções modernas do ecossistema Go (Standard Go Project Layout):

```text
doc-signer/
├── cmd/
│   └── server/
│       └── main.go          # Ponto de entrada secundário
├── internal/
│   ├── app/
│   │   └── app.go           # Inicialização, graceful shutdown e ciclo de vida
│   ├── auth/
│   │   ├── oauth.go         # Integração OAuth2 (Microsoft Entra ID) e Graph API
│   │   └── oauth_test.go    # Testes unitários de autenticação
│   ├── config/
│   │   ├── config.go        # Leitura, parsing e validação de variáveis de ambiente
│   │   └── config_test.go   # Testes de configuração e fallbacks
│   ├── database/
│   │   ├── sqlite.go        # Conexão SQLite (WAL, Busy Timeout, Concorrência)
│   │   ├── repository.go    # Camada de acesso a dados (CRUD, consultas e filtros)
│   │   ├── maintenance.go   # Rotinas de auto-reparo e manutenção 24/7
│   │   └── repository_test.go
│   ├── email/
│   │   ├── sender.go        # Cliente SMTP (AUTH LOGIN, STARTTLS, retentativas exponenciais)
│   │   └── sender_test.go   # Testes unitários do formatador de e-mail
│   ├── handlers/
│   │   ├── router.go        # Roteamento e registro de middlewares
│   │   ├── middleware.go    # Middleware de recuperação de pânico, logs e headers
│   │   ├── embed.go         # Endpoint /assinar (renderiza iframe)
│   │   ├── auth.go          # Endpoints /auth/login e /auth/callback
│   │   ├── sign.go          # Endpoint /confirmar (processa assinatura e hash)
│   │   ├── admin.go         # Painel administrativo /admin com filtros
│   │   ├── export.go        # Exportações /admin/export (CSV, XML, XLSX)
│   │   ├── health.go        # Endpoints /healthz e /livez
│   │   └── handlers_test.go # Testes de integração de endpoints
│   ├── models/
│   │   ├── models.go        # Estruturas de dados (SignatureRecord, UserProfile, etc.)
│   │   └── models_test.go
│   └── service/
│       ├── signer.go        # Regras de negócio, geração de hash SHA-256 e idempotência
│       └── signer_test.go   # Testes unitários do serviço de assinatura
├── templates/
│   ├── embed.html           # Template visual do iframe de confirmação
│   └── admin.html           # Template visual do painel de administração
├── Dockerfile               # Build multi-stage otimizado com HEALTHCHECK nativo
├── docker-compose.yml       # Orquestração com política de restart e volumes persistentes
├── .env.example             # Modelo seguro de variáveis de ambiente
├── .gitignore               # Proteção contra commit de credenciais (.env) e dados
├── .dockerignore            # Exclusão de arquivos sensíveis no build da imagem
└── main.go                  # Ponto de entrada principal com templates embutidos
```

---

## 🛡️ Confiabilidade e Operação 24/7 (Auto-Reparo)

O sistema foi preparado para rodar ininterruptamente sem intervenção manual:

1. **Auto-Reparo e Manutenção de Banco (SQLite)**:
   - **Modo WAL (Write-Ahead Logging)** com `_synchronous=NORMAL` para máxima performance e concorrência sem bloqueio de leituras.
   - **Busy Timeout (5000ms)**: Previne o erro `database is locked` enfileirando requisições concorrentes.
   - **Worker Periódico em Segundo Plano**:
     - Executa `PRAGMA quick_check` e `PRAGMA integrity_check` para validar a saúde do banco.
     - Executa `PRAGMA wal_checkpoint(PASSIVE)` para liberar páginas do log WAL de volta para o arquivo principal, evitando crescimento descontrolado do disco.
     - Executa `PRAGMA optimize` para atualizar as estatísticas do planejador de consultas.
2. **Panic Recovery Middleware**:
   - Qualquer pânico inesperado em handlers HTTP é capturado imediatamente. A stack trace completa é registrada no log com `slog.Error` e o cliente recebe HTTP 500 amigável sem que o servidor caia.
3. **Resiliência no Envio de E-mail (SMTP Office 365)**:
   - Suporte a retentativas com backoff exponencial configurável (`SMTP_MAX_RETRIES` e `SMTP_RETRY_DELAY`) em caso de instabilidades temporárias de rede.
   - Execução em background desacoplada da resposta HTTP do usuário.
4. **Graceful Shutdown**:
   - Intercepta `SIGTERM` e `SIGINT`.
   - Conclui as requisições em trânsito com timeout configurável (`SHUTDOWN_TIMEOUT`).
   - Realiza o checkpoint final do banco antes de finalizar.
5. **Observabilidade e Healthchecks**:
   - `GET /healthz`: Verifica integridade do SQLite e conectividade, retornando status detalhado e uptime em JSON.
   - `GET /livez`: Verificação leve de liveness para o Docker e orquestradores.
   - **Logs Estruturados**: Utiliza `log/slog` (nativo do Go) suportando saídas em texto legível ou JSON estruturado (`LOG_FORMAT=json`).

---

## 🚀 Como Executar

### 1. Pré-requisitos
Copie o arquivo `.env.example` para `.env` e configure as credenciais:
```bash
cp .env.example .env
```

### 2. Rodando Localmente
```bash
go run .
```

### 3. Rodando com Docker Compose
```bash
docker compose up -d --build
```

---

## 🧪 Executando os Testes

Para executar toda a suíte de testes com validação de corrida de concorrência (`-race`):
```bash
go test -v -race ./...
```
