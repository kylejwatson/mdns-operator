# Local patches

Vendored from `github.com/brutella/dnssd` v1.2.14 (without `cmd/`) and wired in with a `replace` directive in the root `go.mod`.

## Hostname queries honour the query type

`handleQuestion` in `responder.go` answered every hostname query with all of the host's addresses, whatever type was asked for. An AAAA query for an IPv4-only host got an A record as its answer and an NSEC in the additional section. Strict clients treat that as unanswered and keep retrying, which stalled each new connection from an LG webOS TV's media player by about 5 seconds.

The answer section now holds only the queried type, or an NSEC if the host has none, and the other addresses go in the additional section. `responder_qtype_test.go` covers this.

Remove this directory and the `replace` directive if upstream gains the same behaviour.
