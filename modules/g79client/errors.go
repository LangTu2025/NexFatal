package g79client

import (
	"errors"
	"strings"
)

// ErrClientVersionTooLow identifies a server rejection caused by an outdated
// game client. It is also recognized by IsClientVersionTooLowError when the
// server has returned a plain-text error instead of a wrapped sentinel.
var ErrClientVersionTooLow = errors.New("g79client: client version too low")

// IsClientVersionTooLowError reports whether err means that the game client
// version is below the version required by the server.
//
// Authentication responses currently expose this condition as ordinary text
// (usually code 2001), so callers must be able to classify both wrapped
// sentinel errors and the existing response text.
func IsClientVersionTooLowError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrClientVersionTooLow) {
		return true
	}

	// Walk both ordinary wrapping and errors.Join chains. The latter matters
	// when an authentication failure is combined with request diagnostics.
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if IsClientVersionTooLowError(cause) {
				return true
			}
		}
		return false
	}
	if cause := errors.Unwrap(err); cause != nil {
		if IsClientVersionTooLowError(cause) {
			return true
		}
	}

	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"客户端版本过低",
		"客户端版本低于",
		"版本过低",
		"版本太低",
		"版本过旧",
		"请升级客户端",
		"client version too low",
		"client version is too low",
		"client version is outdated",
		"client version outdated",
		"client version lower",
		"client version below",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}

	// PE authentication uses code 2001 for this rejection. Remove whitespace so
	// both text and JSON gateway responses are handled consistently.
	compactMessage := strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "").Replace(message)
	return strings.Contains(compactMessage, "code:2001") ||
		strings.Contains(compactMessage, "code=2001") ||
		strings.Contains(compactMessage, `"code":2001`)
}
