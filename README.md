# Heimdall

#### Design document - 23/05-2019

## Purpose

Heimdall&#39;s purpose is to ease the use and management of centralized application configuration. When working with sensitive application configuration such as credentials, situations may arise where access to these needs to be restricted to a few select individuals. Furthermore Heimdall intend to remove the need of exposing configuration to the CI/CD pipeline.

## Components

### Config Injector - Mutating webhook

This mutating webhook will be responsible for fetching configurations, and injecting these as environment variables on kubernetes pods.

### Key Manager - Service

This service will be responsible for managing and creating keys used for encrypting and decrypting configurations.

### CLI

This cli will be responsible for encrypting and injecting configuration values.

## Design

### Overall Design

Configurations cannot be shared across namespaces. The configuration that needs to be applied to a given application will have to follow the specified yaml configuration format. Encrypted values in the yaml configuration file will be base64 encoded. Heimdall is intended to be deployed to the kube-system namespace.

### Config Injector - Mutating webhook

The Config Injector will be watching for kubernetes deployments with a predefined annotation. If this annotation is seen, the Config Injector will fetch the yaml specification at the given location. A config map will be created containing all the defined variables. The Config Injector will then inject the config map on the pods of the deployment, or add the pods back into the queue, if any errors are thrown. This is to ensure that the pods will be in a CrashLoopBackOff if configuration is not applied.

### Key Manager - Service

The key manager will have an endpoint called GetPrivateKey that receives a request for a private key in a specified namespace. The Key Manager will check for this key, and return a reference to it if it exists. If the key doesn&#39;t exist an error will be returned to the callee.

It will have an endpoint called GetPublicKey that receives a request for a public key. If the key doesn&#39;t exist a private/public key pair will be created in the specified namespace, and a reference to the public key will be returned to the callee.

### CLI

The CLI will use the kubeconfig currently defined in the environment, or passed in via. command line arguments.

It will have a command called inject. Inject takes three arguments:

1. A reference to a configuration file
2. A name of the desired variable
3. Input value to be encrypted.

The command will fetch the public key from the supplied namespace. The encrypted value will then be injected into the configuration file.
