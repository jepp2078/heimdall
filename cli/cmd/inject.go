/*
Copyright © 2019 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"

	"github.com/golang/glog"
	"github.com/jepp2078/heimdall/models"
	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v2"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var configFileLocation string
var variable string
var data string

const bitSize = 2048
const heimdallSecretName = "heimdall"

// injectCmd represents the inject command
var injectCmd = &cobra.Command{
	Use:   "inject",
	Short: "Inject encrypted values into config files",
	Long:  ``,
	Run:   run,
}

func init() {
	rootCmd.AddCommand(injectCmd)

	injectCmd.Flags().StringVar(&configFileLocation, "config", "", "Heimdall config file to be injected (required)")
	injectCmd.Flags().StringVar(&variable, "variable", "", "Heimdall variable to be injected (required)")
	injectCmd.Flags().StringVar(&data, "data", "", "Variable to be encrypted (required)")
	injectCmd.MarkFlagRequired("config")
	injectCmd.MarkFlagRequired("variable")
	injectCmd.MarkFlagRequired("data")
}

func run(cmd *cobra.Command, args []string) {
	client := GetKubernetesClient()

	// Open file from in-mem filesystem
	configFile, err := os.Open(configFileLocation)
	defer configFile.Close()

	if err != nil {
		glog.Fatal("Config file not found")
	}

	configuration := &models.Configuration{}
	byteArray, err := ioutil.ReadAll(configFile)
	if err != nil {
		glog.Fatal("Config file could not be read")
	}

	err = yaml.Unmarshal(byteArray, configuration)

	if err != nil {
		glog.Fatal("Config file format corrupted")
	}

	changed := false
	for _, entity := range configuration.Entities {
		if entity.Name == variable {
			if entity.Encrypted {
				entity.Value = encryptValue(client, *configuration, data)
				changed = true
			} else {
				glog.Fatal("Variable not set to use encryption")
			}
		}
	}

	if !changed {
		glog.Fatal("Variable not found")
	}

	changedConfig, err := yaml.Marshal(&configuration)
	if err != nil {
		glog.Fatal("Could not marshal config file")
	}

	_, err = configFile.Write(changedConfig)

	if err != nil {
		glog.Fatal("Could not write config file")
	}
}

func encryptValue(client kubernetes.Interface, configuration models.Configuration, data string) string {
	secret := &coreV1.Secret{}
	res, err := client.CoreV1().Secrets(configuration.Metadata.Namespace).Get(heimdallSecretName, metaV1.GetOptions{})

	if err != nil {
		reader := rand.Reader
		key, err := rsa.GenerateKey(reader, bitSize)

		if err != nil {
			glog.Fatal("Could not generate RSA key")
		}

		secret.SetName(heimdallSecretName)
		secret.SetNamespace(configuration.Metadata.Namespace)
		secret.StringData = make(map[string]string, 2)
		secret.StringData["publicKey"] = getPublicPEMKey(&key.PublicKey)
		secret.StringData["privateKey"] = getPEMKey(key)

		res, err = client.CoreV1().Secrets(configuration.Metadata.Namespace).Create(secret)
		if err != nil {
			glog.Fatal("Could not create secret")
		}
	}

	hash := sha512.New()
	ciphertext, err := rsa.EncryptOAEP(hash, rand.Reader, bytesToPublicKey([]byte(res.StringData["publicKey"])), []byte(data), nil)
	if err != nil {
		glog.Fatal("Could not encrypt data")
	}
	return string(ciphertext)
}

func bytesToPublicKey(pub []byte) *rsa.PublicKey {
	block, _ := pem.Decode(pub)
	enc := x509.IsEncryptedPEMBlock(block)
	b := block.Bytes
	var err error
	if enc {
		b, err = x509.DecryptPEMBlock(block, nil)
		if err != nil {
			glog.Fatal("Could not decrypt public key")
		}
	}
	ifc, err := x509.ParsePKIXPublicKey(b)
	if err != nil {
		glog.Fatal("Could not parse public key")
	}
	key, ok := ifc.(*rsa.PublicKey)
	if !ok {
		glog.Fatal("Could not create public key")
	}
	return key
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
