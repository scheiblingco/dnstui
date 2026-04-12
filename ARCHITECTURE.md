# DNSTUI Architecture

## Overview

DNSTUI is a terminal UI application written in Go for managing DNS records across multiple providers. It follows a layered architecture with a clear separation between configuration, provider business logic, and the user interface.

```
┌─────────────────────────────────────────────────────┐
│                   CLI (cmd/)                        │
│         Cobra commands + flag/env binding           │
└────────────────────┬────────────────────────────────┘
                     │ loads
┌────────────────────▼────────────────────────────────┐
│              Config (internal/config/)              │
│     YAML file · env vars · CLI flags (Viper)        │
└────────────────────┬────────────────────────────────┘
                     │ constructs
┌────────────────────▼────────────────────────────────┐
│          Provider Registry (internal/provider/)     │
│   interface · shared types · factory/registry       │
└──┬─────────────┬──────────────┬────────────────┬────┘
   │             │              │                │
┌──▼──┐    ┌─────▼──┐    ┌──────▼───┐    ┌──────▼────┐
│ CF  │    │Techni- │    │ ClouDNS  │    │Openprov-  │
│     │    │ tium   │    │          │    │ ider      │
└─────┘    └────────┘    └──────────┘    └───────────┘
           providers/
                     │ drives
┌────────────────────▼────────────────────────────────┐
│                TUI (internal/tui/)                  │
│           Bubble Tea model stack · Lipgloss          │
└─────────────────────────────────────────────────────┘
```

---

## Directory Structure

```
dnstui/
├── main.go                      # Entrypoint — calls cmd.Execute()
├── dnstui.example.yaml          # Annotated reference config file
├── go.mod / go.sum
│
├── cmd/                         # Cobra CLI layer
│   ├── root.go                  # Root command, PersistentPreRunE loads config
│   ├── version.go               # `dnstui version` subcommand
│   ├── providers.go             # `dnstui providers` — list configured providers
│   └── search.go                # `dnstui search` — launch GlobalSearch view
│
├── internal/                    # Private application packages
│   ├── config/                  # Configuration loading & validation
│   │   ├── config.go
│   │   └── config_test.go
│   │
│   ├── provider/                # Provider interface, shared types, registry
│   │   ├── types.go             # Account, Zone, Record, RecordType
│   │   ├── provider.go          # Provider interface definition
│   │   ├── registry.go          # Register() / New() / NewAll() factory
│   │   └── registry_test.go
│   │
│   ├── tui/                     # Bubble Tea models & views
│   │   ├── tui.go               # Root model, view stack, Run() / RunWithSearch()
│   │   ├── providerlist.go      # ProviderList + ZoneList views
│   │   ├── recordlist.go        # RecordList view (table)
│   │   ├── recordform.go        # RecordForm view (add/edit)
│   │   ├── confirmdialog.go     # ConfirmDialog overlay
│   │   └── globalsearch.go      # GlobalSearch view
│   │
│   ├── cache/                   # (Phase 6) TTL cache for accounts & zones
│   └── log/                     # (Phase 8) Structured logging setup
│
└── providers/                   # One package per DNS provider implementation
    ├── cloudflare/              # Cloudflare API client
    ├── technitium/              # Technitium DNS server API client
    ├── cloudns/                 # (Phase 7) ClouDNS API client
    └── openprovider/            # (Phase 7) Openprovider API client
```

---

## Packages

### `main`

Sole purpose: call `cmd.Execute()` and exit with a non-zero code on error. No logic lives here.

---

### `cmd/`

