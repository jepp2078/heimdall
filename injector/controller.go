package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"io/ioutil"

	log "github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-billy.v4"
	"gopkg.in/src-d/go-billy.v4/memfs"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"gopkg.in/src-d/go-git.v4/storage/memory"
	"gopkg.in/yaml.v2"
	apiV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	b64 "encoding/base64"
	"encoding/pem"

	"github.com/jepp2078/heimdall/generated"
	"github.com/jepp2078/heimdall/models"
)

const (
	HeimdallAnnotationRepository        = "heimdall-repository"
	HeimdallAnnotationPath              = "heimdall-path"
	HeimdallNameAnnotationName          = "heimdall-name"
	HeimdallConfigVersionAnnotationName = "heimdall-config-version"
	HeimdallInjectedAnnotationName      = "heimdall-injected"
)

// Controller struct defines how a controller should encapsulate
// logging, client connectivity, informing (list and watching)
// queueing, and handling of resource changes
type Controller struct {
	logger         *log.Entry
	clientset      kubernetes.Interface
	keysClient     generated.HeimdallKeysClient
	gitCredentials string
	queue          workqueue.RateLimitingInterface
	informer       cache.SharedIndexInformer
}

// Run is the main path of execution for the controller loop
func (c *Controller) Run(stopCh <-chan struct{}) {

	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()
	// ignore new items in the queue but when all goroutines
	// have completed existing items then shutdown
	defer c.queue.ShutDown()

	// run the informer to start listing and watching resources
	go c.informer.Run(stopCh)

	// do the initial synchronization (one time) to populate resources
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("error syncing cache"))
		return
	}

	// run the runWorker method every second with a stop channel
	wait.Until(c.runWorker, time.Second, stopCh)
}

// HasSynced allows us to satisfy the Controller interface
// by wiring up the informer's HasSynced method to it
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

// runWorker executes the loop to process new items added to the queue
func (c *Controller) runWorker() {
	// invoke processNextItem to fetch and consume the next change
	// to a watched or listed resource
	for c.processNextItem() {
	}
}

// processNextItem retrieves each queued item and takes the
// necessary handler action based off of if the item was
// created or deleted
func (c *Controller) processNextItem() bool {
	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	key, quit := c.queue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer c.queue.Done(key)

	// assert the string out of the key (format `namespace/name`)
	keyRaw := key.(string)

	// take the string key and get the object out of the indexer
	//
	// item will contain the complex object for the resource and
	// exists is a bool that'll indicate whether or not the
	// resource was created (true) or deleted (false)
	//
	// if there is an error in getting the key from the index
	// then we want to retry this particular queue key a certain
	// number of times (5 here) before we forget the queue key
	// and throw an error
	item, exists, err := c.informer.GetIndexer().GetByKey(keyRaw)
	if err != nil {
		if c.queue.NumRequeues(key) < 5 {
			c.logger.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, retrying", key, err)
			c.queue.AddRateLimited(key)
		} else {
			c.logger.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, no more retries", key, err)
			c.queue.Forget(key)
			utilruntime.HandleError(err)
		}
	}

	// if the item doesn't exist then it was deleted and we need to fire off the handler's
	// ObjectDeleted method. but if the object does exist that indicates that the object
	// was created (or updated) so run the ObjectCreated method
	//
	// after both instances, we want to forget the key from the queue, as this indicates
	// a code path of successful queue key processing
	if !exists {
		// If exists is false, we can assume that the key is a reference to a configmap
		parts := strings.Split(keyRaw, "/")
		c.logger.Printf("Deleting configmap: %s/%s", parts[0], parts[1])
		res, err := c.clientset.CoreV1().ConfigMaps(parts[0]).Get(parts[1], metaV1.GetOptions{})
		if err != nil {
			c.logger.Printf("Configmap not found: %s", err)
		} else {
			c.clientset.CoreV1().ConfigMaps(res.Namespace).Delete(res.Name, &metaV1.DeleteOptions{})
		}
	} else {
		deployment := item.(*apiV1.Deployment)
		c.logger.Printf("Deployment discovered: %s/%s", deployment.Namespace, deployment.Name)
		annotations := deployment.Annotations
		if _, found := annotations[HeimdallAnnotationRepository]; found {
			c.logger.Printf("Annotation found")
			if _, found := annotations[HeimdallInjectedAnnotationName]; found {
				c.logger.Printf("Skipping. Deployment already injected")
			} else {
				err := c.addConfigurationToDeployment(deployment, annotations[HeimdallAnnotationRepository], annotations[HeimdallAnnotationPath])
				if err != nil {
					if c.queue.NumRequeues(key) < 3 {
						c.logger.Error(err)
						c.logger.Errorf("Re-queuing key %v more time", (c.queue.NumRequeues(key)-3)*-1)
						c.queue.AddRateLimited(key)
					} else {
						c.queue.Forget(key)
						c.logger.Errorf("Could't add configuration to containers. Forgetting deployment: %s", deployment.Name)
					}
				}
			}
		} else {
			c.logger.Printf("Skipping. Annotation not found.")
		}
	}

	// keep the worker loop running by returning true
	return true
}

