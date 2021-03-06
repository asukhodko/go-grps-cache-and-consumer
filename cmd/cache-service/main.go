package main

import (
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/asukhodko/go-grpc-cache-and-consumer/pkg/cache"
	"github.com/asukhodko/go-grpc-cache-and-consumer/pkg/server"
	"github.com/asukhodko/go-grpc-cache-and-consumer/pkg/service"
	"github.com/asukhodko/go-grpc-cache-and-consumer/pkg/urlfetcher"
)

const (
	defaultPort    = ":50051"
	configFilename = "config.yml"
	redisAddress   = "localhost:6379"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	configBody, err := ioutil.ReadFile(configFilename)
	if err != nil {
		log.Fatalf("failed to read config: %v", err)
	}

	config := &struct {
		URLs             []string `yaml:"URLs"`
		MinTimeout       int      `yaml:"MinTimeout"`
		MaxTimeout       int      `yaml:"MaxTimeout"`
		NumberOfRequests int      `yaml:"NumberOfRequests"`
	}{}
	err = yaml.Unmarshal(configBody, config)
	if err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}

	f := urlfetcher.NewFetcher()
	c := cache.NewCache(config.MinTimeout, config.MaxTimeout, redisAddress)
	svc := service.NewService(f, c, config.URLs, config.NumberOfRequests)
	srv := server.NewServer(port, svc)

	log.Printf("Starting server on port %s.\n", port)
	err = srv.Serve()
	if err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
