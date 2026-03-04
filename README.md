# EVM Firehose Tracer (Go)

A chain-agnostic EVM execution tracer that produces [Firehose](https://firehose.streamingfast.io/) protobuf blocks for blockchain indexing and analytics.

## Overview

This repository contains a **shared tracer implementation** that can be integrated into any EVM-compatible blockchain client (go-ethereum, Erigon, BSC, Polygon, etc.). The tracer captures detailed execution data—including state changes, calls, logs, and gas consumption—and outputs structured protobuf blocks for downstream processing.

### Key Features

- **Chain-agnostic**: Core tracer has zero dependencies on specific blockchain implementations
- **Protocol v3.0**: Latest Firehose protocol with full EIP support (EIP-1559, EIP-4844, EIP-7702, etc.)
- **Parallel execution support**: Handles concurrent transaction tracing with ordinal reordering
- **Comprehensive state tracking**: Balance changes, nonce changes, code changes, storage changes, gas changes, logs
- **System call support**: Chain-specific system calls (e.g., Beacon root, withdrawals)
- **Genesis block handling**: Synthetic transaction for genesis allocation

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Blockchain Client (go-ethereum, Erigon, etc.)              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Chain-Specific Adapter                                │ │
│  │  - Converts client types to shared types              │ │
│  │  - Implements StateReader interface                    │ │
│  │  - Computes chain-specific values (e.g., logsBloom)   │ │
│  └─────────────────────┬──────────────────────────────────┘ │
│                        │                                     │
│                        ▼                                     │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Shared Tracer (this repository)                      │ │
│  │  - Chain-agnostic core                                │ │
│  │  - Lifecycle hooks (OnBlockStart, OnTxStart, etc.)    │ │
│  │  - State change tracking                              │ │
│  │  - Ordinal management                                 │ │
│  │  - Protobuf serialization                             │ │
│  └─────────────────────┬──────────────────────────────────┘ │
└────────────────────────┼────────────────────────────────────┘
                         │
                         ▼
              Firehose Protobuf Output
           (sf.ethereum.type.v2.Block)
```

## Installation

```bash
go get github.com/streamingfast/evm-firehose-tracer-go
```

## Usage

### Basic Integration

```go
import (
    firehose "github.com/streamingfast/evm-firehose-tracer-go"
)

// 1. Create tracer
tracer := firehose.NewTracer(&firehose.Config{
    OutputWriter: os.Stdout,
})

// 2. Initialize with chain config
tracer.OnBlockchainInit("geth", "v1.14.0", &firehose.ChainConfig{
    ChainID: big.NewInt(1),
    // ... fork times
})

// 3. Trace blocks
tracer.OnBlockStart(blockEvent)

// 4. Trace transactions
tracer.OnTxStart(txEvent, stateReader)
tracer.OnCallEnter(depth, typ, from, to, input, gas, value)
// ... state changes (OnBalanceChange, OnStorageChange, etc.)
tracer.OnCallExit(depth, output, gasUsed, err, reverted)
tracer.OnTxEnd(receiptData, err)

tracer.OnBlockEnd(nil)

// 5. Cleanup
tracer.OnClose()
```

### Chain-Specific Adapter Example

See `go-ethereum/eth/tracers/firehose_adapters.go` for a reference implementation that:
- Converts go-ethereum types to shared tracer types
- Implements `StateReader` interface using `StateDB`
- Maps balance/gas change reasons to protobuf enums

## Testing

```bash
# Run all tests
go test ./...

# Run specific test suite
go test ./tests -v -run TestTracer_ReceiptLogsBloom

# Run with debug logging
FIREHOSE_ETHEREUM_TRACER_LOG_LEVEL=trace go test ./tests -v -run TestName
```

## Key Concepts

### Lifecycle Hooks

The tracer follows a strict lifecycle with hooks for each phase:

1. **Blockchain Init**: `OnBlockchainInit(nodeName, version, chainConfig)`
2. **Block Lifecycle**:
   - `OnBlockStart(event)` → `OnTxStart(...)` → ... → `OnTxEnd(...)` → `OnBlockEnd(err)`
3. **Transaction Lifecycle**:
   - `OnTxStart(event, stateReader)` → `OnCallEnter(...)` → ... → `OnCallExit(...)` → `OnTxEnd(receipt, err)`
4. **Call Lifecycle**:
   - `OnCallEnter(depth, typ, from, to, ...)` → state changes → `OnCallExit(depth, output, ...)`

### State Changes

All state changes are recorded with ordinals for deterministic ordering:

- **Balance Changes**: `OnBalanceChange(addr, prev, new, reason)`
- **Nonce Changes**: `OnNonceChange(addr, prev, new)`
- **Code Changes**: `OnCodeChange(addr, prevHash, newHash, prevCode, newCode)`
- **Storage Changes**: `OnStorageChange(addr, key, prev, new)`
- **Gas Changes**: `OnGasChange(oldGas, newGas, reason)`
- **Logs**: `OnLog(addr, topics, data, blockIndex)`

### Ordinals

Ordinals provide deterministic ordering of all events within a block:
- Every state change, call, and log receives a monotonically increasing ordinal
- Enables precise reconstruction of execution order
- Critical for parallel execution with reordering

### Parallel Execution

The tracer supports parallel transaction execution:

```go
// Coordinator tracer
coordinator := firehose.NewTracer(config)
coordinator.OnBlockStart(blockEvent)

// Spawn isolated tracers
isolated1 := coordinator.OnTxSpawn(0)
isolated2 := coordinator.OnTxSpawn(1)

// Execute in parallel
go executeTransaction(isolated1, tx1)
go executeTransaction(isolated2, tx2)

// Commit in order
coordinator.OnTxCommit(isolated1)
coordinator.OnTxCommit(isolated2)

coordinator.OnBlockEnd(nil)
```

## Integration Examples

### go-ethereum

This repository includes a complete go-ethereum integration in `go-ethereum/eth/tracers/`:
- `firehose.go`: Tracer registration and hooks setup
- `firehose_adapters.go`: Type conversions and state reader implementation

## License

Apache 2.0

## Resources

- [Firehose Documentation](https://firehose.streamingfast.io/)
- [Protobuf Definitions](https://github.com/streamingfast/firehose-ethereum/tree/develop/types/pb)
- [StreamingFast](https://www.streamingfast.io/)