func (c *Controller) addConfigurationToDeployment(obj *apiV1.Deployment, repository string, path string) error {
	deployment := obj.DeepCopy()
	// Create configmap and append to container
	configuration, err := c.generateConfiguration(repository, path)

	if err != nil {
		return fmt.Errorf("%s", err)
	}

	deployment.Annotations[HeimdallInjectedAnnotationName] = "true"
	deployment.Annotations[HeimdallNameAnnotationName] = configuration.Metadata.Name
	deployment.Annotations[HeimdallConfigVersionAnnotationName] = configuration.ConfigVersion

	for i := range deployment.Spec.Template.Spec.Containers {
		configMapSource, err := c.createConfigMapFromConfiguration(configuration)

		if err != nil {
			return fmt.Errorf("%s", err)
		}

		if configMapSource != nil {
			envFrom := coreV1.EnvFromSource{ConfigMapRef: configMapSource}
			c.logger.Printf("Injecting ConfigMap to deployment: %s/%s", deployment.Namespace, deployment.Name)
			deployment.Spec.Template.Spec.Containers[i].EnvFrom = append(deployment.Spec.Template.Spec.Containers[i].EnvFrom, envFrom)
		}

		// err := c.appendSecretsFromConfiguration(*container, configuration)
	}

	_, err = c.clientset.AppsV1().Deployments(configuration.Metadata.Namespace).Update(deployment)

	if err != nil {
		return fmt.Errorf("%s", err)
	}

	return nil
}

func (c *Controller) generateConfiguration(repository string, path string) (*models.Configuration, error) {
	// Filesystem abstraction based on memory
	fs := memfs.New()
	// Git objects storer based on memory
	storer := memory.NewStorage()

	if c.gitCredentials != "" {
		// Get the secret containing git-credentatials
		parts := strings.Split(c.gitCredentials, "/")
		c.logger.Printf("Fetching git-credentials secret: %s/%s", parts[0], parts[1])
		secret, err := c.clientset.CoreV1().Secrets(parts[0]).Get(parts[1], metaV1.GetOptions{})

		if err != nil {
			return nil, fmt.Errorf("%s", err)
		}

		err = c.fetchConfigFromRemote(repository, fs, storer, secret)

		if err != nil {
			return nil, fmt.Errorf("%s", err)
		}

	} else {
		err := c.fetchConfigFromRemote(repository, fs, storer, &coreV1.Secret{})

		if err != nil {
			return nil, fmt.Errorf("%s", err)
		}
	}

	configuration, err := c.unmarshalConfiguration(path, fs)

	if err != nil {
		return nil, fmt.Errorf("%s", err)
	}

	return configuration, nil
}