Built with [Cobra](https://github.com/spf13/cobra). The root command's `PersistentPreRunE` is the single location where `config.Load()` is called, ensuring the validated `*config.Config` is available to every subcommand.

**Subcommands:**

| Command | Description |
|---|---|
| _(no subcommand)_ | Launch the TUI (ProviderList entry point) |
| `version` | Print the build version |
| `providers` | List registered provider types and configured accounts |
| `search` | Launch the TUI directly at the GlobalSearch view |

**Flag → Viper binding:** `--config` / `-c` sets an explicit config file path; `--log-level` / `-l` overrides `log_level` and is bound to Viper so it participates in the full precedence chain.

**Build-time version injection:**
```
go build -ldflags "-X github.com/scheiblingco/dnstui/cmd.Version=1.2.3" .
```

---

### `internal/config/`

Owns the full configuration lifecycle: defaults → file → environment → CLI flags (increasing precedence, managed by Viper).

**Key types:**

```go
type Config struct {
    LogLevel  string           // debug | info | warn | error
    Providers []ProviderConfig // ordered list of provider accounts
    Cache     CacheConfig
}

type ProviderConfig struct {
    Name     string         // human-readable alias (shown in TUI)
    Type     string         // must match a registered provider type name
    Settings map[string]any // provider-specific credentials/endpoints
}

type CacheConfig struct {
    TTLSeconds int  // freshness window for account/zone list cache (default 300)
    DiskCache  bool // persist cache across sessions (default true)
}
```

**Config file discovery** (in order, first match wins):
1. Path given via `--config` flag
2. `$HOME/.config/dnstui/dnstui.yaml`
3. `$XDG_CONFIG_HOME/dnstui/dnstui.yaml`
4. `./dnstui.yaml` (current directory)

**Environment variable mapping:** Prefix `DNSTUI_`, dots replaced with underscores.  
Examples: `DNSTUI_LOG_LEVEL=debug`, `DNSTUI_CACHE_TTL_SECONDS=60`.

**Validation rules enforced by `Config.Validate()`:**
- `log_level` must be one of `debug`, `info`, `warn`, `error`
- Every provider entry must have a non-empty `name` and `type`
- `cache.ttl_seconds` must be ≥ 0

---

### `internal/provider/`

Defines the contract that all provider implementations must fulfil, plus the shared data model and the self-registration registry.

#### Provider Interface

```go
type Provider interface {
    ProviderName() string   // stable lowercase type key, e.g. "cloudflare"
    FriendlyName() string   // user-configured alias from ProviderConfig.Name

    ListAccounts(ctx context.Context) ([]Account, error)
    ListZones(ctx context.Context, accountID string) ([]Zone, error)
    ListRecords(ctx context.Context, zoneID string) ([]Record, error)
    CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error)
    UpdateRecord(ctx context.Context, zoneID, recordID string, r Record) (Record, error)
    DeleteRecord(ctx context.Context, zoneID, recordID string) error
}
```

All list methods must return an empty (non-nil) slice when there are no results.

#### Shared Types

| Type | Purpose |
|---|---|
| `Account` | Top-level account/sub-account within a provider |
| `Zone` | A DNS zone (domain), linked to an `Account` by `AccountID` |
| `Record` | A single DNS resource record |
| `RecordType` | Typed string constant: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `SRV`, `CAA`, `PTR`, `SOA`, `TLSA`, `SSHFP`, `NAPTR` |
| `SearchEntry` | A flattened account-or-domain item used by the global search index |
| `SearchEntryKind` | Typed string: `"account"` or `"domain"` |

`Record.Extra map[string]any` carries provider-specific fields that don't fit the common model (e.g. Cloudflare's `proxied` flag, Technitium `comments`). Keys are lower-snake-case strings.

#### Provider Registry

Providers self-register in their package `init()` function:

```go
// Inside providers/cloudflare/cloudflare.go
func init() {
    provider.Register("cloudflare", New)
}
```

The registry maps type name → `Constructor` (`func(config.ProviderConfig) (Provider, error)`).

`provider.NewAll(cfgs []config.ProviderConfig)` is called at startup to instantiate all configured providers; it fails fast on the first construction error.

#### Search Cache

`cache.go` provides `BuildSearchCache(ctx, providers)` which walks every provider's accounts and zones and returns a flat `[]SearchEntry` slice. The root TUI model calls this as a background `tea.Cmd` immediately on startup (via `Model.Init()`). Each entry carries its `Kind` (`"account"` or `"domain"`), a pre-formatted display `Label`, a reference to the owning `Provider`, and either the `Account` or `Zone` value.

