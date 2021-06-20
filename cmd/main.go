// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
)

var apiHost string

func main() {
	c := &cobra.Command{
		Short:        "Used for FileSystem interface with swarm network",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			hostPath := os.Getenv("API")
			if hostPath == "" {
				hostPath = ""
			}
			hostAddr, err := ioutil.ReadFile(hostPath)
			if err != nil {
				return err
			}
			apiHost = string(hostAddr)
			return nil
		},
	}

	initMountCommands(c)
	initSnapshotCommands(c)

	c.SetOutput(c.OutOrStdout())
	err := c.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
