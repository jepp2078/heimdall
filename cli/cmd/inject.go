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
	"github.com/spf13/cobra"
)

var configFile string

// injectCmd represents the inject command
var injectCmd = &cobra.Command{
	Use:   "inject",
	Short: "Inject encrypted values into config files",
	Long:  ``,
	Run:   run,
}

func init() {
	rootCmd.AddCommand(injectCmd)

	rootCmd.Flags().StringVar(&configFile, "config", "", "Heimdall config file to be injected")
}

func run(cmd *cobra.Command, args []string) {
	GetKubernetesClient()
}
