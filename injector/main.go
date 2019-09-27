package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jepp2078/heimdall/generated"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	grpc "google.golang.org/grpc"
	apiV1 "k8s.io/api/apps/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

var (
	kubeConfigFile    = flag.String("kubeconfig", "", "Path to kubeconfig file with authorization and master location information.")
	gitCredentials    = flag.String("git-credentials", "", "Reference to secret that holds git credentials. Formatted username:password. If left blank, no credentials will be used.")
	cleanUpConfigmaps = flag.Bool("configmap-cleanup", false, "If set to true, config-maps injected into deployments, will be deleted when the deployment is deleted.")
	keysAddr          = flag.String("keys-address", "heimdall-keys:8080", "The address of the heimdall-keys pod")
)

func init() {
	flag.Parse()
	if *gitCredentials != "" {
		parts := strings.Split(*gitCredentials, "/")
		if len(parts) != 2 {
			panic("git-credentials flag invalid. Format as namespace/secretName.")
		}
	}
}

// main code path
func main() {

	// create grpc connection to keys service
	grpc, err := createKeysClient()

	defer grpc.Close()

	if err != nil {
		log.Fatalf("Fatal: %s", err.Error())
		return
	}

	keysClient := generated.NewHeimdallKeysClient(grpc)

	// get the Kubernetes client for connectivity
	client := getKubernetesClient()

	//create logger
	logger := log.NewEntry(log.New())

	// create the informer so that we can not only list resources
	// but also watch them for all deployments
	informer := cache.NewSharedIndexInformer(
		// the ListWatch contains two different functions that our
		// informer requires: ListFunc to take care of listing and watching
		// the resources we want to handle
		&cache.ListWatch{
			ListFunc: func(options metaV1.ListOptions) (runtime.Object, error) {
				// list all of the deployments in all namespaces
				return client.AppsV1().Deployments("").List(options)
			},
			WatchFunc: func(options metaV1.ListOptions) (watch.Interface, error) {
				// watch all of the deployments in all namespaces
				return client.AppsV1().Deployments("").Watch(options)
			},
		},
		&apiV1.Deployment{}, // the target type (deployment)
		0,                   // no resync (period of 0)
		cache.Indexers{},
	)

	// create a new queue so that when the informer gets a resource that is either
	// a result of listing or watching, we can add an idenfitying key to the queue
	// so that it can be handled in the handler
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// add event handlers to handle the three types of events for resources:
	//  - adding new resources
	//  - updating existing resources
	//  - deleting resources
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// convert the resource object into a key (in this case
			// we are just doing it in the format of 'namespace/name')
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				// add the key to the queue for the handler to get
				queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, obj interface{}) {
			//check if already heimdall injected
			if validateUpdate(oldObj, obj) {
				// convert the resource object into a key (in this case
				// we are just doing it in the format of 'namespace/name')
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					// add the key to the queue for the handler to get
					queue.Add(key)
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamsespaceKeyFunc is a helper function that allows
			// us to check the DeletedFinalStateUnknown existence in the event that
			// a resource was deleted but it is still contained in the index
			//
			// this then in turn calls MetaNamespaceKeyFunc
			//
			// If the deleted deployment has been injected with a configmap, a reference
			// to that config map will be contructed, and appended to the queue
			if *cleanUpConfigmaps {
				_, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					deployment := obj.(*apiV1.Deployment)
					annotations := deployment.Annotations
					if _, found := annotations[HeimdallAnnotationName]; found {
						name := fmt.Sprintf("%s/heimdall-%s-%s", deployment.Namespace, annotations[HeimdallNameAnnotationName], annotations[HeimdallConfigVersionAnnotationName])
						queue.Add(name)
					}
				}
			}
		},
	})

	// construct the Controller object which has all of the necessary components to
	// handle logging, connections, informing (listing and watching), the queue,
	// and the handler
	controller := Controller{
		logger:         logger,
		clientset:      client,
		keysClient:     keysClient,
		gitCredentials: *gitCredentials,
		informer:       informer,
		queue:          queue}

	// use a channel to synchronize the finalization for a graceful shutdown
	stopCh := make(chan struct{})
	defer close(stopCh)

	// run the controller loop to process items
	go controller.Run(stopCh)

	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}

func validateUpdate(oldObj, obj interface{}) bool {
	oldAnnotation := oldObj.(*apiV1.Deployment).Annotations[HeimdallInjectedAnnotationName]
	newAnnotation := obj.(*apiV1.Deployment).Annotations[HeimdallInjectedAnnotationName]

	return oldAnnotation != newAnnotation
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

func createKeysClient() (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())
	conn, err := grpc.Dial(*keysAddr, opts...)
	if err != nil {
		log.Fatalf("failed to dial: %v", err)
		return nil, err
	}
	log.Infof("Could etablish connection to Keys GRPC address: %s", *keysAddr)
	return conn, nil
}
