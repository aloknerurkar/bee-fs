package main

import (
	"encoding/hex"
	"path/filepath"
	"time"

	"github.com/aloknerurkar/bee-fs/pkg/fuse"
	"github.com/aloknerurkar/bee-fs/pkg/store/beestore"
	"github.com/briandowns/spinner"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

func initRestoreCommands(root *cobra.Command) {
	restoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "bee-fs endpoint snapshot restore commands",
	}

	var (
		APIHost     string
		APIHostPort int
		APIUseSSL   bool
		Encrypted   bool
	)

	snapRestoreCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Create bee-fs endpoint snapshot",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {

			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Color("green")
			s.Start()

			dstFile := args[0] + ".tar.gz"
			err := func() error {
				if len(args) == 2 {
					dstFile = filepath.Join(args[1], dstFile)
				}
				addrBytes, err := hex.DecodeString(args[0])
				if err != nil {
					return err
				}
				bStore := beestore.NewAPIStore(
					APIHost,
					APIHostPort,
					APIUseSSL,
					"",
				)
				return fs.Restore(cmd.Context(), swarm.NewAddress(addrBytes), dstFile, bStore, Encrypted)
			}()

			s.Stop()

			if err != nil {
				return err
			}

			cmd.Println("Successfully restored snapshot", args[0], "at", dstFile)

			return nil
		},
	}

	snapRestoreCmd.Flags().StringVar(&APIHost, "host", "127.0.0.1", "Bee API Host")
	snapRestoreCmd.Flags().IntVar(&APIHostPort, "port", 1633, "Bee API port")
	snapRestoreCmd.Flags().BoolVar(&APIUseSSL, "ssl", false, "use ssl")
	snapRestoreCmd.Flags().BoolVar(&Encrypted, "encrypted", false, "is encrypted")

	restoreCmd.AddCommand(snapRestoreCmd)

	root.AddCommand(restoreCmd)
}
