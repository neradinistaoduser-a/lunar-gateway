package client

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/fullstorydev/grpcurl"
	"github.com/goccy/go-json"
	"github.com/gorilla/mux"
	"github.com/jhump/protoreflect/grpcreflect"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	refClient       *grpcreflect.Client
	cc              *grpc.ClientConn
	descSource      grpcurl.DescriptorSource
	grpcServiceName string
}

func (client *Client) WrapGrpcMethod(writer http.ResponseWriter, req *http.Request) {
	token := req.Header.Get("Authorization")

	if token == "" || len(token) <= 8 {
		http.Error(writer, "Invalid token", http.StatusUnauthorized)
		return
	}

	response, username, err := InterceptRequest(token[7:])
	if err != nil {
		http.Error(writer, "Invalid token", http.StatusUnauthorized)
		return
	}

	rl_response, err := CallRateLimiter(username, req.Context().Value("mtdName").(string))

	/*if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}*/

	if !rl_response && err == nil {
		http.Error(writer, os.Getenv("rl.user.message"), http.StatusBadRequest)
		return
	}

	req.Header.Add("authz-token", response)
	client.InvokeGrpcMethod(writer, req)
}

func (client *Client) InvokeGrpcMethod(writer http.ResponseWriter, req *http.Request) {
	descSource := client.descSource
	mtdName := req.Context().Value("mtdName").(string)
	reader := prepareReader(req)

	headers := prepareHeaders(req.Header)

	ctx := req.Context()

	ctx = otel.GetTextMapPropagator().Extract(
		ctx,
		propagation.HeaderCarrier(req.Header),
	)
	md := metadata.New(nil)
	otel.GetTextMapPropagator().Inject(
		ctx,
		propagation.HeaderCarrier(md),
	)
	for k, v := range md {
		headers = append(headers, fmt.Sprintf("%s: %s", k, v[0]))
	}

	var resultBuffer bytes.Buffer
	rf, formatter, _ := grpcurl.RequestParserAndFormatter(
		grpcurl.Format("json"),
		descSource,
		reader,
		grpcurl.FormatOptions{},
	)

	h := &grpcurl.DefaultEventHandler{
		Out:            &resultBuffer,
		Formatter:      formatter,
		VerbosityLevel: 0,
	}

	err := grpcurl.InvokeRPC(
		ctx,
		descSource,
		client.cc,
		client.grpcServiceName+"/"+mtdName,
		headers,
		h,
		rf.Next,
	)
	if err != nil {
		log.Printf("[Error] %+v", err)
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	httpCode := statusCodes[h.Status.Code()]
	if h.Status.Message() != "" {
		http.Error(writer, h.Status.Message(), httpCode)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(httpCode)
	writer.Write(resultBuffer.Bytes())
}

func (client *Client) WithSystemRateLimiter(writer http.ResponseWriter, req *http.Request) bool {
	rl_response, err := CallRateLimiter("system", "")

	if err != nil || !rl_response {
		return false
	}

	return true
}

func prepareReader(req *http.Request) io.Reader {
	var reader io.Reader
	params := mux.Vars(req)
	var reqBodyMap map[string]interface{}
	json.NewDecoder(req.Body).Decode(&reqBodyMap)
	if reqBodyMap != nil {
		for k, v := range params {
			reqBodyMap[k] = v
		}
		reqBodyBytes, _ := json.Marshal(reqBodyMap)
		reader = bytes.NewReader(reqBodyBytes)
	} else {
		paramsBytes, _ := json.Marshal(params)
		reader = bytes.NewReader(paramsBytes)
	}

	return reader
}

func prepareHeaders(headers http.Header) []string {
	preparedHeaders := make([]string, 0, len(headers))
	for k, v := range headers {
		if k == "Connection" {
			continue
		}
		fmt.Printf("%s: %s", k, v[0])
		preparedHeaders = append(preparedHeaders, fmt.Sprintf("%s: %s", k, v[0]))
	}
	return preparedHeaders
}

var statusCodes = map[codes.Code]int{
	codes.Internal:         http.StatusInternalServerError,
	codes.Unauthenticated:  http.StatusUnauthorized,
	codes.PermissionDenied: http.StatusForbidden,
	codes.Unimplemented:    http.StatusNotImplemented,
	codes.Unavailable:      http.StatusServiceUnavailable,
	codes.OK:               http.StatusOK,
	codes.AlreadyExists:    http.StatusBadRequest,
	codes.NotFound:         http.StatusNotFound,
	codes.Unknown:          http.StatusInternalServerError,
	codes.InvalidArgument:  http.StatusBadRequest,
}