When the background load completes, a `CacheLoadedMsg` is dispatched to the root model, which stores the entries and — if the `GlobalSearch` view is already open — updates it live.

---

### `providers/<name>/`

Each directory is a self-contained Go package implementing the `provider.Provider` interface for one DNS provider. Provider packages have no cross-dependencies — they only import `internal/provider` (for shared types) and `internal/config` (for `ProviderConfig`).

| Package | Type key | Auth method | Status |
|---|---|---|---|
| `providers/cloudflare` | `cloudflare` | API token or API key + email | ✅ Complete |
| `providers/technitium` | `technitium` | API key, HTTP header | ✅ Complete |
| `providers/cloudns` | `cloudns` | auth-id + password (or sub-auth) | Phase 7 |
| `providers/openprovider` | `openprovider` | API token | Phase 7 |

Provider-specific settings are decoded from `ProviderConfig.Settings` (a `map[string]any`) into a typed struct at construction time using `github.com/go-viper/mapstructure/v2`.

#### Cloudflare provider (`providers/cloudflare/`)

Five files:

| File | Purpose |
|---|---|
| `cloudflare.go` | `Settings` struct, `New()` constructor, `ProviderName()`/`FriendlyName()`, `init()` registration |
| `apitypes.go` | CF API response types: `cfResponse[T]`, `cfAccount`, `cfZone`, `cfRecord`, `cfRecordRequest`, `apiErrors()` |
| `client.go` | `newRequest()`, `doRequest()` (3-attempt backoff on 5xx), `getAllPages[T]()`, `doJSON[T]()` |
| `records.go` | All six `Provider` interface methods |
| `mapping.go` | `toRecord()` (CF → shared), `toRequest()` (shared → CF) |

**Auth:** `Authorization: Bearer <token>` (preferred) or legacy `X-Auth-Key` + `X-Auth-Email`.

**Pagination:** all list endpoints are fully consumed via `getAllPages` (100 results/page).

**Cloudflare-specific extras** stored in `Record.Extra`:
- `proxied` (bool) — CDN proxy status
- `proxiable` (bool) — whether the record type supports proxying
- `comment` (string) — optional record note
- `data` (string, JSON) — raw structured data for SRV/CAA/TLSA/SSHFP/NAPTR records

**TTL convention:** CF TTL `1` ("automatic") maps to provider TTL `0` and vice versa.

**`base_url` setting** can override the API base URL — used by integration tests (`httptest.NewServer`).

---

#### Technitium provider (`providers/technitium/`)

Single file `technitium.go` containing all logic.

| Section | Notes |
|---|---|
| **Auth** | API token appended as `?token=<key>` to every request URL |
| **Accounts** | Synthesised from `GET /api/user/session/get` — Technitium has no sub-account concept |
| **Zones** | `GET /api/zones/list` — returns all zone types |
| **Records** | `GET /api/zones/records/get?zone=<name>` |
| **Create** | `GET /api/zones/records/add` with type-specific query params |
| **Update** | `GET /api/zones/records/update` — passes `oldValue` for overwrite targeting |
| **Delete** | `GET /api/zones/records/delete` |
| **Record ID** | Composite string `"name\x00type\x00value"` (null-byte delimited) — round-trips through update/delete |
| **Extras** | `comment` (string), `disabled` (bool), `weight`+`port` (SRV), `caa_flags`+`caa_tag` (CAA) |

---

### `internal/tui/`

Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea). Navigation follows a **model stack**: each view is a `tea.Model` pushed onto `Model.stack`; `Esc` pops back to the previous view. Two entry points:
- `tui.Run(providers)` — starts at `ProviderList`
- `tui.RunWithSearch(providers)` — starts at `GlobalSearch` (used by `dnstui search`)

All provider API calls are dispatched as `tea.Cmd` (non-blocking). A `spinner` component is shown while loading.

**Shared messages** (defined in `tui.go`):

