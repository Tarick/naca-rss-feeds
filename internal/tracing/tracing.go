package tracing

import (
	"io"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"

	jaegerConfig "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-client-go/zipkin"
	// We need Zipkin support since Istio uses its headers for tracing
	// This lib will enable Zipkin headers (e.g. X-B3-Parentspanid) propagation
)

// Config defines tracing configuration to be used in config file
type Config struct {
	ServiceName       string  `mapstructure:"service_name"`
	SamplerRate       float64 `mapstructure:"sampler_rate"`
	SamplerType       string  `mapstructure:"sampler_type"`
	AgentAddress      string  `mapstructure:"agent_address"`
	CollectorEndpoint string  `mapstructure:"collector_endpoint"`
	LogSpans          bool    `mapstructure:"log_spans"`
	Disabled          bool    `mapstructure:"disabled"`
}

// New returns an instance of opentracing Tracer based on Jaeger instance
func New(config Config, logger jaeger.Logger) (opentracing.Tracer, io.Closer, error) {
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
		Disabled: config.Disabled,
	}
	// Zipkin shares span ID between client and server spans; it must be enabled via the following option.
	zipkinPropagator := zipkin.NewZipkinB3HTTPHeaderPropagator()
	tracer, closer, err := cfg.NewTracer(jaegerConfig.Logger(logger),
		jaegerConfig.Extractor(opentracing.HTTPHeaders, zipkinPropagator),
		jaegerConfig.Injector(opentracing.HTTPHeaders, zipkinPropagator),
		jaegerConfig.ZipkinSharedRPCSpan(true))

	return tracer, closer, err
}
