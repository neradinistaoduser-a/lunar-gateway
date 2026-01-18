package client

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/grpcreflect"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ClientRegistry struct {
	Clients map[string]Client
}

func (registry *ClientRegistry) NewClient(name, address string) {
	log.Printf("New client %s address %s /n", name, address)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	var cc *grpc.ClientConn
	var err error
	defer cancel()
	if cc, err = grpc.DialContext(
		ctx,
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithBlock(),
	); err != nil {
		panic(err)
	}
	refClient := grpcreflect.NewClientAuto(context.Background(), cc)
	descSource := grpcurl.DescriptorSourceFromServer(context.Background(), refClient)
	services, _ := descSource.ListServices()
	filteredServices := make([]string, 0)
	for _, str := range services {
		if !strings.HasPrefix(str, "grpc.") {
			filteredServices = append(filteredServices, str)
		}
	}
	registry.Clients[name] = Client{refClient: refClient, cc: cc, descSource: descSource, grpcServiceName: filteredServices[0]}
}
