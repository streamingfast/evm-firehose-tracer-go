package firehose

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Tracer debugging levels (controlled via environment variable)
// Here what you can expect from the debugging levels:
// - Info == block start/end + trx start/end
// - Debug == Info + call start/end + error
// - Trace == Debug + state db changes, log, balance, nonce, code, storage, gas
// - TraceFull == Trace + opcode
var tracerLogLevel = strings.ToLower(os.Getenv("FIREHOSE_ETHEREUM_TRACER_LOG_LEVEL"))
var isInfoEnabled = tracerLogLevel == "info" || tracerLogLevel == "debug" || tracerLogLevel == "trace" || tracerLogLevel == "trace_full"
var isDebugEnabled = tracerLogLevel == "debug" || tracerLogLevel == "trace" || tracerLogLevel == "trace_full"
var isTraceEnabled = tracerLogLevel == "trace" || tracerLogLevel == "trace_full"
var isTraceFullEnabled = tracerLogLevel == "trace_full"

// ============================================================================
// Logging with Deferred Evaluation (Performance-Critical)
// ============================================================================
// These functions check if logging is enabled BEFORE formatting arguments.
// This ensures zero overhead in production where logging is typically disabled.

// firehoseInfo logs info-level messages (block/tx start/end)
func firehoseInfo(msg string, args ...interface{}) {
	if isInfoEnabled {
		fmt.Fprintf(os.Stderr, "[Firehose] "+msg+"\n", args...)
	}
}

// firehoseDebug logs debug-level messages (call start/end, errors)
func firehoseDebug(msg string, args ...interface{}) {
	if isDebugEnabled {
		fmt.Fprintf(os.Stderr, "[Firehose] "+msg+"\n", args...)
	}
}

// firehoseTrace logs trace-level messages (state changes, gas, etc.)
func firehoseTrace(msg string, args ...interface{}) {
	if isTraceEnabled {
		fmt.Fprintf(os.Stderr, "[Firehose] "+msg+"\n", args...)
	}
}

// firehoseTraceFull logs full trace-level messages (opcodes)
func firehoseTraceFull(msg string, args ...interface{}) {
	if isTraceFullEnabled {
		fmt.Fprintf(os.Stderr, "[Firehose] "+msg+"\n", args...)
	}
}

// ============================================================================
// View Types for Deferred String Formatting (Performance-Critical)
// ============================================================================
// These types implement fmt.Stringer and defer expensive operations
// (hex encoding, string conversion) until the log is actually printed.
//
// This is critical for performance because:
// 1. Logging is typically disabled in production
// 2. Without deferred evaluation, we'd waste CPU on string conversions
// 3. With deferred evaluation, zero overhead when logging is off

// _errorView defers error formatting until String() is called
type _errorView struct {
	err error
}

func errorView(err error) _errorView {
	return _errorView{err}
}

func (e _errorView) String() string {
	if e.err == nil {
		return "<no error>"
	}
	return `"` + e.err.Error() + `"`
}

// _shortAddressView defers address shortening until String() is called
type _shortAddressView [20]byte

//go:inline
func shortAddressView(addr *[20]byte) *_shortAddressView {
	return (*_shortAddressView)(addr)
}

func (a *_shortAddressView) String() string {
	if a == nil {
		return "<nil>"
	}
	return shortenAddress((*[20]byte)(a))
}

func shortenAddress(addr *[20]byte) string {
	full := hex.EncodeToString(addr[:])
	if len(full) < 10 {
		return full
	}
	return "0x" + full[:4] + ".." + full[len(full)-4:]
}

// inputView defers input data formatting until String() is called
type inputView []byte

func (b inputView) String() string {
	if len(b) == 0 {
		return "<empty>"
	}
	if len(b) < 4 {
		return hex.EncodeToString(b)
	}

	method := b[:4]
	rest := b[4:]

	if len(rest)%32 == 0 {
		return fmt.Sprintf("%s (%d params)", hex.EncodeToString(method), len(rest)/32)
	}

	return fmt.Sprintf("%d bytes", len(b))
}

// outputView defers output data formatting until String() is called
type outputView []byte

func (b outputView) String() string {
	if len(b) == 0 {
		return "<empty>"
	}
	return fmt.Sprintf("%d bytes", len(b))
}
