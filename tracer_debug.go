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

// opCodeView defers opcode name lookup until String() is called
type opCodeView byte

func (o opCodeView) String() string {
	if name, ok := opCodeNames[byte(o)]; ok {
		return name
	}
	return fmt.Sprintf("0x%02x", byte(o))
}

// opCodeNames maps EVM opcode bytes to their human-readable names.
// Covers all opcodes defined in the Yellow Paper / EVM spec.
var opCodeNames = map[byte]string{
	// Stop and Arithmetic
	0x00: "STOP",
	0x01: "ADD",
	0x02: "MUL",
	0x03: "SUB",
	0x04: "DIV",
	0x05: "SDIV",
	0x06: "MOD",
	0x07: "SMOD",
	0x08: "ADDMOD",
	0x09: "MULMOD",
	0x0a: "EXP",
	0x0b: "SIGNEXTEND",
	// Comparison & Bitwise Logic
	0x10: "LT",
	0x11: "GT",
	0x12: "SLT",
	0x13: "SGT",
	0x14: "EQ",
	0x15: "ISZERO",
	0x16: "AND",
	0x17: "OR",
	0x18: "XOR",
	0x19: "NOT",
	0x1a: "BYTE",
	0x1b: "SHL",
	0x1c: "SHR",
	0x1d: "SAR",
	// SHA3
	0x20: "KECCAK256",
	// Environmental Information
	0x30: "ADDRESS",
	0x31: "BALANCE",
	0x32: "ORIGIN",
	0x33: "CALLER",
	0x34: "CALLVALUE",
	0x35: "CALLDATALOAD",
	0x36: "CALLDATASIZE",
	0x37: "CALLDATACOPY",
	0x38: "CODESIZE",
	0x39: "CODECOPY",
	0x3a: "GASPRICE",
	0x3b: "EXTCODESIZE",
	0x3c: "EXTCODECOPY",
	0x3d: "RETURNDATASIZE",
	0x3e: "RETURNDATACOPY",
	0x3f: "EXTCODEHASH",
	// Block Information
	0x40: "BLOCKHASH",
	0x41: "COINBASE",
	0x42: "TIMESTAMP",
	0x43: "NUMBER",
	0x44: "PREVRANDAO",
	0x45: "GASLIMIT",
	0x46: "CHAINID",
	0x47: "SELFBALANCE",
	0x48: "BASEFEE",
	0x49: "BLOBHASH",
	0x4a: "BLOBBASEFEE",
	// Stack, Memory, Storage and Flow Operations
	0x50: "POP",
	0x51: "MLOAD",
	0x52: "MSTORE",
	0x53: "MSTORE8",
	0x54: "SLOAD",
	0x55: "SSTORE",
	0x56: "JUMP",
	0x57: "JUMPI",
	0x58: "PC",
	0x59: "MSIZE",
	0x5a: "GAS",
	0x5b: "JUMPDEST",
	0x5c: "TLOAD",
	0x5d: "TSTORE",
	0x5e: "MCOPY",
	0x5f: "PUSH0",
	// Push Operations
	0x60: "PUSH1",
	0x61: "PUSH2",
	0x62: "PUSH3",
	0x63: "PUSH4",
	0x64: "PUSH5",
	0x65: "PUSH6",
	0x66: "PUSH7",
	0x67: "PUSH8",
	0x68: "PUSH9",
	0x69: "PUSH10",
	0x6a: "PUSH11",
	0x6b: "PUSH12",
	0x6c: "PUSH13",
	0x6d: "PUSH14",
	0x6e: "PUSH15",
	0x6f: "PUSH16",
	0x70: "PUSH17",
	0x71: "PUSH18",
	0x72: "PUSH19",
	0x73: "PUSH20",
	0x74: "PUSH21",
	0x75: "PUSH22",
	0x76: "PUSH23",
	0x77: "PUSH24",
	0x78: "PUSH25",
	0x79: "PUSH26",
	0x7a: "PUSH27",
	0x7b: "PUSH28",
	0x7c: "PUSH29",
	0x7d: "PUSH30",
	0x7e: "PUSH31",
	0x7f: "PUSH32",
	// Duplication Operations
	0x80: "DUP1",
	0x81: "DUP2",
	0x82: "DUP3",
	0x83: "DUP4",
	0x84: "DUP5",
	0x85: "DUP6",
	0x86: "DUP7",
	0x87: "DUP8",
	0x88: "DUP9",
	0x89: "DUP10",
	0x8a: "DUP11",
	0x8b: "DUP12",
	0x8c: "DUP13",
	0x8d: "DUP14",
	0x8e: "DUP15",
	0x8f: "DUP16",
	// Exchange Operations
	0x90: "SWAP1",
	0x91: "SWAP2",
	0x92: "SWAP3",
	0x93: "SWAP4",
	0x94: "SWAP5",
	0x95: "SWAP6",
	0x96: "SWAP7",
	0x97: "SWAP8",
	0x98: "SWAP9",
	0x99: "SWAP10",
	0x9a: "SWAP11",
	0x9b: "SWAP12",
	0x9c: "SWAP13",
	0x9d: "SWAP14",
	0x9e: "SWAP15",
	0x9f: "SWAP16",
	// Logging Operations
	0xa0: "LOG0",
	0xa1: "LOG1",
	0xa2: "LOG2",
	0xa3: "LOG3",
	0xa4: "LOG4",
	// System Operations
	0xf0: "CREATE",
	0xf1: "CALL",
	0xf2: "CALLCODE",
	0xf3: "RETURN",
	0xf4: "DELEGATECALL",
	0xf5: "CREATE2",
	0xfa: "STATICCALL",
	0xfd: "REVERT",
	0xfe: "INVALID",
	0xff: "SELFDESTRUCT",
}
