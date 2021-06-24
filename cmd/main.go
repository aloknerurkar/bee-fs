// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

var apiHost string

func getApiFilepath() string {
	hostPath := os.Getenv("API")
	if hostPath == "" {
		hPath, err := homedir.Dir()
		if err != nil {
			return ""
		}
		hostPath = hPath + string(os.PathSeparator) + ".bee-fs" + string(os.PathSeparator) + "api"
	}
	return hostPath
}

func main() {
	c := &cobra.Command{
		Short:        "Used for FileSystem interface with swarm network",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			hostPath := getApiFilepath()
			hostAddr, err := ioutil.ReadFile(hostPath)
			if err == nil {
				apiHost = string(hostAddr)
			}
			return nil
		},
	}

	initDaemon(c)
	initMountCommands(c)
	initSnapshotCommands(c)
	initRestoreCommands(c)

	c.SetOutput(c.OutOrStdout())
	err := c.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
