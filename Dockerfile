# Etapa de build
FROM golang:1.24-alpine AS builder

# Instala dependências de build
RUN apk add --no-cache git

# Define diretório de trabalho
WORKDIR /app

# Copia arquivos de dependências
COPY go.mod go.sum ./

# Baixa dependências
RUN go mod download

# Copia código-fonte
COPY . .

# Compila o binário
RUN CGO_ENABLED=0 GOOS=linux go build -o oidc-radius-bridge ./cmd/server

# Etapa de runtime
FROM alpine:3.19

# Instala dependências de runtime
RUN apk add --no-cache ca-certificates tzdata

# Cria usuário não root
RUN adduser -D -g '' appuser

# Define diretório de trabalho
WORKDIR /app

# Copia binário e scripts do builder
COPY --from=builder /app/oidc-radius-bridge .
COPY --from=builder /app/scripts/radius_auth.py /app/scripts/

# Permissões corretas
RUN chown -R appuser:appuser /app

# Usa usuário não root
USER appuser

# Expõe a porta 8080
EXPOSE 8080

# Comando padrão
CMD ["./oidc-radius-bridge"]
