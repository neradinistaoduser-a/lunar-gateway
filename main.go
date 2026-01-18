package main

import (
	"context"
	"fmt"
	"gateway/config"
	"gateway/startup"
	"log"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

var path = "config.yml"
var noAuthPath = "no_auth_config.yml"

func main() {

	tracingEndpoint := os.Getenv("JAEGER_OTLP_ENDPOINT")
	shutdown := initTracing("lunar-gateway", tracingEndpoint)
	tr := otel.Tracer("debug")
	_, span := tr.Start(context.Background(), "gateway-startup-test")
	span.End()
	defer shutdown()

	conf, err := config.LoadConfig(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	noAuthConf, err := config.LoadConfig(noAuthPath)
	if err != nil {
		fmt.Println(err)
		return
	}

	args := os.Args
	useRateLimiter := false
	if len(args) > 1 && args[0] == "sysrl" {
		useRateLimiter = true
	}

	gateway := startup.NewServer(conf, noAuthConf, useRateLimiter)
	gateway.Start()
}

func initTracing(serviceName, endpoint string) func() {
	ctx := context.Background()

	exp, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		log.Fatal(err)
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return func() {
		_ = tp.Shutdown(ctx)
	}
}
