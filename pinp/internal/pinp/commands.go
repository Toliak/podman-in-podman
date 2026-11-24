package pinp

import "strings"

const (
	CommandNone    = ""
	CommandCheck   = "check"
	CommandPrepare = "prepare"
)

// pinpCommandTable is the single dispatch table for pinp-only subcommands.
// Canonical names all use the reserved pinp- prefix so they can never collide
// with a real podman subcommand; prepare/gen-sub are kept as bare aliases for
// the harness and CLI ergonomics. Bare "check" is deliberately absent: it is
// broken in favor of pinp-check so a real podman "check" is never hijacked.
var pinpCommandTable = map[string]string{
	"pinp-check":   CommandCheck,
	"pinp-prepare": CommandPrepare,
	"pinp-gen-sub": CommandPrepare,
	"prepare":      CommandPrepare,
	"gen-sub":      CommandPrepare,
}

// PinpCommand returns the pinp-only command id for sub, or CommandNone when
// sub is an ordinary podman subcommand.
func PinpCommand(sub string) string {
	return pinpCommandTable[sub]
}

// IsReservedPinpName reports whether sub claims the reserved pinp- namespace.
// Such a token that is not in pinpCommandTable is a typo and must be rejected
// rather than forwarded to podman (which would fail anyway).
func IsReservedPinpName(sub string) bool {
	return strings.HasPrefix(sub, "pinp-")
}

// KnownPinpCommands returns the canonical dispatch names in a stable order,
// for use in the "unknown pinp subcommand" error message.
func KnownPinpCommands() []string {
	return []string{"pinp-check", "pinp-prepare", "pinp-gen-sub"}
}