func (c *Controller) unmarshalConfiguration(file string, fs billy.Filesystem) (*models.Configuration, error) {
	// Open file from in-mem filesystem
	configFile, err := fs.Open(file)
	defer configFile.Close()

	if err != nil {
		return nil, fmt.Errorf("%s", err)
	}

	configuration := &models.Configuration{}
	byteArray, err := ioutil.ReadAll(configFile)
	if err != nil {
		return nil, fmt.Errorf("%s", err)
	}

	err = yaml.Unmarshal(byteArray, configuration)

	if err != nil {
		return nil, fmt.Errorf("%s", err)
	}

	return configuration, nil
}

func (c *Controller) fetchConfigFromRemote(location string, fs billy.Filesystem, storer *memory.Storage, secret *coreV1.Secret) error {
	// Fetch configuration from repository
	if secret.Name == "" {
		c.logger.Printf("Fetching config from remote: %s", location)
		_, err := git.Clone(storer, fs, &git.CloneOptions{
			URL: location,
		})
		if err != nil {
			return fmt.Errorf("%s", err)
		}
	} else {
		c.logger.Printf("Fetching config with auth from remote: %s", location)
		_, err := git.Clone(storer, fs, &git.CloneOptions{
			URL: location,
			Auth: &http.BasicAuth{
				Username: string(secret.Data["username"]),
				Password: string(secret.Data["password"]),
			},
		})
		if err != nil {
			return fmt.Errorf("%s", err)
		}
	}

	return nil
}

func (c *Controller) createConfigMapFromConfiguration(configuration *models.Configuration) (*coreV1.ConfigMapEnvSource, error) {
	if len(configuration.Entities) > 0 {
		configMap := &coreV1.ConfigMap{}
		data := make(map[string]string)
		for _, entity := range configuration.Entities {
			if !entity.Encrypted {
				data[entity.Name] = entity.Value
			} else {
				query := &generated.Namespace{Namespace: configuration.Metadata.Namespace}
				ctx := context.Background()
				key, err := c.keysClient.GetPrivateKey(ctx, query)

				if err != nil {
					return nil, fmt.Errorf("%s", err)
				}

				value, err := unencryptConfiguration(entity.Value, key)

				if err != nil {
					return nil, fmt.Errorf("%s", err)
				}

				data[entity.Name] = value
			}
		}

		configMap.Name = fmt.Sprintf("heimdall-%s-%s", configuration.Metadata.Name, configuration.ConfigVersion)
		configMap.Namespace = configuration.Metadata.Namespace
		configMap.Data = data

		res, err := c.clientset.CoreV1().ConfigMaps(configuration.Metadata.Namespace).Get(configMap.Name, metaV1.GetOptions{})

		if err != nil {
			c.logger.Printf("Creating config map: %s/%s", configuration.Metadata.Namespace, configMap.Name)
			res, err = c.clientset.CoreV1().ConfigMaps(configuration.Metadata.Namespace).Create(configMap)
			if err != nil {
				return nil, fmt.Errorf("%s", err)
			}
		} else {
			oldConfigMap := res.DeepCopy()

			oldConfigMap.Data = data
			c.logger.Printf("Updating config map: %s/%s", oldConfigMap.Namespace, oldConfigMap.Name)
			res, err = c.clientset.CoreV1().ConfigMaps(configuration.Metadata.Namespace).Update(oldConfigMap)
			if err != nil {
				return nil, fmt.Errorf("%s", err)
			}
		}

		envSrc := &coreV1.ConfigMapEnvSource{
			LocalObjectReference: coreV1.LocalObjectReference{
				Name: res.Name,
			},
		}

		return envSrc, nil
	}

	return nil, nil
}

func unencryptConfiguration(data string, key *generated.Key) (string, error) {
	hash := sha512.New()
	pem, _ := pem.Decode([]byte(key.GetKey()))

	pKey, err := x509.ParsePKCS1PrivateKey(pem.Bytes)

	if err != nil {
		return "", fmt.Errorf("%s", err)
	}

	dataBytes, err := b64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("%s", err)
	}

	plaintext, err := rsa.DecryptOAEP(hash, rand.Reader, pKey, dataBytes, nil)

	if err != nil {
		return "", fmt.Errorf("%s", err)
	}

	return string(plaintext), nil
}
