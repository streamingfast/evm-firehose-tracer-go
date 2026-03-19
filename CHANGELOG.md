# Change log

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased (v5.0.0)

### Changed

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
