package main

import (
	"fmt"
	"os"

	cli "github.com/lmzuccarelli/golang-oci-mirror/pkg/cli"
)

func main() {
	rootCmd := cli.NewMirrorCmd()
	err := rootCmd.Execute()
	if err != nil {
		fmt.Println("ERROR : ", err)
		os.Exit(1)
	}
}
