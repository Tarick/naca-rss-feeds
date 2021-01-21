package tracing

import (
	"fmt"
	"io"
	"os"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
)

type Config struct {
	ServiceName       string  `mapstructure:"service_name"`
	SamplerRate       float64 `mapstructure:"sampler_rate"`
	SamplerType       string  `mapstructure:"sample_type"`
	AgentAddress      string  `mapstructure:"agent_address"`
	CollectorEndpoint string  `mapstructure:"collector_endpoint"`
	LogSpans          bool    `mapstructure:"log_spans"`
}

// New returns an instance of opentracing Tracer based on Jaeger instance
func New(config Config) (opentracing.Tracer, io.Closer) {
	cfg := &jaegerConfig.Configuration{
		ServiceName: config.ServiceName,
		Sampler: &jaegerConfig.SamplerConfig{
			Type:  config.SamplerType,
			Param: config.SamplerRate,
		},
		Reporter: &jaegerConfig.ReporterConfig{
			LogSpans:           config.LogSpans,
			LocalAgentHostPort: config.AgentAddress,
			CollectorEndpoint:  config.CollectorEndpoint,
		},
	}
	// FIXME: provide system logger to log spans, right now it is just stdout
	tracer, closer, err := cfg.NewTracer(jaegerConfig.Logger(jaeger.StdLogger))
	if err != nil {
		fmt.Printf("ERROR: cannot init Jaeger: %v\n", err)
		os.Exit(1)
	}
	return tracer, closer
}
