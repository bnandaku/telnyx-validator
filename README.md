# telnyx-validator

An HTTP service that validates phone numbers using the [Telnyx Number Lookup API](https://developers.telnyx.com/docs/numbers/number-lookup). It performs a deep lookup — carrier, caller name (CNAM), and portability (LERG) — and rejects numbers that have no carrier or are VoIP.

## How it works

For each request the service makes two parallel Telnyx lookups:

1. **Carrier lookup** — resolves the carrier name, line type, and portability data (LRN, ported status, OCN, city/state)
2. **Caller name lookup** — resolves the subscriber name via CNAM

A number is considered **invalid** when:

| Rejection reason | Cause |
|---|---|
| `invalid_format` | Not a recognisable E.164 number |
| `not_found` | Telnyx has no record of the number |
| `no_carrier` | Lookup succeeded but no carrier resolved |
| `voip_number` | Carrier line type is `voip` |

## Requirements

- [Telnyx account](https://telnyx.com) with an API key
- Go 1.21+ **or** Docker

## Running locally

```bash
export TELNYX_API_KEY=your_api_key
go run .
```

The server listens on port `8080` by default. Override with the `PORT` environment variable:

```bash
PORT=9090 TELNYX_API_KEY=your_api_key go run .
```

## Running with Docker

### Build

```bash
docker build -t telnyx-validator .
```

### Run

```bash
docker run -p 8080:8080 -e TELNYX_API_KEY=your_api_key telnyx-validator
```

### docker-compose example

```yaml
services:
  validator:
    build: .
    ports:
      - "8080:8080"
    environment:
      TELNYX_API_KEY: your_api_key
      PORT: 8080
```

## API

### `POST /validate`

Validate a phone number via JSON body.

**Request**

```http
POST /validate
Content-Type: application/json

{
  "phone_number": "+15550001111"
}
```

The `phone_number` field accepts E.164 format. Common punctuation (spaces, dashes, parentheses, dots) is stripped automatically before validation, so all of the following are equivalent:

```
+15550001111
+1 555 000 1111
+1-555-000-1111
(555) 000-1111
```

**Response — valid number**

```json
{
  "phone_number": "+15550001111",
  "valid": true,
  "national_format": "555-000-1111",
  "country_code": "US",
  "carrier": {
    "name": "AT&T",
    "type": "mobile",
    "normalized_carrier": "AT&T",
    "mobile_country_code": "310",
    "mobile_network_code": "410",
    "error_code": ""
  },
  "caller_name": {
    "caller_name": "John Doe",
    "error_code": ""
  },
  "portability": {
    "lrn": "15550001111",
    "ported_status": "N",
    "ported_date": "",
    "ocn": "6529",
    "line_type": "mobile",
    "spid": "6529",
    "altspid": "",
    "city": "New York",
    "state": "NY"
  }
}
```

**Response — VoIP number**

```json
{
  "phone_number": "+15550002222",
  "valid": false,
  "rejection_reason": "voip_number",
  "carrier": {
    "name": "Twilio",
    "type": "voip",
    "normalized_carrier": "Twilio"
  },
  "portability": {
    "lrn": "15550002222",
    "ported_status": "N"
  }
}
```

**Response — invalid format**

```json
{
  "phone_number": "abc",
  "valid": false,
  "rejection_reason": "invalid_format"
}
```

---

### `GET /validate`

Same validation via query parameter — useful for quick testing.

```http
GET /validate?phone_number=%2B15550001111
```

Response shape is identical to `POST /validate`.

---

### `GET /health`

Health check endpoint.

```http
GET /health
```

```json
{ "status": "ok" }
```

---

## Response fields

| Field | Type | Description |
|---|---|---|
| `phone_number` | string | E.164 normalised number |
| `valid` | bool | `true` if the number passed all checks |
| `rejection_reason` | string | Populated when `valid` is `false` |
| `national_format` | string | Hyphen-formatted national number |
| `country_code` | string | ISO country code (e.g. `US`) |
| `carrier.name` | string | Carrier or SPID name |
| `carrier.type` | string | `mobile`, `landline`, `voip`, `toll free`, `unknown` |
| `carrier.normalized_carrier` | string | Primary network carrier |
| `carrier.mobile_country_code` | string | MCC (mobile numbers only) |
| `carrier.mobile_network_code` | string | MNC (mobile numbers only) |
| `caller_name.caller_name` | string | Subscriber name from CNAM database |
| `portability.lrn` | string | Local Routing Number |
| `portability.ported_status` | string | `Y` if ported, `N` otherwise |
| `portability.ported_date` | string | ISO date of last port |
| `portability.ocn` | string | Operating Company Name code |
| `portability.line_type` | string | LERG line type classification |
| `portability.spid` | string | Service Provider ID |
| `portability.city` | string | Rate centre city |
| `portability.state` | string | Rate centre state |

## Error responses

| Status | Body | Cause |
|---|---|---|
| `400` | `{"error": "phone_number is required"}` | Missing field |
| `400` | `{"error": "invalid JSON body"}` | Malformed request body |
| `405` | `{"error": "method not allowed"}` | Wrong HTTP method |
| `422` | `{"valid": false, "rejection_reason": "invalid_format"}` | Unparseable number |
| `502` | `{"error": "lookup failed: ..."}` | Telnyx API unreachable or error |

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `TELNYX_API_KEY` | Yes | — | Telnyx API key |
| `PORT` | No | `8080` | Port the server listens on |

## Dependencies

- [`github.com/bnandaku/telnyx-api`](https://github.com/bnandaku/telnyx-api) — Telnyx Go client

## License

MIT
