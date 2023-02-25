/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"flag"
	"os"

	"github.com/Shuanglu/namespace-termination-locker/pkg/server"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var tlsSecret string
var v string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "namespace-termination-locker",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		server.Server()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.namespace-termination-locker.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.

	cobra.OnInitialize(initConfig)

	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	// rootCmd.PersistentFlags().StringVar(&tlsSecret, "tlsSecret", "namespace-termination-locker-webhook", "secret name of the tls cert/key")
	rootCmd.PersistentFlags().StringP("v", "v", "0", "log level")
}

func initConfig() {

	if v != "0" {
		flag.Set("v", v)
		klog.InitFlags(nil)
		defer klog.Flush()
		flag.Parse()
	}
}
