package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var kubeConfigFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "heimdall-cli",
	Short: "CLI to interface with deployed Heimdall",
	Long:  `CLI responsible for encrypting and injecting configuration values and deploying Heimdall to clusters.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeConfigFile, "kube-config", "", "kube-config file (default is $HOME/.kube/config)")
}

func GetKubernetesClient() kubernetes.Interface {
	var kubeconfig *string
	if kubeConfigFile == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		}
	} else {
		kubeconfig = &kubeConfigFile
	}

	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)

	if err != nil {
		fmt.Println("getClusterConfig:", err)
	}

	//Set insecure to be able to communicate with self signed certs
	config.CAData = nil
	config.Insecure = true

	// generate the client based off of the config
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println("getClusterConfig: %v", err)
	}
	return client
}
