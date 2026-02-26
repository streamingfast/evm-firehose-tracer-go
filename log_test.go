package firehose

import "github.com/streamingfast/logging"

var zlogTest, _ = logging.PackageLogger("test", "github.com/streamingfast/evm-firehose-tracer-go/test")

func init() {
	logging.InstantiateLoggers()
}
