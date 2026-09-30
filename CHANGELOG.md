# Change log

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased (v5.0.0)

### Added

* `firehoseTraceFull` opcode logging: `OnOpcode` and `OnOpcodeFault` now emit trace-full-level log lines (enabled via `FIREHOSE_ETHEREUM_TRACER_LOG_LEVEL=trace_full`) including the opcode name, gas, cost, and error. All standard EVM opcodes (including Cancun additions) resolve to their human-readable name (e.g. `CALL`, `SSTORE`); unknown opcodes fall back to `0x??` hex.
* `FlashBlockData.IsFinal` flag to mark the final flash block iteration for a block. When set, the emitted `FIRE BLOCK` line encodes the flash block index as `Idx + 1000` (partials 1..9 emit as 1..9, the final 10th partial emits as 1010), matching the Optimism Geth firehose tracer behavior.
* `FinalityStatus.IsEmpty()` method.
* EIP-7843 (Amsterdam): `BlockData.SlotNumber` field and `BlockHeader.SlotNumber` propagation.
* `ChainConfig.ShanghaiBlock`, `CancunBlock` and `PragueBlock` to activate forks by block number, for chains that schedule them that way (e.g. Polygon PoS). A fork is active when either its time or its block condition holds.
* `Config.IsNeverRevertedLog` to mark logs that stay in the receipt even when the call that emitted them is reverted.
* `Config.BeforeBlockFlush` called with the completed block right before it is written out, letting the chain rewrite it.
* `Config.DropTransactionsWithoutReceipt` to leave out of the block a transaction that ends without a receipt, for chains that skip failed transactions and carry on with the block (e.g. Arbitrum).
* `BlockEvent.Rules` to give the fork rules of a block instead of computing them from `ChainConfig`, for chains whose fork activation depends on data outside it (e.g. Arbitrum activates Prague from the ArbOS version).
* EIP-7928 (Amsterdam): `BlockData.BlockAccessListHash` field and `BlockHeader.BlockAccessListHash` propagation.

### Changed

* `OnLog` now accepts a log emitted while no call is active in a transaction (e.g. Polygon's fee transfer log emitted after the root call ends) and attaches it to the root call, instead of panicking. It still panics when there is no root call to attach the log to: in a system call, or in a transaction without calls.
* `OnStorageChange` now accepts a storage change made while no call is active in a transaction (e.g. Arbitrum's ArbOS writes around the EVM call) and attaches it to the root call, with the same panics as `OnLog` when there is no root call.
* `OnSystemCallStart` can now be called while a transaction is being traced (e.g. Arbitrum runs a system call within its internal transaction). The transaction is set aside and restored when the system call ends.
* Keccak preimages are now filtered: `Call.KeccakPreimages` only holds preimages of at most 256 bytes that explain a storage slot written by the transaction or system call, directly, at an offset below 2^64, or through nested hashing up to 16 levels. Preimages used only for reads, signatures, CREATE2 addresses or contract-level hashing are dropped.
* Trace/debug log calls in `OnNonceChange`, `OnCodeChange`, and `OnStorageChange` are now emitted before early-return guards so they fire even for no-op (equal old/new value) invocations.
* `OnBalanceChange`, `OnNonceChange`, `OnCodeChange`, and `OnStorageChange` now skip recording when old and new values are equal. This avoids emitting no-op state changes in the block model.
* `FIRE BLOCK` output line now includes a flash block index slot and a computed `lib_num`. New format: `FIRE BLOCK <block_num> <flash_block_idx> <block_hash> <prev_num> <prev_hash> <lib_num> <timestamp_unix_nano> <payload_base64>`. `flash_block_idx` is `0` for non-flash blocks. `lib_num` is derived from the current `FinalityStatus` (falling back to `max(block_num-200, 0)` when no finality is known, and always capped to no more than 200 blocks behind `block_num`).
* The LIB number in the `FIRE BLOCK` line is now capped to the block number, so a replayed block never advertises a LIB ahead of itself.
* Writing to the output stream now retries short writes and panics when the data still can't be written, instead of silently dropping the block. Block serialization errors also panic.
* Block withdrawals are now always recorded. The `Config.SkipWithdrawals` flag has been removed; consumers that previously relied on it to suppress withdrawals should handle filtering on their side if needed.

### Removed

* Gas changes tracking (`OnGasChange`, per-opcode gas recording) is no longer supported. The `GasChanges` field on calls will always be empty. Consumers that relied on this data must migrate to alternative gas accounting.
* Remove `Config.SkipWithdrawals` flag (see above).

## v4.0.4

### Fixed

* SetCode authorization `r` and `s` signature fields now serialize as empty string (`""`) when zero, matching production behavior of the native tracer.

## v4.0.3

### Added

* Add `Tracer.GetConfig() *Config` getter to expose the tracer's runtime configuration.
* Add `Config.LogKeyValues() []any` returning a flat key-value list (keys prefixed with `config_`, values as human-readable strings) suitable for structured logging.

## v4.0.2

### Added

* Add optional `configFunc func(*Config)` parameter to `OnBlockchainInit` allowing callers to tweak `Config` fields based on chain-specific knowledge available at init time (e.g. setting `SkipWithdrawals` based on chain ID).

## v4.0.1

### Added

* Add `Config.SkipWithdrawals` flag to suppress recording of `block.Withdrawals` entries (e.g. Ethereum Mainnet which does not record withdrawals in the block model).

### Removed

* Remove gas changes tracking: `OnGasChange` hook, per-opcode gas recording, and all `GasChange` fields from the block model. This produces [Ethereum Mainnet Block version 5](https://docs.substreams.dev/reference-material/chain-support/ethereum-data-model#version-5).
* Remove all backward compatibility code that was present for prior block model versions.

## v4.0.0

### Added

* First release of the module as `github.com/streamingfast/evm-firehose-tracer-go/v4`, aligned with [Ethereum Mainnet Block version 4](https://docs.substreams.dev/reference-material/chain-support/ethereum-data-model#version-4) for the Ethereum Block `sf.ethereum.type.v2.Block` protobuf model.
