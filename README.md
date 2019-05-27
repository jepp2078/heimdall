# Heimdall

Design document - 23/05-2019

Author: Jeppe Johansen

Purpose

Heimdall&#39;s purpose is to ease the use and management of centralized application configuration. When working with sensitive application configuration such as credentials, situations may arise where access to these needs to be restricted to a few select individuals. Furthermore Heimdall intend to remove the need of exposing configuration to the CI/CD pipeline.

Components

Config Injector - Mutating webhook

This mutating webhook will be responsible for fetching configurations, and injecting these as environment variables and secrets on kubernetes pods.

Secret Manager - Service

This service will be responsible for encrypting and decrypting supplied values. Furthermore it is also responsible for creating, exposing and managing secrets.

Key Manager - Service

This service will be responsible for managing and creating keys used for encrypting and decrypting configurations. It will use the built in certificate service in kubernetes.

KCF - CLI

This cli will be responsible for encrypting and injecting configuration values and deploying Heimdall to clusters.

Design

Overall Design

Configurations cannot be shared across namespaces unless explicitly specified in the configuration of Heimdall. The configuration that needs to be applied to a given application will have to follow the specified yaml configuration format. Encrypted values in the yaml configuration file will be base64 encoded. Heimdall can only be run with RBAC enabled. This is to ensure that users are restricted to use the Secret Manager targeting namespaces they own or have access to. Heimdall is intended to be deployed to the system namespace.

Config Injector - Mutating webhook

The Config Injector will be written in GO using client-go.

The Config Injector will be watching for kubernetes pods with a predefined label in the format of heimdall/identification:location:file. If this label is seen, the Config Injector will fetch the yaml specification at the given location. A config map will be created containing all the non-encrypted variables. Then for each encrypted variable, the secret managers endpoint GetSecret will be invoked with the value of the encrypted variable, and the namespace that the pod resides in. The output of this call is a reference to a secret, or access denied. The Config Injector will then inject the secret(s) and the config map on the kubernetes pod, or add the pod back into the queue, if any errors are thrown. This is to ensure that the pod will be in a CrashLoopBackOff.

Secret Manager - Service

The Secret Manager is to be implemented as a GRPC service written in GO using client-go.

It will have an endpoint called GetSecret that receives a base64 encrypted string and a namespace. The endpoint will invoke the Key Managers endpoint GetPrivateKey with the received namespace to fetch the reference to the private key used to decrypt the base64 string, if the key is not found an error will be returned. The Secret Manager will try to decrypt the string, and if successful, create a secret in the received namespace. A reference to this secret is returned to the callee.

It will have an endpoint called CreateSecret that that receives a unencrypted string and a namespace. The endpoint will invoke the Key Managers endpoint GetPublicKey with the received namespace to fetch the reference to the public key used to encrypt the received string. If this fails an error is thrown. The Secret Manager will encrypt the string, base64 encrypt it, and return the resulting value to the callee.

Key Manager - Service

The Key Manager is to be implemented as a GRPC service in GO using client-go.

It will have an endpoint called GetPrivateKey that receives a request for a private key in a specified namespace. The Key Manager will check for this key, and return a reference to it if it exists. If the key doesn&#39;t exist an error will be returned to the callee.

It will have an endpoint called GetPublicKey that receives a request for a public key. If the key doesn&#39;t exist a private/public key pair will be created in the specified namespace, and a reference to the public key will be returned to the callee.

KCF - CLI

The CLI will be implemented in GO using client-go.

It will use the kubeconfig currently defined in the environment, or passed in via. command line arguments.

It will have a command called inject. Inject takes three arguments:

1. A reference to a configuration file
2. A name of the desired variable
3. Input value to be encrypted.

The command will invoke the Secret Managers endpoint CreateSecret with the supplied value and the namespace from the configuration file. The encrypted value will then be injected

 into the configuration file. If an error is received this error is propagated to the user.

It will have a command called init. Init takes no arguments, and simply deploys Heimdall to the environment in the supplied kubeconfig.
