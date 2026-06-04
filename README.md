# OIDC-RADIUS Bridge

Serviço de integração que autentica requisições FreeRADIUS contra um provedor OpenID Connect (OIDC) como o Keycloak. Funciona como middleware entre a infraestrutura RADIUS legada e provedores de identidade modernos baseados em OAuth2/OIDC.

![SSO UEM](https://github.com/user-attachments/assets/785a5f04-102c-42ee-b78a-e618e50e1932)

---

## Índice

- [Visão Geral](#visão-geral)
- [Arquitetura](#arquitetura)
- [Fluxo de Autenticação](#fluxo-de-autenticação)
- [Estrutura do Projeto](#estrutura-do-projeto)
- [Componentes](#componentes)
- [Configuração](#configuração)
- [Instalação](#instalação)
- [Integração com FreeRADIUS](#integração-com-freeradius)
- [API Reference](#api-reference)
- [Logging](#logging)
- [Segurança](#segurança)
- [Dependências](#dependências)
- [Desenvolvimento](#desenvolvimento)

---

## Visão Geral

O **OIDC-RADIUS Bridge** resolve um problema clássico de infraestrutura de rede: equipamentos como switches gerenciados, access points Wi-Fi, VPNs e outros clientes de rede usam RADIUS para autenticação de utilizadores, mas as organizações modernas gerem identidades em provedores OIDC/OAuth2 (Keycloak, Azure AD, Okta, etc.).

Este serviço faz a ponte entre os dois mundos:

```
Cliente de Rede → FreeRADIUS → [OIDC-RADIUS Bridge] → Keycloak / OIDC Provider
```

O utilizador autentica com as suas credenciais corporativas (SSO), e o RADIUS responde com sucesso ou rejeição.

---

## Arquitetura

```
┌─────────────────────────────────────────────────────────────────┐
│                        Rede Corporativa                         │
│                                                                 │
│  ┌─────────────┐   RADIUS    ┌──────────────────┐              │
│  │  Switch /   │ ──────────► │                  │              │
│  │  AP / VPN   │             │   FreeRADIUS     │              │
│  │  (cliente)  │ ◄────────── │   Server         │              │
│  └─────────────┘   RADIUS    └────────┬─────────┘              │
│                                       │                         │
│                                  exec module                    │
│                                  (chama script)                 │
│                                       │                         │
│                              ┌────────▼────────┐               │
│                              │  radius_auth.py  │               │
│                              │  (Python script) │               │
│                              └────────┬────────┘               │
│                                       │ HTTP POST               │
│                                       │ localhost:8080          │
│                              ┌────────▼────────┐               │
│                              │  OIDC-RADIUS     │               │
│                              │  Bridge (Go)     │               │
│                              │  :8080 /auth     │               │
│                              └────────┬────────┘               │
└───────────────────────────────────────┼─────────────────────────┘
                                        │ OAuth2 Password Grant
                                        │ HTTPS
                               ┌────────▼────────┐
                               │  OIDC Provider   │
                               │  (Keycloak)      │
                               │  /token endpoint │
                               └─────────────────┘
```

### Protocolos envolvidos

| Segmento | Protocolo | Notas |
|----------|-----------|-------|
| Cliente → FreeRADIUS | RADIUS (UDP 1812) | Protocolo legado de rede |
| FreeRADIUS → Script | Chamada de processo (exec) | Args: username, password |
| Script → Bridge | HTTP POST JSON (localhost) | Sem TLS (loopback local) |
| Bridge → OIDC | HTTPS OAuth2 (Password Grant) | TLS obrigatório |

---

## Fluxo de Autenticação

```
Cliente           FreeRADIUS       radius_auth.py    Bridge (Go)        Keycloak
   │                   │                 │                 │                 │
   │── Access-Request ►│                 │                 │                 │
   │   (user + pass)   │                 │                 │                 │
   │                   │─ exec(u,p) ────►│                 │                 │
   │                   │                 │─ POST /auth ───►│                 │
   │                   │                 │  {user, pass}   │                 │
   │                   │                 │                 │─ POST /token ──►│
   │                   │                 │                 │  Password Grant  │
   │                   │                 │                 │◄─ access_token ─│
   │                   │                 │◄── 200 OK ──────│  (ou 401)       │
   │                   │◄── exit(0) ─────│                 │                 │
   │◄─ Access-Accept ──│                 │                 │                 │
```

**Passo a passo:**

1. O cliente de rede envia um `Access-Request` RADIUS com `User-Name` e `User-Password`.
2. O FreeRADIUS chama `radius_auth.py` via módulo `exec`, passando username e password como argumentos.
3. O script Python faz um `POST /auth` para o Bridge em `http://localhost:8080/auth` com payload JSON.
4. O Bridge Go valida a requisição e chama `OIDCProvider.Authenticate()`.
5. O `OIDCProvider` usa o fluxo **Resource Owner Password Credentials Grant** para obter um token do Keycloak.
6. O Keycloak valida as credenciais e retorna um access token (sucesso) ou erro 401 (falha).
7. O Bridge responde com HTTP 200 (sucesso) ou 401 (falha).
8. O script termina com `exit(0)` (sucesso) ou `exit(1)` (falha).
9. O FreeRADIUS responde ao cliente com `Access-Accept` ou `Access-Reject`.

---

## Estrutura do Projeto

```
oicd-radius-bridge/
├── cmd/
│   └── server/
│       └── main.go              # Ponto de entrada: inicialização e servidor HTTP
├── config/
│   └── config.go                # Carregamento de configuração via variáveis de ambiente
├── internal/
│   ├── api/
│   │   └── handler.go           # Handler HTTP: POST /auth
│   └── auth/
│       ├── oidc.go              # Implementação do provedor OIDC (discovery + token)
│       └── service.go           # Interface Service e implementação OIDCService
├── pkg/
│   └── logger/
│       └── logger.go            # Logger estruturado com prefixo e nível
├── scripts/
│   └── radius_auth.py           # Script Python chamado pelo FreeRADIUS via exec
├── .env                         # Variáveis de ambiente (não commitar com segredos reais)
├── .env.example                 # Template de configuração
├── Dockerfile                   # Build multi-stage + imagem Alpine mínima
├── go.mod                       # Módulo Go e dependências
└── go.sum                       # Checksums de dependências
```

---

## Componentes

### `cmd/server/main.go` — Ponto de Entrada

Responsável pela inicialização do sistema na seguinte sequência:

1. Carrega o `.env` via `godotenv` (falha não-fatal se o ficheiro não existir)
2. Cria o logger
3. Carrega a configuração (`config.LoadConfig()`)
4. Inicializa o `OIDCProvider` — faz discovery OIDC (`/.well-known/openid-configuration`)
5. Cria o `OIDCService` com o provider
6. Cria o `Handler` HTTP e regista as rotas
7. Sobe servidor HTTP na porta `:8080`
8. Aguarda `SIGINT` ou `SIGTERM` para shutdown gracioso (timeout 5s)

**Timeouts do servidor:**
- `ReadTimeout`: 5s
- `WriteTimeout`: 5s
- `IdleTimeout`: 30s

---

### `config/config.go` — Configuração

Estrutura `Config` populada a partir de variáveis de ambiente com fallback para valores padrão:

```go
type Config struct {
    OIDCProviderURL  string  // URL de discovery do provedor OIDC
    OIDCClientID     string  // Client ID OAuth2
    OIDCClientSecret string  // Client Secret OAuth2
    LogLevel         string  // Nível de log
}
```

---

### `internal/auth/oidc.go` — Provedor OIDC

Faz discovery OIDC em `OIDC_PROVIDER_URL` para obter o token endpoint e outros metadados. Usa `oauth2.Config.PasswordCredentialsToken()` para o fluxo **Resource Owner Password Credentials Grant**.

Scopes solicitados: `openid`, `profile`, `email`.

> O fluxo Password Credentials está depreciado no OAuth 2.1, mas é amplamente suportado pelo Keycloak para integrações de serviço como esta.

---

### `internal/auth/service.go` — Serviço de Autenticação

Interface de abstracção sobre o OIDCProvider:

```go
type Service interface {
    Authenticate(ctx context.Context, username, password string) error
}
```

O token retornado pelo Keycloak é descartado — apenas o sucesso/falha é relevante para o RADIUS.

---

### `internal/api/handler.go` — Handler HTTP

Regista a rota `POST /auth`. Processamento:

1. Verifica método HTTP (`POST` only)
2. Verifica `Content-Type: application/json`
3. Decode do JSON body para `AuthRequest{Username, Password}`
4. Valida que username e password não estão vazios
5. Chama `authService.Authenticate()`
6. Retorna `AuthResponse{Success, Message}` como JSON

---

### `scripts/radius_auth.py` — Script FreeRADIUS

Chamado pelo FreeRADIUS via módulo `exec`:

```
radius_auth.py <username> <password>
  exit 0 → autenticação bem-sucedida
  exit 1 → falha ou erro de rede
```

Faz `POST http://localhost:8080/auth` com timeout de 5 segundos.

---

## Configuração

### Variáveis de Ambiente

| Variável | Obrigatória | Padrão | Descrição |
|----------|-------------|--------|-----------|
| `OIDC_PROVIDER_URL` | Sim | `https://account.uem.mz/realms/uem` | URL base do realm Keycloak |
| `OIDC_CLIENT_ID` | Sim | `radius-client` | Client ID configurado no Keycloak |
| `OIDC_CLIENT_SECRET` | Sim | _(vazio)_ | Client Secret do client confidencial |
| `LOG_LEVEL` | Não | `info` | Nível de log (debug, info, warn, error) |

### Ficheiro `.env`

```env
OIDC_PROVIDER_URL=https://seu-keycloak.exemplo.com/realms/nome-realm
OIDC_CLIENT_ID=radius-oidc
OIDC_CLIENT_SECRET=seu-client-secret-aqui
LOG_LEVEL=info
```

### Configuração do Keycloak

O client OAuth2 no Keycloak deve ter:

- **Client Protocol:** `openid-connect`
- **Access Type:** `confidential`
- **Direct Access Grants Enabled:** `ON` (necessário para Password Credentials Grant)

---

## Instalação

### Via Docker (Recomendado)

```bash
# 1. Clonar o repositório
git clone https://github.com/nazarioz/oidc-radius-bridge.git
cd oidc-radius-bridge

# 2. Criar e editar o .env
cp .env.example .env

# 3. Build da imagem
docker build -t oidc-radius-bridge .

# 4. Executar (host network para o FreeRADIUS aceder via localhost)
docker run -d \
  --name oidc-radius-bridge \
  --network host \
  --restart unless-stopped \
  -v $(pwd)/.env:/app/.env:ro \
  oidc-radius-bridge
```

### Manual

```bash
# Dependências
go mod download
pip3 install requests

# Build
go build -o oidc-radius-bridge ./cmd/server
chmod +x scripts/radius_auth.py

# Executar
cp .env.example .env
./oidc-radius-bridge
```

### Como serviço systemd

```ini
# /etc/systemd/system/oidc-radius-bridge.service
[Unit]
Description=OIDC-RADIUS Bridge
After=network-online.target

[Service]
Type=simple
User=oidc-radius
WorkingDirectory=/opt/oidc-radius-bridge
ExecStart=/opt/oidc-radius-bridge/oidc-radius-bridge
EnvironmentFile=/opt/oidc-radius-bridge/.env
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now oidc-radius-bridge
```

---

## Integração com FreeRADIUS

### 1. Instalar dependência Python

```bash
pip3 install requests
```

### 2. Copiar o script

```bash
cp scripts/radius_auth.py /etc/freeradius/3.0/scripts/
chmod +x /etc/freeradius/3.0/scripts/radius_auth.py
```

### 3. Configurar o módulo exec

`/etc/freeradius/3.0/mods-available/oidc_auth`:

```ini
exec oidc_auth {
    wait = yes
    program = "/etc/freeradius/3.0/scripts/radius_auth.py %{User-Name} %{User-Password}"
    input_pairs = request
    output_pairs = reply
    shell_escape = yes
}
```

```bash
ln -s /etc/freeradius/3.0/mods-available/oidc_auth \
      /etc/freeradius/3.0/mods-enabled/oidc_auth
```

### 4. Configurar o virtual server

Em `/etc/freeradius/3.0/sites-enabled/default`:

```ini
authorize {
    update control {
        Auth-Type := OIDC
    }
}

authenticate {
    Auth-Type OIDC {
        oidc_auth
    }
}
```

### 5. Reiniciar

```bash
systemctl restart freeradius
journalctl -u freeradius -f
```

---

## API Reference

### `POST /auth`

**Request:**

```http
POST /auth HTTP/1.1
Host: localhost:8080
Content-Type: application/json

{"username": "user@exemplo.com", "password": "Password123"}
```

**Respostas:**

| Código | Body | Situação |
|--------|------|----------|
| `200` | `{"success":true,"message":"Authentication successful"}` | Credenciais válidas |
| `401` | `{"success":false,"message":"Authentication failed"}` | Credenciais inválidas |
| `400` | texto simples | Content-Type errado / JSON inválido / campos em falta |
| `405` | texto simples | Método que não seja POST |

**Exemplo curl:**

```bash
curl -s -X POST http://localhost:8080/auth \
  -H "Content-Type: application/json" \
  -d '{"username":"user@exemplo.com","password":"Password123"}' | jq .
```

---

## Logging

Formato de output:

```
[OIDC-RADIUS] 2025/06/04 10:30:00 ficheiro.go:linha: [LEVEL] Mensagem
```

Exemplos:

```
[OIDC-RADIUS] 2025/06/04 10:30:00 main.go:42: [INFO] OIDC provider initialized successfully
[OIDC-RADIUS] 2025/06/04 10:30:00 main.go:64: [INFO] Starting OIDC-RADIUS bridge server on port 8080...
[OIDC-RADIUS] 2025/06/04 10:30:05 handler.go:89: [INFO] Successfully authenticated user: user@exemplo.com
[OIDC-RADIUS] 2025/06/04 10:30:07 handler.go:80: [ERROR] Authentication failed for user baduser: ...
```

> Passwords e tokens nunca são escritos nos logs.

---

## Segurança

### Comunicação local

O serviço escuta em `:8080` (todas as interfaces). Em produção, restringir o bind a `127.0.0.1` em `cmd/server/main.go`:

```go
Addr: "127.0.0.1:8080",
```

Ou bloquear via iptables:

```bash
iptables -A INPUT -p tcp --dport 8080 ! -s 127.0.0.1 -j DROP
```

### Credenciais

- Credenciais do utilizador apenas em memória — não são persistidas nem logadas.
- `OIDC_CLIENT_SECRET` não deve ser commitado no repositório.
- O access token retornado pelo Keycloak é descartado após validação.

### Container

- Executa como utilizador não-root (`appuser`)
- Imagem base mínima (`alpine:3.19`)
- Binary estático (`CGO_ENABLED=0`)
- Inclui `ca-certificates` para validação TLS

---

## Dependências

### Go

| Pacote | Versão | Uso |
|--------|--------|-----|
| `github.com/coreos/go-oidc` | v2.3.0 | Discovery OIDC |
| `golang.org/x/oauth2` | v0.30.0 | Password Credentials Grant |
| `github.com/joho/godotenv` | v1.5.1 | Carregamento do `.env` |
| `layeh.com/radius` | v0.0.0-20221205 | Biblioteca RADIUS (disponível para uso futuro) |

### Python

| Pacote | Uso |
|--------|-----|
| `requests` | HTTP client para chamar o Bridge |

---

## Desenvolvimento

```bash
# Executar localmente
go run ./cmd/server

# Build
go build -o oidc-radius-bridge ./cmd/server

# Testar endpoint
curl -X POST http://localhost:8080/auth \
  -H "Content-Type: application/json" \
  -d '{"username":"user@exemplo.com","password":"Password123"}'

# Testar script directamente
python3 scripts/radius_auth.py user@exemplo.com Password123
echo "Exit: $?"

# Testar via radtest
radtest user@exemplo.com Password123 localhost 0 testing123
```

### Provedores OIDC suportados

Qualquer provedor com OIDC Discovery (RFC 8414) e suporte a Direct Access Grants:

| Provedor | URL de discovery |
|----------|-----------------|
| Keycloak | `https://keycloak.exemplo.com/realms/{realm}` |
| Azure AD | `https://login.microsoftonline.com/{tenant}/v2.0` |
| Okta | `https://dev-xxx.okta.com/oauth2/default` |
| Google | `https://accounts.google.com` |

---

## Licença

MIT License
