package startup

import (
	"context"
	"fmt"
	"gateway/client"
	"gateway/config"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Server struct {
	config         *config.Config
	noAuthConfig   *config.Config
	noAuthMethods  []string
	useRateLimiter bool
}

func NewServer(config *config.Config, noAuthConfig *config.Config, useRateLimiter bool) *Server {
	return &Server{config: config,
		noAuthConfig:   noAuthConfig,
		noAuthMethods:  nil,
		useRateLimiter: useRateLimiter}
}

func (s *Server) Start() {
	clientRegistry := s.prepareClients()
	s.noAuthMethods = getNoAuthMethods(s.noAuthConfig.Groups["core"]["v1"])
	router := s.prepareRoutes(clientRegistry)
	handler := otelhttp.NewHandler(router, "lunar-http")

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", s.config.Gateway.Port),
		Handler: handler,
	}

	log.Fatal(server.ListenAndServe())
}

func (s *Server) prepareClients() *client.ClientRegistry {
	log.Println("Preparing clients")
	clientRegistry := &client.ClientRegistry{
		Clients: make(map[string]client.Client),
	}

	for k, v := range s.config.Services {
		clientRegistry.NewClient(k, v)
	}

	return clientRegistry
}
func (s *Server) prepareRoutes(clientRegistry *client.ClientRegistry) *mux.Router {
	router := mux.NewRouter().PathPrefix(s.config.Gateway.Route).Subrouter()

	for group, versions := range s.config.Groups {
		groupRouter := router.PathPrefix("/" + group).Subrouter()
		for version, methods := range versions {
			versionRouter := groupRouter.PathPrefix("/" + version).Subrouter()
			for mtdName, mtdConf := range methods {
				log.Printf("Name %s Conf %+v", mtdName, mtdConf)
				client := clientRegistry.Clients[mtdConf.Service]
				versionRouter.Path(mtdConf.MethodRoute).HandlerFunc(s.methodInterceptor(mtdName, client)).Methods(mtdConf.Type)
			}
		}
	}

	return router
}

func (s *Server) methodInterceptor(mtdName string, client client.Client) http.HandlerFunc {
	var h http.HandlerFunc

	if isNoAuthMethod(mtdName, s.noAuthMethods) {
		h = client.InvokeGrpcMethod
	} else {
		h = client.WrapGrpcMethod
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), "mtdName", mtdName)

		if s.useRateLimiter {
			systemAllowed := client.WithSystemRateLimiter(w, r)
			if !systemAllowed {
				http.Error(w, os.Getenv("rl.system.message"), http.StatusBadRequest)
				return
			}
		}

		h.ServeHTTP(w, r.WithContext(ctx))

	})
}

func getNoAuthMethods(noAuthMap map[string]config.MethodConfig) []string {
	keys := make([]string, 0, len(noAuthMap))
	for key := range noAuthMap {
		keys = append(keys, key)
	}
	return keys
}

func isNoAuthMethod(mtd string, noAuthMethods []string) bool {
	for _, element := range noAuthMethods {
		if element == mtd {
			return true
		}
	}
	return false
}