| Message | Direction |
|---|---|
| `PushMsg{Model}` | Child pushes new view onto stack |
| `PopMsg{}` | Child signals "go back" |
| `ErrorMsg{Err}` | Any view surfaces an error to the status bar |
| `StatusMsg{Text}` | Any view surfaces a success note to the status bar |
| `AccountsLoadedMsg` | Async accounts response |
| `ZonesLoadedMsg` | Async zones response |
| `RecordsLoadedMsg` | Async records response |
| `RecordSavedMsg` | Create/update completed |
| `RecordDeletedMsg` | Delete completed |
| `CacheLoadedMsg` | Startup search-cache background load completed |

**Views:**

| File | View | Description |
|---|---|---|
| `providerlist.go` | `ProviderList` | Filterable list of all configured providers; loading spinner while fetching accounts |
| `providerlist.go` | `ZoneList` | Filterable list of zones; `Tab` cycles accounts for multi-account providers |
| `recordlist.go` | `RecordList` | Table of records (`n` new, `e` edit, `d` delete, `r` refresh); delegates CRUD to form/dialog |
| `recordform.go` | `RecordForm` | Per-field text inputs for add/edit; `ctrl+s` to save; client-side validation before submit |
| `confirmdialog.go` | `ConfirmDialog` | Generic yes/no overlay; executes any `tea.Cmd` on confirmation |
| `globalsearch.go` | `GlobalSearch` | Ctrl+K modal showing all cached accounts and domains across providers; filters as you type; Enter navigates to the selected account's ZoneList or domain's RecordList |

**Key bindings (global):** `ctrl+c` quit | `ctrl+k` open global search | `esc` back | `r` refresh | `n` new | `e` edit | `d` delete | `tab`/`↑↓` navigate | `/` filter.

---

### `internal/cache/` _(not used — caching is handled by `internal/provider/cache.go`)_

In-memory TTL cache for `Account` and `Zone` lists (keyed by provider + account ID). When `DiskCache` is enabled, the cache is serialised to `$XDG_CACHE_HOME/dnstui/cache.json` on exit and reloaded on startup. Cache can be manually invalidated with the `r` keybind in the TUI.

---

### `internal/log/` _(Phase 8)_

Thin wrapper around Go's standard `log/slog`. Configures the handler (text or JSON) and level at startup from `Config.LogLevel`. All packages receive a `*slog.Logger` via function arguments — no package-level global logger.

---

## Configuration Precedence

```
CLI flags  >  environment variables  >  config file  >  built-in defaults
```

All four layers are managed by a single `*viper.Viper` instance created in `cmd/root.go` and passed to `config.Load()`.

---

## Adding a New Provider

1. Create `providers/<name>/<name>.go`.
2. Define a settings struct and implement all methods of `provider.Provider`.
3. Register in `init()`:
   ```go
   func init() {
       provider.Register("<name>", New)
   }
   ```
4. Blank-import the package in `cmd/root.go` (or `main.go`) to trigger `init()`:
   ```go
   import _ "github.com/scheiblingco/dnstui/providers/<name>"
   ```
5. Add an example stanza to `dnstui.example.yaml`.

---

## Key Dependencies

| Module | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI command/flag parsing |
| `github.com/spf13/viper` | Config file + env var + flag merging |
| `github.com/charmbracelet/bubbletea` | TUI event loop & model framework |
| `github.com/charmbracelet/bubbles` | Prebuilt TUI components |
| `github.com/charmbracelet/lipgloss` | Terminal styling |
| `github.com/go-viper/mapstructure/v2` | Settings map → typed struct decoding |
| `log/slog` (stdlib) | Structured logging |

---

## Implementation Phases

| Phase | Scope | Status |
|---|---|---|
| 1 | Project scaffold, module init, config package, Cobra entrypoint | ✅ Complete |
| 2 | Provider interface, shared types, registry/factory | ✅ Complete |
| 3 | Cloudflare provider | ✅ Complete |
| 4 | Technitium provider | ✅ Complete |
| 5 | TUI (Bubble Tea) — all views | ✅ Complete |
| 6 | Caching (TTL + disk) | Planned |
| 7 | ClouDNS and Openprovider | Planned |
| 8 | Error handling, logging, input validation | Planned |
| 9 | Unit + integration tests, documentation | Planned |
