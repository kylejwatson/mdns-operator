# Local patches

Vendored from `github.com/brutella/dnssd` v1.2.14 (without `cmd/`) and wired in with a `replace` directive in the root `go.mod`.

## Hostname queries honour the query type

`handleQuestion` in `responder.go` answered every hostname query with all of the host's addresses, whatever type was asked for. An AAAA query for an IPv4-only host got an A record as its answer and an NSEC in the additional section. That reply doesn't say the AAAA record is missing, so clients can't use it as a negative answer. It was changed while investigating 5 second gaps between connections from an LG webOS TV's media player, but the change didn't remove them: the TV keeps retrying AAAA even with the corrected reply.

The answer section now holds only the queried type, or an NSEC if the host has none, and the other addresses go in the additional section. `responder_qtype_test.go` covers this.

## Optional IPv4-mapped AAAA records

`Config.MappedAAAA` publishes each IPv4 address as an IPv4-mapped AAAA record as well (for example `::ffff:192.168.69.51`). An LG webOS TV's media player waits about 5 seconds per connection when a `.local` name has no AAAA record, and doesn't accept an NSEC as the answer. This is a workaround and is off by default.

Remove this directory and the `replace` directive if upstream gains the same behaviour.
