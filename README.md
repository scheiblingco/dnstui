# DNSTUI
DNSTUI is a terminal UI for managing DNS records. It provides an easy-to-use interface for adding, editing, and deleting records with various providers.

## Overview

### Configuration
Configuration is 12-factor compliant and can be set via environment variables, CLI flags or a configuration file. The application supports multiple providers, and you can specify the provider to use for each record.

### Providers
DNSTUI has a modular drop-in system for providers. The initial implementation should support:
- Cloudflare
- ClouDNS
- Openprovider
- Technitium

### Features
- Add, edit, and delete DNS records
- Support for multiple providers
- User-friendly terminal interface
- Configuration via environment variables, CLI flags, or configuration file
- Validation of input data
- Error handling and logging
- **Global search** (`Ctrl+K` from any screen) — instantly filter and navigate to any account or domain across all providers
- Startup caching of accounts and domains for fast, responsive search

### Cloudflare
- Support for multiple accounts with a single or multiple logins
- Support for multiple zones per account
- Support for all record types (A, AAAA, CNAME, MX, TXT, etc.)
- Support for Cloudflare's API features (e.g., TTL, proxied status)
- Support for Cloudflare's authentication methods (API token, API key)

### ClouDNS
- Support for multiple accounts
- Support for all record types (A, AAAA, CNAME, MX, TXT, etc.)
- Support for ClouDNS's API features (e.g., TTL, priority for MX records)
- Support for ClouDNS's authentication methods (API key)

### Openprovider
- Support for multiple accounts
- Support for all record types (A, AAAA, CNAME, MX, TXT, etc.)
- Support for Openprovider's API features (e.g., TTL, priority for MX records)
- Support for Openprovider's authentication methods (API key)


### Technitium
- Support for multiple connections and accounts
- Support for all record types (A, AAAA, CNAME, MX, TXT, etc.)
- Support for Technitium's API features (e.g., TTL, priority for MX records)
- Support for Technitium's authentication methods (API key)


### Plan suggestion
1. Implement the core application structure and configuration management
2. Implement the provider interface and create a base provider class
3. Implement the Cloudflare provider
4. Implement the TUI for managing DNS records
5. Implement the global search
6. Implement caching for accounts and domains     
7. Drop in the remaining providers (ClouDNS, Openprovider, Technitium)
8. Add error handling, logging, and input validation
9. Write tests and documentation