package logging

import (
	"context"
	"io"
	"os"

	"github.com/newrelic/go-agent/v3/integrations/logcontext-v2/zerologWriter"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/rs/zerolog"
)

var (
	baseWriter io.Writer
	ctxWriter  *zerologWriter.ZerologWriter
	logger     zerolog.Logger
)

func Init(nrApp *newrelic.Application) {
	w := zerologWriter.New(os.Stdout, nrApp)
	baseWriter = w
	ctxWriter = &w
	logger = zerolog.New(baseWriter).With().Timestamp().Logger()
}

func Logger() *zerolog.Logger { return &logger }

func LoggerWithContext(ctx context.Context) *zerolog.Logger {
	if ctxWriter == nil || ctx == nil {
		return &logger
	}
	txnWriter := ctxWriter.WithContext(ctx)
	l := logger.Output(txnWriter)
	return &l
}

func StartSegment(ctx context.Context, name string) func() {
	txn := newrelic.FromContext(ctx)
	if txn == nil {
		return func() {}
	}
	s := txn.StartSegment(name)
	return func() { s.End() }
}
