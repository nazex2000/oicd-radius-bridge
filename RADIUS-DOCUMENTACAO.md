# Documentação Técnica — Sistema de Autenticação RADIUS + OIDC Bridge
**Universidade Eduardo Mondlane (UEM)**
**Última actualização:** Junho 2026
**Servidor:** `eduroam` | IP: `196.3.100.204`

---

## Índice

1. [Visão Geral da Arquitectura](#1-visão-geral-da-arquitectura)
2. [Componentes do Sistema](#2-componentes-do-sistema)
3. [Fluxo de Autenticação Detalhado](#3-fluxo-de-autenticação-detalhado)
4. [FreeRADIUS — Configuração Completa](#4-freeradius--configuração-completa)
5. [OIDC-RADIUS Bridge (Go)](#5-oidc-radius-bridge-go)
6. [Keycloak (Backend de Identidade)](#6-keycloak-backend-de-identidade)
7. [Certificados TLS](#7-certificados-tls)
8. [Clientes RADIUS Registados](#8-clientes-radius-registados)
9. [Gestão Operacional](#9-gestão-operacional)
10. [Procedimentos de Manutenção](#10-procedimentos-de-manutenção)
11. [Resolução de Problemas](#11-resolução-de-problemas)
12. [Preparação para eduroam](#12-preparação-para-eduroam)
13. [Referência Rápida](#13-referência-rápida)

---

## 1. Visão Geral da Arquitectura

Este sistema implementa uma **bridge de autenticação entre o protocolo RADIUS (legado) e o Keycloak (OIDC moderno)**. Permite que equipamentos de rede (switches, access points) que apenas falam RADIUS autentiquem utilizadores cujas identidades estão centralizadas no Keycloak da UEM.

### Diagrama Geral

```
┌─────────────────────────────────────────────────────────────────────┐
│  EQUIPAMENTOS DE REDE (NAS — Network Access Servers)                │
│                                                                     │
│  AP / Switch UEM          AP / Switch UEM        AP / Switch UEM    │
│  196.3.96.159             196.3.100.197           196.3.98.45       │
│  (uem-orlando)            (uem-srv2)              (uem-orlando-2)   │
└────────────────────────────────┬────────────────────────────────────┘
                                 │  UDP porta 1812 (autenticação)
                                 │  UDP porta 1813 (accounting)
                                 │  Shared Secret: uem@2025
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│  FREERADIUS 3.0.26                                                  │
│  Servidor: eduroam (196.3.100.204)                                  │
│  /etc/freeradius/3.0/                                               │
│                                                                     │
│  ┌──────────────────┐    ┌──────────────────────────────────────┐  │
│  │  Virtual Server  │    │  Virtual Server: inner-tunnel         │  │
│  │  "default"       │    │  (dentro do túnel TLS do EAP)        │  │
│  │  porta 1812/1813 │───▶│  authorize → Auth-Type := REST       │  │
│  │  EAP-TTLS inicio │    │  authenticate → módulo rest          │  │
│  └──────────────────┘    └──────────────┬───────────────────────┘  │
│                                         │ HTTP POST /auth           │
│                                         │ localhost:8080            │
└─────────────────────────────────────────┼───────────────────────────┘
                                          │
                    ┌─────────────────────▼──────────────────────────┐
                    │  OIDC-RADIUS BRIDGE (Go 1.24)                  │
                    │  Docker container: oidc-radius-bridge          │
                    │  Imagem: oidc:1.0.2                            │
                    │  Porta: 127.0.0.1:8080 (apenas localhost)      │
                    │                                                │
                    │  POST /auth                                    │
                    │  {"username": "...", "password": "..."}        │
                    │      ↓                                         │
                    │  OAuth2 Password Grant (ROPC)                  │
                    └─────────────────────┬──────────────────────────┘
                                          │  HTTPS
                                          ▼
                    ┌───────────────────────────────────────────────┐
                    │  KEYCLOAK                                     │
                    │  https://account.uem.mz/realms/develop        │
                    │  Client ID: radius-oidc                       │
                    │                                               │
                    │  Valida username + password                   │
                    │  Retorna token JWT se válido                  │
                    └───────────────────────────────────────────────┘
```

### Protocolo EAP Utilizado

O método de autenticação é **EAP-TTLS/PAP**:

- **EAP-TTLS** — Extensible Authentication Protocol com Tunneled TLS. Cria um túnel TLS encriptado entre o dispositivo e o servidor RADIUS. O utilizador nunca vê o certificado nem envia credenciais fora do túnel.
- **PAP** (inner method) — Dentro do túnel TLS, o dispositivo envia username e password em texto limpo *mas protegido pelo túnel TLS*. O FreeRADIUS passa essas credenciais ao Bridge, que valida no Keycloak.

**Porquê TTLS/PAP e não PEAP/MSCHAPv2?**
O MSCHAPv2 (usado dentro do PEAP) requer que o servidor conheça o hash NT da password do utilizador. O Keycloak não expõe hashes NT, apenas valida credenciais via protocolo OIDC. Logo, o único método compatível com o nosso backend Keycloak é TTLS/PAP.

---

## 2. Componentes do Sistema

### 2.1 FreeRADIUS

| Atributo | Valor |
|---------|-------|
| Versão | 3.0.26 |
| Directório de config | `/etc/freeradius/3.0/` |
| Utilizador do processo | `freerad` |
| Ficheiro de log | `/var/log/freeradius/radius.log` |
| Serviço systemd | `freeradius.service` |
| Auto-arranque | Sim (enabled) |

### 2.2 OIDC-RADIUS Bridge

| Atributo | Valor |
|---------|-------|
| Linguagem | Go 1.24 |
| Código fonte | `/home/netadmin/oicd-radius-bridge/` |
| Container Docker | `oidc-radius-bridge` |
| Imagem | `oidc:1.0.2` |
| Porta exposta | `127.0.0.1:8080` (apenas localhost) |
| Restart policy | `always` (inicia com o sistema) |
| Configuração | `/home/netadmin/oicd-radius-bridge/.env` |
| docker-compose | `/home/netadmin/oicd-radius-bridge/docker-compose.yml` |

### 2.3 Keycloak (externo)

| Atributo | Valor |
|---------|-------|
| URL | `https://account.uem.mz/realms/develop` |
| Client ID | `radius-oidc` |
| Client Secret | `JLfmLsbINMOVcBdU8aQTLP3dQCRGUadW` |
| Fluxo OAuth2 | Resource Owner Password Credentials (ROPC) |

---

## 3. Fluxo de Autenticação Detalhado

### Passo a passo — utilizador a ligar-se ao WiFi

```
1. Utilizador selecciona a rede WiFi e insere username@uem.mz + password

2. O Access Point (AP) recebe as credenciais e inicia o protocolo EAP com o 
   dispositivo do utilizador.

3. O AP envia um RADIUS Access-Request para FreeRADIUS (196.3.100.204:1812)
   com o atributo EAP-Message (início do handshake EAP-TTLS).

4. FreeRADIUS — Virtual Server "default" — pipeline authorize:
   - Módulo eap detecta EAP-Message → inicia negociação EAP-TTLS
   - FreeRADIUS envia EAP-Request/TTLS ao AP, que passa ao dispositivo
   - Dispositivo e FreeRADIUS fazem handshake TLS
   - FreeRADIUS apresenta o seu certificado (eduroam.uem.mz)
   - Dispositivo valida o certificado contra o CA da UEM
   - Túnel TLS estabelecido

5. Dentro do túnel TLS (inner-tunnel):
   - Dispositivo envia username e password via PAP (protegido pelo túnel)
   - FreeRADIUS recebe User-Name e User-Password no inner-tunnel
   - Pipeline authorize do inner-tunnel:
     → eap (noop para PAP)
     → files (verifica utilizadores locais — vazio em produção)
     → if (User-Password presente) → Auth-Type := REST ✓

6. FreeRADIUS — inner-tunnel — pipeline authenticate:
   - Auth-Type = REST → chama módulo rest
   - Módulo rest faz HTTP POST para http://127.0.0.1:8080/auth
     Body: {"username": "user@uem.mz", "password": "password123"}

7. OIDC-RADIUS Bridge (Docker):
   - Recebe POST /auth
   - Valida Content-Type e campos obrigatórios
   - Chama Keycloak via OAuth2 ROPC:
     POST https://account.uem.mz/realms/develop/protocol/openid-connect/token
     grant_type=password&username=user@uem.mz&password=password123
     &client_id=radius-oidc&client_secret=...&scope=openid profile email
   - Keycloak responde com token JWT (sucesso) ou erro 401 (falha)
   - Bridge retorna HTTP 200 {"success":true} ou HTTP 401 {"success":false}

8. FreeRADIUS:
   - HTTP 200 → autenticação bem-sucedida → Access-Accept para AP
   - HTTP 401 → autenticação falhou → Access-Reject para AP

9. AP:
   - Access-Accept → utilizador entra na rede ✓
   - Access-Reject → utilizador é recusado ✗
```

### Diagrama de Sequência Simplificado

```
Dispositivo    AP/Switch     FreeRADIUS    Bridge(Docker)    Keycloak
     │              │              │               │              │
     │──EAP-Start──▶│              │               │              │
     │              │──RADIUS Req─▶│               │              │
     │◀─EAP Req TLS─│◀─────────────│               │              │
     │    (TLS Handshake)           │               │              │
     │──────────────────────────── TLS established ─────────────────
     │──PAP: user+pass (dentro TLS)▶│               │              │
     │              │              │──POST /auth───▶│              │
     │              │              │               │──ROPC Grant──▶│
     │              │              │               │◀─JWT Token────│
     │              │              │◀──200 OK──────│              │
     │              │◀─Access-Accept──────────────│              │
     │◀─Rede OK─────│              │               │              │
```

---

## 4. FreeRADIUS — Configuração Completa

### Estrutura de Ficheiros

```
/etc/freeradius/3.0/
├── radiusd.conf              ← Configuração principal (threads, logs, includes)
├── clients.conf              ← Equipamentos de rede autorizados (NAS)
├── proxy.conf                ← Routing de realms (eduroam, local)
├── certs/
│   ├── ca.pem                ← Certificado CA (distribuir aos dispositivos)
│   ├── ca.key                ← Chave privada CA (PROTEGER)
│   ├── server.pem            ← Certificado do servidor RADIUS
│   ├── server.key            ← Chave privada do servidor (PROTEGER)
│   ├── ca.cnf                ← Config para gerar CA
│   └── server.cnf            ← Config para gerar cert servidor
├── sites-enabled/
│   ├── default -> ../sites-available/default   ← Virtual server principal
│   └── inner-tunnel          ← Virtual server do túnel EAP (ficheiro directo)
├── mods-enabled/
│   ├── eap -> ../mods-available/eap            ← Módulo EAP (TTLS/PEAP/TLS)
│   ├── rest -> ../mods-available/rest          ← Módulo REST (→ Bridge)
│   ├── pap, chap, mschap     ← Módulos de autenticação
│   ├── files                 ← Utilizadores locais
│   ├── realm                 ← Processamento de @domínio
│   └── ...                   ← Outros módulos standard
└── mods-config/
    ├── files/authorize       ← Ficheiro de utilizadores locais
    └── rest/                 ← Config REST adicional
```

### 4.1 Virtual Server "default" (`sites-enabled/default`)

É o servidor principal. Recebe todos os pedidos RADIUS na porta 1812.

**Secção `authorize`** — decide como vai ser autenticado:
```
eap          → se pedido tem EAP-Message, inicia negociação EAP-TTLS
               (o módulo EAP trata de todo o handshake TLS)

se não-EAP e tem User-Password → Auth-Type := REST
  (para autenticação PAP directa, sem EAP — raro em WiFi)

rest         → tenta buscar atributos do utilizador (noop — Bridge não tem endpoint GET)
filter_username, preprocess, chap, mschap, digest
suffix       → extrai o realm do username (user@uem.mz → realm = uem.mz)
files        → verifica ficheiro local de utilizadores
pap          → prepara autenticação PAP
```

**Secção `authenticate`** — executa a autenticação:
```
Auth-Type REST { rest }    → chama Bridge via HTTP
Auth-Type MS-CHAP { mschap } → MS-CHAP local (não usado com Bridge)
eap                        → processa handshake EAP (TTLS, PEAP, etc.)
```

**Secção `accounting`** — regista sessões:
```
detail  → grava em /var/log/freeradius/radacct/
unix    → registo UNIX
exec    → script externo (não configurado)
```

### 4.2 Virtual Server "inner-tunnel" (`sites-enabled/inner-tunnel`)

Corre **dentro do túnel TLS** estabelecido pelo EAP-TTLS. Só é invocado para pedidos EAP-TTLS/PEAP. Neste ponto o canal já está encriptado.

```
authorize:
  eap          → EAP aninhado (raro em TTLS/PAP)
  files        → verifica utilizadores locais (emergência/testes)

  if (User-Password):
      Auth-Type := REST     → TTLS/PAP: vai para Keycloak ✓

  elsif (MS-CHAP-Response || MS-CHAP2-Response):
      Reject com mensagem   → MSCHAPv2 não suportado ✗

  elsif (não há EAP-Message):
      Reject com mensagem   → método desconhecido ✗

authenticate:
  Auth-Type REST { rest }   → Bridge → Keycloak
  Auth-Type PAP  { pap }    → utilizadores locais (ficheiro)
  eap                       → EAP aninhado
```

### 4.3 Módulo EAP (`mods-available/eap`)

```
default_eap_type = ttls      ← tipo padrão para novos pedidos

tls-config tls-common:
  Certificados: /etc/freeradius/3.0/certs/
  TLS versão:   1.2 (min e max)
  Cifras:       HIGH sem MD5/RC4/3DES/aNULL
  Curva ECDH:   prime256v1

ttls:
  inner tunnel → inner-tunnel virtual server
  copy_request_to_tunnel = yes   ← passa atributos NAS para o inner tunnel
  use_tunneled_reply = yes        ← usa resposta do inner tunnel na resposta final

peap:
  inner = mschapv2 (configurado mas NÃO funcional com Keycloak)
  inner tunnel → inner-tunnel virtual server
```

### 4.4 Módulo REST (`mods-available/rest`)

Liga o FreeRADIUS ao Bridge Go:

```
connect_uri = "http://127.0.0.1:8080/"

authenticate:
  POST http://127.0.0.1:8080/auth
  Content-Type: application/json
  Body: {"username": "%{User-Name}", "password": "%{User-Password}"}

  HTTP 200 → Accept
  HTTP 401 → Reject
  Outro    → Fail (tratado como Reject)

Pool de conexões:
  min/max: herdado do thread pool (5-32 threads)
  idle_timeout: 60s
  retry_delay: 30s
```

### 4.5 Módulo Files (`mods-config/files/authorize`)

Ficheiro de utilizadores locais. Útil para contas de teste ou emergência que não passam pelo Keycloak.

```
# Actualmente vazio (linhas DEFAULT para PPP/SLIP são legado)
# Para adicionar utilizador local:
# utilizador Cleartext-Password := "password"
```

**Nota:** Utilizadores neste ficheiro autenticam directamente no FreeRADIUS, sem passar pelo Keycloak.

---

## 5. OIDC-RADIUS Bridge (Go)

### Localização e Estrutura

```
/home/netadmin/oicd-radius-bridge/
├── cmd/server/main.go        ← ponto de entrada, servidor HTTP
├── config/config.go          ← carrega variáveis de ambiente
├── internal/
│   ├── api/handler.go        ← endpoint POST /auth
│   └── auth/
│       ├── oidc.go           ← cliente OIDC/OAuth2
│       └── service.go        ← interface de autenticação
├── pkg/logger/logger.go      ← logger estruturado [OIDC-RADIUS]
├── scripts/radius_auth.py    ← script Python legacy (não usado actualmente)
├── Dockerfile                ← build multi-stage (golang → alpine)
├── docker-compose.yml        ← orquestração Docker
└── .env                      ← configuração (OIDC_PROVIDER_URL, etc.)
```

### Configuração (`.env`)

```env
OIDC_PROVIDER_URL=https://account.uem.mz/realms/develop
OIDC_CLIENT_ID=radius-oidc
OIDC_CLIENT_SECRET=JLfmLsbINMOVcBdU8aQTLP3dQCRGUadW
LOG_LEVEL=info
```

### docker-compose.yml

```yaml
services:
  oidc-radius-bridge:
    build: .
    image: oidc:1.0.2
    container_name: oidc-radius-bridge
    restart: always                    # reinicia com o sistema
    ports:
      - "127.0.0.1:8080:8080"         # só acessível localmente
    env_file:
      - .env
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

### Endpoint da API

```
POST http://127.0.0.1:8080/auth
Content-Type: application/json

Request:
{
  "username": "utilizador@uem.mz",
  "password": "password123"
}

Response (sucesso):
HTTP 200
{"success": true, "message": "Authentication successful"}

Response (falha):
HTTP 401
{"success": false, "message": "Authentication failed"}

Response (erro de input):
HTTP 400
{"success": false, "message": "username and password are required"}
```

### Como o Docker inicia automaticamente

O `restart: always` no docker-compose garante que o container inicia com o Docker daemon. O Docker daemon por sua vez inicia com o sistema via systemd. Portanto a ordem de arranque é:

```
Sistema inicia → Docker daemon → container oidc-radius-bridge → FreeRADIUS
```

**Nota:** O FreeRADIUS demora ~2-3 segundos a iniciar após o Docker. Se o Bridge ainda não estiver pronto quando o FreeRADIUS começa a aceitar pedidos, o módulo REST tentará reconectar automaticamente (retry_delay = 30s).

---

## 6. Keycloak (Backend de Identidade)

O Keycloak é o sistema de gestão de identidades da UEM. O FreeRADIUS não tem acesso directo ao Keycloak — toda a comunicação passa pelo Bridge.

### Configuração necessária no Keycloak

No realm `develop` de `account.uem.mz`, o client `radius-oidc` deve ter:

| Configuração | Valor |
|-------------|-------|
| Client authentication | ON (confidential) |
| Direct Access Grants | **ON** (obrigatório para ROPC) |
| Standard Flow | Pode estar OFF |
| Service accounts | Pode estar OFF |

**O "Direct Access Grants" tem de estar activado** — é este que permite o fluxo ROPC (username+password directamente).

### Fluxo OAuth2 usado (ROPC)

```
POST https://account.uem.mz/realms/develop/protocol/openid-connect/token
Content-Type: application/x-www-form-urlencoded

grant_type=password
&username=utilizador@uem.mz
&password=password123
&client_id=radius-oidc
&client_secret=JLfmLsbINMOVcBdU8aQTLP3dQCRGUadW
&scope=openid profile email

Resposta de sucesso: HTTP 200 com JWT access_token
Resposta de falha:   HTTP 401 {"error": "invalid_grant"}
```

---

## 7. Certificados TLS

Os certificados são usados pelo EAP-TTLS para encriptar o túnel. O dispositivo do utilizador valida o certificado do servidor antes de enviar credenciais.

### Certificados actuais

| Certificado | Ficheiro | Validade |
|-------------|---------|---------|
| CA da UEM | `/etc/freeradius/3.0/certs/ca.pem` | 4 Jun 2026 → 1 Jun 2036 (10 anos) |
| Servidor RADIUS | `/etc/freeradius/3.0/certs/server.pem` | 4 Jun 2026 → 6 Set 2028 (825 dias) |

### Detalhes do certificado CA

```
Subject: C=MZ, ST=Maputo, L=Maputo, O=Universidade Eduardo Mondlane
CN: UEM eduroam Certificate Authority
Email: noc@uem.mz
```

### Detalhes do certificado Servidor

```
Subject: C=MZ, ST=Maputo, O=Universidade Eduardo Mondlane
CN: eduroam.uem.mz
Email: noc@uem.mz
SANs: DNS:eduroam.uem.mz, DNS:radius.uem.mz, NAIRealm:uem.mz
```

### Distribuição do CA aos dispositivos

Os dispositivos dos utilizadores precisam de confiar no CA da UEM para validar o certificado do servidor RADIUS. Sem isso, a ligação EAP-TTLS falhará (ou o dispositivo avisa o utilizador que o certificado não é confiável).

**Formas de distribuir o CA:**

1. **eduroam CAT** (recomendado para eduroam) — O eduroam Configuration Assistance Tool gera perfis de configuração automáticos para Windows, macOS, iOS, Android que incluem o CA e configuram o EAP-TTLS/PAP.

2. **MDM/GPO** — Para dispositivos geridos pela UEM, distribuir via Mobile Device Management ou Group Policy.

3. **Manual** — O utilizador instala o ca.pem manualmente no dispositivo.

Para exportar o CA para distribuição:
```bash
sudo cat /etc/freeradius/3.0/certs/ca.pem
```

### Renovar o certificado do servidor (quando expirar em Set 2028)

```bash
cd /etc/freeradius/3.0/certs

# Limpar apenas o certificado servidor (manter CA)
rm -f server.pem server.key server.crt server.csr 02.pem

# Gerar novo CSR
openssl req -new -keyout server.key -out server.csr \
    -config server.cnf -passout pass:whatever

# Assinar com o CA existente
openssl ca -batch -keyfile ca.key -cert ca.pem \
    -in server.csr -passin pass:whatever \
    -out server.crt -config server.cnf -extensions v3_req
cp server.crt server.pem

# Corrigir permissões
chown freerad:freerad server.pem server.key server.crt server.csr
chmod 640 server.key
chmod 644 server.pem server.crt

# Reiniciar FreeRADIUS
sudo systemctl restart freeradius
```

### Renovar o CA (quando expirar em Jun 2036)

Renovar o CA implica redistribuir o novo CA a **todos os dispositivos** da rede. Planear com antecedência (6-12 meses antes).

```bash
cd /etc/freeradius/3.0/certs
rm -f ca.pem ca.key ca.crl 01.pem 02.pem server.pem server.key server.crt server.csr
echo "01" > serial
> index.txt
echo "unique_subject = yes" > index.txt.attr

# Gerar novo CA
openssl req -new -x509 -keyout ca.key -out ca.pem -days 3650 \
    -config ca.cnf -passout pass:whatever

# Gerar novo certificado servidor
openssl req -new -keyout server.key -out server.csr \
    -config server.cnf -passout pass:whatever
openssl ca -batch -keyfile ca.key -cert ca.pem \
    -in server.csr -passin pass:whatever \
    -out server.crt -config server.cnf -extensions v3_req
cp server.crt server.pem

chown freerad:freerad *.pem *.key *.crt *.csr dh
chmod 640 *.key
chmod 644 *.pem dh

sudo systemctl restart freeradius
```

---

## 8. Clientes RADIUS Registados

Os "clientes RADIUS" são os equipamentos de rede (NAS) autorizados a enviar pedidos ao servidor.

**Ficheiro:** `/etc/freeradius/3.0/clients.conf`

| Nome | IP | Secret | Notas |
|------|----|--------|-------|
| `localhost` | 127.0.0.1 | `testing123` | Testes locais com `radtest` |
| `localhost_ipv6` | ::1 | `testing123` | Testes locais IPv6 |
| `uem-orlando` | 196.3.96.159 | `uem@2025` | Equipamento UEM Orlando |
| `uem-srv2` | 196.3.100.197 | `uem@2025` | Servidor UEM 2 |
| `uem-orlando-2` | 196.3.98.45 | `uem@2025` | Equipamento UEM Orlando 2 |

Todos os clientes de rede têm `require_message_authenticator = yes` (protecção contra BlastRADIUS).

---

## 9. Gestão Operacional

### 9.1 Adicionar um novo equipamento de rede (AP ou Switch)

1. Editar o ficheiro de clientes:
```bash
sudo nano /etc/freeradius/3.0/clients.conf
```

2. Adicionar no fim do ficheiro:
```
client nome-do-equipamento {
    ipaddr      = IP_DO_EQUIPAMENTO
    secret      = uem@2025
    require_message_authenticator = yes
    shortname   = nome-curto
}
```

3. Aplicar sem interromper serviço:
```bash
sudo systemctl reload freeradius
```

4. Verificar que foi carregado:
```bash
sudo journalctl -u freeradius -n 20
```

5. Testar autenticação a partir do novo IP (ou simular com radtest):
```bash
radtest utilizador@uem.mz password 127.0.0.1 0 testing123
```

**Nota:** Se o equipamento tiver um IP diferente do registado, o FreeRADIUS rejeita o pedido com `unknown client`. Verificar os logs se houver problemas.

### 9.2 Remover um equipamento

1. Editar `/etc/freeradius/3.0/clients.conf` e apagar ou comentar o bloco `client { }`.
2. `sudo systemctl reload freeradius`

### 9.3 Alterar o secret de um equipamento

1. Editar `/etc/freeradius/3.0/clients.conf` — alterar o campo `secret`.
2. Alterar o mesmo secret no equipamento de rede.
3. `sudo systemctl reload freeradius`

**Importante:** O secret no FreeRADIUS e no equipamento têm de ser **exactamente iguais**, incluindo maiúsculas/minúsculas.

### 9.4 Adicionar utilizador local (sem Keycloak)

Para contas de emergência ou testes que autenticam directamente no FreeRADIUS:

1. Editar:
```bash
sudo nano /etc/freeradius/3.0/mods-config/files/authorize
```

2. Adicionar:
```
username Cleartext-Password := "password"
```

3. `sudo systemctl reload freeradius`

**Atenção:** Passwords em texto claro no ficheiro. Usar apenas para contas temporárias de teste. Em produção, todos os utilizadores devem estar no Keycloak.

### 9.5 Alterar configuração do Bridge (Keycloak)

Se a URL do Keycloak, Client ID ou Secret mudarem:

1. Editar o ficheiro de configuração:
```bash
nano /home/netadmin/oicd-radius-bridge/.env
```

2. Reiniciar o container:
```bash
cd /home/netadmin/oicd-radius-bridge
docker compose restart
```

3. Verificar logs:
```bash
docker logs oidc-radius-bridge --tail 20
```

### 9.6 Actualizar o código do Bridge

Se houver alterações no código Go:

```bash
cd /home/netadmin/oicd-radius-bridge

# Editar código fonte em cmd/, internal/, etc.

# Reconstruir imagem e reiniciar
docker compose down
docker compose up -d --build

# Verificar
docker logs oidc-radius-bridge --tail 30
```

---

## 10. Procedimentos de Manutenção

### 10.1 Verificar estado do sistema

```bash
# FreeRADIUS
sudo systemctl status freeradius

# Bridge Docker
docker ps --filter name=oidc-radius-bridge

# Testar autenticação end-to-end (deve retornar Reject com mensagem do Keycloak)
radtest utilizador@uem.mz wrongpassword 127.0.0.1 0 testing123

# Testar se Bridge responde
curl -s -X POST http://localhost:8080/auth \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"test"}'
```

### 10.2 Ver logs em tempo real

```bash
# FreeRADIUS — log de autenticações
sudo tail -f /var/log/freeradius/radius.log

# FreeRADIUS — log detalhado via journalctl
sudo journalctl -u freeradius -f

# Bridge Docker
docker logs oidc-radius-bridge -f

# Modo debug do FreeRADIUS (parar o serviço primeiro, correr em foreground)
sudo systemctl stop freeradius
sudo freeradius -X   # Ctrl+C para sair, depois:
sudo systemctl start freeradius
```

### 10.3 Reiniciar serviços

```bash
# FreeRADIUS
sudo systemctl restart freeradius

# Bridge
docker restart oidc-radius-bridge

# Ou via docker compose
cd /home/netadmin/oicd-radius-bridge
docker compose restart
```

### 10.4 Verificar configuração sem reiniciar

```bash
# Testa se a config está correcta (não reinicia)
sudo freeradius -XC

# Se OK: "Configuration appears to be OK"
# Se erro: mostra ficheiro e linha do problema
```

### 10.5 Recarregar configuração sem parar o serviço

```bash
# Aplica alterações em clients.conf, sites, mods sem interromper autenticações activas
sudo systemctl reload freeradius
```

### 10.6 Ver contadores e estatísticas

```bash
# Estatísticas do servidor RADIUS (se status server estiver activo)
radtest -t status "" "" 127.0.0.1 0 testing123

# Logs de accounting (por dia)
ls /var/log/freeradius/radacct/
```

### 10.7 Backup da configuração

```bash
# Backup completo da configuração
sudo tar -czf /home/netadmin/backup-radius-$(date +%Y%m%d).tar.gz \
    /etc/freeradius/3.0/ \
    /home/netadmin/oicd-radius-bridge/.env \
    /home/netadmin/oicd-radius-bridge/docker-compose.yml

# Listar o que ficou no backup
tar -tzf /home/netadmin/backup-radius-$(date +%Y%m%d).tar.gz | head -30
```

### 10.8 Restaurar configuração

```bash
sudo tar -xzf /home/netadmin/backup-radius-YYYYMMDD.tar.gz -C /
sudo systemctl restart freeradius
```

---

## 11. Resolução de Problemas

### 11.1 FreeRADIUS não inicia

```bash
# Ver erro exacto
sudo journalctl -u freeradius -n 50
# ou
sudo freeradius -XC 2>&1
```

**Erros comuns:**

| Erro | Causa | Solução |
|------|-------|---------|
| `Expecting section start brace '{'` | Erro de sintaxe num ficheiro de config | Ver ficheiro e linha indicada, corrigir sintaxe |
| `Failed to load module` | Módulo em mods-enabled não existe | Verificar symlinks em mods-enabled |
| `Address already in use` | Outro processo na porta 1812 | `sudo ss -ulpn \| grep 1812` para identificar |
| `Permission denied` on cert | Permissões erradas nos certificados | `sudo chown freerad:freerad /etc/freeradius/3.0/certs/*.key` |

### 11.2 Autenticação sempre a falhar

**Passo 1** — Verificar se o Bridge responde:
```bash
curl -s -X POST http://localhost:8080/auth \
  -H "Content-Type: application/json" \
  -d '{"username":"user@uem.mz","password":"correctpassword"}'
# Deve retornar {"success":true,...}
```

**Passo 2** — Se Bridge falha, verificar logs:
```bash
docker logs oidc-radius-bridge --tail 50
# Procurar erros de ligação ao Keycloak
```

**Passo 3** — Verificar se Keycloak está acessível:
```bash
curl -s https://account.uem.mz/realms/develop/.well-known/openid-configuration \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print('OK:', d['issuer'])"
```

**Passo 4** — Ver o que o FreeRADIUS está a fazer em detalhe:
```bash
sudo systemctl stop freeradius
sudo freeradius -X 2>&1 | grep -E "ERROR|WARNING|Auth|REST|reject|accept"
# Tentar autenticação de outro terminal enquanto está em debug
```

### 11.3 Equipamento de rede rejeitado ("unknown client")

```bash
# Ver no log o IP que está a enviar pedidos
sudo tail -f /var/log/freeradius/radius.log | grep -i "unknown\|reject"
```

O IP do equipamento não está em `clients.conf`. Adicionar como descrito em 9.1.

### 11.4 Dispositivo não consegue ligar (EAP falha)

**Sintoma:** O dispositivo inicia a ligação mas falha durante o handshake EAP.

**Causa mais comum:** O dispositivo não confia no certificado CA da UEM.

**Solução:**
1. Instalar o CA certificate no dispositivo (ver secção 7)
2. Ou temporariamente: desactivar validação de certificado no supplicant (apenas para testes)

**Verificar se é problema de certificado:**
```bash
sudo freeradius -X 2>&1 | grep -i "tls\|cert\|ssl"
```

### 11.5 Mensagem "MSCHAPv2 nao suportado"

O dispositivo está configurado para PEAP/MSCHAPv2 em vez de TTLS/PAP.

**Solução:** Reconfigurar o supplicant WiFi do dispositivo:
- Método EAP: **EAP-TTLS** (não PEAP)
- Inner method / Phase 2: **PAP** (não MSCHAPv2)
- Certificado CA: CA da UEM

### 11.6 Bridge não inicia com o sistema

```bash
# Verificar estado
docker ps -a --filter name=oidc-radius-bridge

# Se o container existe mas não está a correr
docker start oidc-radius-bridge

# Se o container não existe (foi removido)
cd /home/netadmin/oicd-radius-bridge
docker compose up -d

# Verificar se Docker inicia com o sistema
sudo systemctl is-enabled docker
# Se não: sudo systemctl enable docker
```

### 11.7 Logs do Bridge não aparecem

```bash
# Ver logs do container
docker logs oidc-radius-bridge

# Se container não existe
docker ps -a

# Verificar se imagem existe
docker images | grep oidc

# Reconstruir se necessário
cd /home/netadmin/oicd-radius-bridge
docker compose up -d --build
```

---

## 12. Preparação para eduroam

O sistema está preparado para integração com a federação eduroam. O que falta:

### 12.1 Obter credenciais da federação

Contactar a **RENU** (Rede Educacional Nacional de Moçambique) ou o operador eduroam.mz para obter:
- IP do servidor RADIUS nacional (NRPS)
- Shared secret para o servidor nacional
- IP que o servidor nacional vai usar para enviar pedidos (para adicionar em clients.conf)

### 12.2 Activar proxy para visitantes

Quando tiver as credenciais, em `/etc/freeradius/3.0/proxy.conf`:

1. Actualizar `home_server eduroam_nacional` com IP e secret reais
2. Descomentar o bloco de realm para visitantes:
```
realm "~.+\\..+" {
    auth_pool = eduroam_pool
    acct_pool = eduroam_pool
    nostrip
}
```

### 12.3 Adicionar servidor nacional como cliente

Em `/etc/freeradius/3.0/clients.conf`, descomentar e preencher:
```
client eduroam_nacional {
    ipaddr    = IP_DO_SERVIDOR_NACIONAL
    secret    = SECRET_DO_SERVIDOR_NACIONAL
    require_message_authenticator = yes
    shortname = eduroam-nacional
}
```

### 12.4 Distribuir o CA via eduroam CAT

Registar a instituição no portal eduroam CAT (cat.eduroam.org) e carregar:
- O ficheiro `ca.pem` da UEM
- Configuração EAP-TTLS/PAP
- Informações da instituição

O CAT gera instaladores automáticos para todos os sistemas operativos.

### 12.5 Testar com utilizador visitante

Após activar o proxy, testar com uma conta de outra instituição eduroam:
```bash
radtest visitante@outra-inst.mz password 127.0.0.1 0 testing123
# Deve ser proxied para o servidor nacional e retornar Accept ou Reject
```

---

## 13. Referência Rápida

### Comandos do dia-a-dia

```bash
# Estado geral
sudo systemctl status freeradius
docker ps --filter name=oidc-radius-bridge

# Logs
sudo tail -f /var/log/freeradius/radius.log
docker logs oidc-radius-bridge -f --tail 50

# Reiniciar
sudo systemctl restart freeradius
docker restart oidc-radius-bridge

# Recarregar config (sem parar)
sudo systemctl reload freeradius

# Verificar config
sudo freeradius -XC

# Testar autenticação
radtest user@uem.mz password 127.0.0.1 0 testing123

# Testar Bridge directamente
curl -s -X POST http://localhost:8080/auth \
  -H "Content-Type: application/json" \
  -d '{"username":"user@uem.mz","password":"pass"}'

# Debug completo
sudo systemctl stop freeradius && sudo freeradius -X
```

### Ficheiros importantes

| Ficheiro | Para quê |
|---------|---------|
| `/etc/freeradius/3.0/clients.conf` | Adicionar/remover equipamentos de rede |
| `/etc/freeradius/3.0/proxy.conf` | Configurar realms eduroam |
| `/etc/freeradius/3.0/mods-available/eap` | Configurar métodos EAP e certificados |
| `/etc/freeradius/3.0/sites-enabled/inner-tunnel` | Lógica de autenticação dentro do túnel |
| `/etc/freeradius/3.0/certs/ca.pem` | CA certificate — distribuir aos dispositivos |
| `/etc/freeradius/3.0/mods-config/files/authorize` | Utilizadores locais (emergência) |
| `/home/netadmin/oicd-radius-bridge/.env` | Config do Bridge (Keycloak URL, secret) |
| `/home/netadmin/oicd-radius-bridge/docker-compose.yml` | Orquestração Docker |

### Portas e Endereços

| Serviço | Endereço | Porto | Protocolo |
|---------|---------|-------|-----------|
| RADIUS Auth | 196.3.100.204 | 1812 | UDP |
| RADIUS Acct | 196.3.100.204 | 1813 | UDP |
| Bridge HTTP | 127.0.0.1 | 8080 | TCP (apenas local) |
| Keycloak | account.uem.mz | 443 | HTTPS |

### Credenciais dos clientes de rede

| Equipamento | IP | Secret |
|------------|-----|--------|
| uem-orlando | 196.3.96.159 | uem@2025 |
| uem-srv2 | 196.3.100.197 | uem@2025 |
| uem-orlando-2 | 196.3.98.45 | uem@2025 |
| testes locais | 127.0.0.1 | testing123 |

### Configuração WiFi para dispositivos dos utilizadores

| Campo | Valor |
|-------|-------|
| Método EAP | EAP-TTLS |
| Fase 2 / Inner method | PAP |
| Certificado CA | UEM eduroam Certificate Authority |
| Identidade (username) | utilizador@uem.mz |
| Password | password do Keycloak |
