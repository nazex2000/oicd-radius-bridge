# OIDC-RADIUS Bridge

A bridge service that authenticates FreeRADIUS requests against an OIDC provider (like Keycloak).
![SSO UEM](https://github.com/user-attachments/assets/785a5f04-102c-42ee-b78a-e618e50e1932)

## Features

- OIDC authentication integration
- FreeRADIUS compatibility
- Docker support
- Secure local communication
- Detailed logging
- Environment-based configuration

## Requirements

- Go 1.24 or higher
- FreeRADIUS server
- OIDC provider (e.g., Keycloak)
- Python 3.x (for FreeRADIUS integration)
- Docker (optional)

## Installation

### Using Docker (Recommended)

1. Build the Docker image:
```bash
docker build -t oidc-radius-bridge .
```

2. Run the container:
```bash
docker run -d \
  --name oidc-radius-bridge \
  --network host \
  -v $(pwd)/.env:/app/.env \
  oidc-radius-bridge
```

### Manual Installation

1. Clone the repository:
```bash
git clone https://github.com/nazarioz/oidc-radius-bridge.git
cd oidc-radius-bridge
```

2. Install dependencies:
```bash
go mod download
```

3. Build the application:
```bash
go build -o oidc-radius-bridge ./cmd/server
```

4. Make the Python script executable:
```bash
chmod +x scripts/radius_auth.py
```

## Configuration

### Environment Variables

Create a `.env` file with the following variables:

```env
# OIDC Configuration
OIDC_PROVIDER_URL=https://your-keycloak-url/realms/your-realm
OIDC_CLIENT_ID=your-client-id
OIDC_CLIENT_SECRET=your-client-secret

# Logging
LOG_LEVEL=info  # debug, info, warn, error
```

### FreeRADIUS Configuration

1. Install the Python requests library:
```bash
pip install requests
```

2. Configure FreeRADIUS to use the authentication script:
```ini
# /etc/freeradius/3.0/mods-enabled/exec
exec {
    wait = yes
    program = "/path/to/scripts/radius_auth.py %{User-Name} %{User-Password}"
    input_pairs = request
    output_pairs = reply
    shell_escape = yes
}
```

3. Update the sites configuration:
```ini
# /etc/freeradius/3.0/sites-enabled/default
authorize {
    exec
    ...
}
```

## Usage

### Running the Service

1. Start the service:
```bash
./oidc-radius-bridge
```

2. The service will listen on `127.0.0.1:8080` for authentication requests from FreeRADIUS.

### Testing Authentication

Test the authentication endpoint:
```bash
curl -X POST http://127.0.0.1:8080/auth \
  -H "Content-Type: application/json" \
  -d '{"username":"user@example.com","password":"userpassword"}'
```

## Project Structure

```
.
├── cmd/
│   └── server/          # Main application entry point
├── internal/
│   ├── api/            # HTTP API handlers
│   ├── auth/           # Authentication logic
│   └── config/         # Configuration management
├── pkg/
│   └── logger/         # Logging utilities
├── scripts/
│   └── radius_auth.py  # FreeRADIUS integration script
├── .env               # Environment configuration
├── Dockerfile         # Docker configuration
└── README.md         # This file
```

## Security Considerations

1. **Local Communication**
   - Service only listens on localhost (127.0.0.1)
   - No external access required

2. **FreeRADIUS Security**
   - Uses FreeRADIUS's built-in security features
   - Configurable timeouts and retry limits
   - IP whitelisting support

3. **Docker Security**
   - Runs as non-root user
   - Minimal base image
   - No unnecessary dependencies

## Logging

The service uses structured logging with the following format:
```
[LEVEL] [TIMESTAMP] Message
```

Available log levels:
- DEBUG: Detailed information for debugging
- INFO: General operational information
- WARN: Warning messages
- ERROR: Error messages

## Support

For issues and feature requests, please create an issue in the GitHub repository.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
 
