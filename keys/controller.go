package main

import (
	"context"
	"net"

	generated "github.com/jepp2078/heimdall"

	log "github.com/sirupsen/logrus"
	"grpc.go4.org"
	"k8s.io/client-go/kubernetes"
)

// Controller struct defines how a controller should encapsulate
// logging and client connectivity
type Controller struct {
	logger    *log.Entry
	clientset kubernetes.Interface
}

// GetPrivateKey receives a request for a private key in a specified namespace.
// The Key Manager will check for this key, and return a reference to it if it exists.
// If the key doesn't exist an error will be returned to the callee.
func (c *Controller) GetPrivateKey(ctx context.Context, namespace *generated.Namespace) (*generated.Key, error) {
	return nil, nil
}

// GetPublicKey receives a request for a public key. If the key doesn't exist a private/public key pair will be created in the specified namespace, and a reference to the public key will be returned to the callee.
func (s *Controller) GetPublicKey(ctx context.Context, namespace *generated.Namespace) (*generated.Key, error) {
	return nil, nil
}

func (c *Controller) Run() {
	lis, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		c.logger.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	generated.RegisterHeimdallKeysServer(grpcServer, c)
	c.logger.Info("GRPC service exposed on tcp://127.0.0.1:8080/")
	c.logger.Fatalf("Fatal: %s", grpcServer.Serve(lis))
}
