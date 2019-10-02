package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	kubeConfigFile = flag.String("kubeconfig", "", "Path to kubeconfig file with authorization and master location information.")
	port           = flag.String("port", "8081", "Application port to use")
)

func init() {
	flag.Parse()
}

// main code path
func main() {
	// get the Kubernetes client for connectivity
	client := getKubernetesClient()

	//create logger
	logger := log.NewEntry(log.New())

	// construct the Controller object which has all of the necessary components to
	// handle logging and connections
	grpcServer := Controller{
		logger:    logger,
		clientset: client,
	}

	go grpcServer.Run(*port)

	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}

func getKubernetesClient() kubernetes.Interface {
	// creates the in-cluster config
	log.Info("Trying to configure InClusterConfig")
	configInCluster, err := rest.InClusterConfig()
	if err == nil {
		// creates the clientset
		clientSet, err := kubernetes.NewForConfig(configInCluster)
		if err != nil {
			log.Warn(err.Error())
		}
		log.Info("Successfully constructed k8s client")
		return clientSet
	}

	log.Warn("InClusterConfig failed: " + err.Error())

	// construct the path to resolve to `~/.kube/config`
	kubeConfigPath := os.Getenv("KUBECONFIG")

	if kubeConfigPath == "" {
		kubeConfigPath = *kubeConfigFile
	}

	log.Info("KubePath: " + kubeConfigPath)
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	//Set insecure to be able to communicate with self signed certs
	config.CAData = nil
	config.Insecure = true
	if err != nil {
		log.Fatalf("getClusterConfig: %v", err)
	}

	// generate the client based off of the config
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("getClusterConfig: %v", err)
	}

	log.Info("Successfully constructed k8s client")
	return client
}
