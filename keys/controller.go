package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"

	"github.com/jepp2078/heimdall/generated"
	log "github.com/sirupsen/logrus"
	grpc "google.golang.org/grpc"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	heimdallSecretName = "heimdall"
	bitSize            = 2048
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
	c.logger.Infof("Getting private key for namespace: %s", namespace.GetNamespace())
	res, err := c.clientset.CoreV1().Secrets(namespace.GetNamespace()).Get(heimdallSecretName, metaV1.GetOptions{})
	if err != nil {
		return nil, err
	}
	outKey := &generated.Key{}
	outKey.Key = string(res.Data["privateKey"])
	return outKey, nil
}

// GetPublicKey receives a request for a public key.
// If the key doesn't exist a private/public key pair will be created in the specified namespace,
// and a reference to the public key will be returned to the callee.
func (c *Controller) GetPublicKey(ctx context.Context, namespace *generated.Namespace) (*generated.Key, error) {
	c.logger.Infof("Getting public key for namespace: %s", namespace.GetNamespace())
	secret := &coreV1.Secret{}
	res, err := c.clientset.CoreV1().Secrets(namespace.GetNamespace()).Get(heimdallSecretName, metaV1.GetOptions{})

	if err != nil {
		c.logger.Printf("Creating heimdall secret: %s/%s", namespace.GetNamespace(), heimdallSecretName)
		reader := rand.Reader
		key, err := rsa.GenerateKey(reader, bitSize)

		if err != nil {
			return nil, fmt.Errorf("%s", err)
		}

		secret.Name = heimdallSecretName
		secret.Namespace = namespace.GetNamespace()
		secret.StringData["publicKey"] = getPublicPEMKey(&key.PublicKey)
		secret.StringData["privateKey"] = getPEMKey(key)

		res, err = c.clientset.CoreV1().Secrets(namespace.GetNamespace()).Create(secret)
		if err != nil {
			return nil, fmt.Errorf("%s", err)
		}
	}

	outKey := &generated.Key{}
	outKey.Key = string(res.Data["publicKey"])
	return outKey, nil
}

func getPEMKey(key *rsa.PrivateKey) string {
	var privateKey = &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	bytes := pem.EncodeToMemory(privateKey)

	return string(bytes)
}

func getPublicPEMKey(pubkey *rsa.PublicKey) string {

	var pemkey = &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(pubkey),
	}

	bytes := pem.EncodeToMemory(pemkey)

	return string(bytes)
}

func (c *Controller) Run(port string) {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	generated.RegisterHeimdallKeysServer(grpcServer, c)
	log.Infof("GRPC service exposed on tcp://127.0.0.1:%s/", port)
	log.Fatalf("Fatal: %s", grpcServer.Serve(lis))
}
