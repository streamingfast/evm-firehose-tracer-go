# Change log

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v5.0.0

### Removed

* Gas changes tracking (`OnGasChange`, per-opcode gas recording) is no longer supported. The `GasChanges` field on calls will always be empty. Consumers that relied on this data must migrate to alternative gas accounting.

## v4.0.0

### Added

* First release of the module as `github.com/streamingfast/evm-firehose-tracer-go/v4`, aligned with [Ethereum Mainnet Block version 4](https://docs.substreams.dev/reference-material/chain-support/ethereum-data-model#version-4) for the Ethereum Block `sf.ethereum.type.v2.Block` protobuf model.
