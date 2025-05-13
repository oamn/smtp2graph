# smtp2graph

A lightweight proxy that provides an SMTP endpoint and relays emails to Microsoft 365 via the Microsoft Graph API. Easily integrate legacy systems or scripts with modern Microsoft mail services.

## Features

- Simple SMTP server interface
- Relays mail to Microsoft 365 using Graph API
- Sentry error reporting (optional)

## Quick Start

To get started with smtp2graph:

1. Register a Microsoft Entra (Azure AD) application.
2. Set required environment variables.
3. Run the application using Docker.
4. Send mail via SMTP.

See the sections below for detailed instructions.

### Registering a Microsoft Entra App

To allow smtp2graph to relay emails via Microsoft 365, you must register an application in Microsoft Entra (Azure AD):

1. **Go to the Azure Portal:**
   - Visit https://portal.azure.com and sign in with an account that has permission to register applications.
2. **Register a new application:**
   - Navigate to **Azure Active Directory** > **App registrations** > **New registration**.
   - Enter a name (e.g., `smtp2graph`).
   - Set the supported account type (usually "Accounts in this organizational directory only").
   - Redirect URI is not required for client credentials flow.
   - Click **Register**.
3. **Configure API permissions:**
   - In your app registration, go to **API permissions** > **Add a permission** > **Microsoft Graph** > **Application permissions**.
   - Search for and add `Mail.Send`.
   - Click **Grant admin consent** for your tenant (requires admin privileges).
4. **Create a client secret:**
   - Go to **Certificates & secrets** > **New client secret**.
   - Add a description and choose an expiry period.
   - Click **Add** and copy the generated value. This is your `CLIENT_SECRET`.
5. **Collect required IDs:**
   - **CLIENT_ID:** Found on the app registration's **Overview** page as "Application (client) ID".
   - **TENANT_ID:** Found on the same page as "Directory (tenant) ID".
   - **CLIENT_SECRET:** The value you copied above.

**Note:**

- The app must have `Mail.Send` application permission (not delegated).
- Admin consent is required for application permissions.

3. **Set environment variables:**
   - `ENTRA_CLIENT_ID` (Microsoft Entra App registration client ID, required)
   - `ENTRA_TENANT_ID` (Microsoft Entra Directory/tenant ID, required)
   - `ENTRA_CLIENT_SECRET` (Microsoft Entra App registration client secret, required)
   - `SENDER_EMAIL` (Email address used as sender, required)
   - `SENDER_PASSWORD` (Password for the sender email, required)
   - `SMTP_SERVER_ADDR` (SMTP listen address, default: `:1025`)
   - `SMTP_SERVER_DOMAIN` (SMTP server domain, default: `localhost`)
   - `SMTP_MAX_MESSAGE_BYTES` (Maximum allowed message size in bytes, default: `10485760`)
   - `SMTP_MAX_RECIPIENTS` (Maximum allowed recipients per message, default: `50`)
   - `SMTP_WRITE_TIMEOUT` (Write timeout for SMTP connections, default: `10s`)
   - `SMTP_READ_TIMEOUT` (Read timeout for SMTP connections, default: `10s`)
   - `SENTRY_DSN` (Sentry DSN for error reporting, optional)

### Running with Docker

The recommended way to run smtp2graph is via Docker. You can use the published image from GitHub Container Registry:

```sh
docker run --rm \
  -e ENTRA_CLIENT_ID=... \
  -e ENTRA_TENANT_ID=... \
  -e ENTRA_CLIENT_SECRET=... \
  -e SENDER_EMAIL=... \
  -e SENDER_PASSWORD=... \
  -e SMTP_SERVER_ADDR=:1025 \
  -p 1025:1025 \
  ghcr.io/oamn/smtp2graph:latest
```

**Security Recommendation:**

For production deployments, it is strongly recommended to use implicit SSL/TLS (SMTPS, typically port 465) for all SMTP connections. You should use a TLS reverse proxy in front of smtp2graph to provide secure SMTP connections.

Set any additional environment variables as needed. Adjust port mapping if you change `SMTP_SERVER_ADDR`.

### Usage Example

Send an email using any SMTP client (e.g., `swaks`, `ncat`, or a script):

```sh
swaks --to user@example.com --from sender@example.com --server localhost:1025 --auth
```

## Local Development

To develop or test smtp2graph locally, you will need:

- **Go 1.24 or newer**
- **A Microsoft Entra (Azure AD) application** with Mail.Send permissions (see above for setup)

Clone this repository and set the required environment variables. You can then build and run the application directly:

```sh
go build -o smtp2graph .
./smtp2graph
```

To check the current version and build info:

```sh
./smtp2graph --version
```

To run all tests:

```sh
go test -v ./...
```

## License

MIT License. See `LICENSE` file.
