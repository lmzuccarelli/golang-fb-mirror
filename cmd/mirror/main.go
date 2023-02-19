package main

import (
	"fmt"
	"os"

	cli "github.com/lmzuccarelli/golang-oci-mirror/pkg/cli"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
)

func main() {

	// setup pluggable logger
	// feel free to plugin you own logger
	// just use the PluggableLoggerInterface
	// in the file pkg/log/logger.go

	log := clog.New("info")
	rootCmd := cli.NewMirrorCmd(log)
	err := rootCmd.Execute()
	if err != nil {
		fmt.Println("ERROR : ", err)
		os.Exit(1)
	}
}
