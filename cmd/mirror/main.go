package main

import (
	"fmt"
	"os"

	"github.com/lmzuccarelli/golang-oci-mirror/pkg/services"
)

func main() {
	rootCmd := services.NewMirrorCmd()
	err := rootCmd.Execute()
	if err != nil {
		fmt.Println("ERROR : ", err)
		os.Exit(1)
	}
}
