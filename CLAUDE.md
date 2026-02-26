## Native Tracer

Our current goal in this project is ensuring that `go-ethereum/eth/tracers/firehose.go` tracer behavior is faithfully ported to the shared tracer in such way that for every sequence of tracing calls made, the same tracing sequence on native tracer and on shared tracer are strictly equivalent ensure strict conformance of our tracer against the spec of the native tracers.

Always align behavior of the shared tracer with the behavior from native tracer. Try to copy as much code as possible from native tracer when implementing fixes.

DO NOT try hack/differences in behavior in shared tracer versus native tracer.

## Shared Tracer

The shared tracer `tracer.go` (and friends) MUST NOT depend on go-ethereum at all, it must be agnostic of it. If you need behavior provided by `f.evm` (and `f.evm.StateDB`) introduce abstraction so that chain specific implementation can pass the concrete implementation.

## Testing

### Debugging

Ok you have a magic trick for debugging issue in tracer (and native tracer) that consist of running a particular test and having `FIREHOSE_ETHEREUM_TRACER_LOG_LEVEL=trace` as env var. As the tracer execute, it prints execution data

```log
[Firehose (geth)] resetting transaction state
[Firehose] trx end (tracer=global)
```

The one with `[Firehose (geth)]` are native tracer, the one without it's our implementation. That's an effective way of debugging