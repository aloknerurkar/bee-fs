package main

import (
	"encoding/hex"
	"fmt"
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
		Use:   "snapshot <Reference> <Folder path to restore snapshot>",
		Short: "Restore bee-fs snapshot using reference",
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

	var SnapshotRef string

	fileRestoreCmd := &cobra.Command{
		Use:   "file <File path from root of snapshot> <Folder path to restore file>",
		Short: "Restore file from bee-fs snapshot",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {

			s := spinner.New(
				spinner.CharSets[9],
				100*time.Millisecond,
				spinner.WithSuffix("restoring"),
			)
			s.Color("green")
			s.Start()

			srcFile := args[0]
			dstPath := ""
			err := func() error {
				if len(args) == 2 {
					dstPath = args[1]
				}
				addrBytes, err := hex.DecodeString(SnapshotRef)
				if err != nil {
					return fmt.Errorf("invalid snapshot reference %w", err)
				}
				bStore := beestore.NewAPIStore(
					APIHost,
					APIHostPort,
					APIUseSSL,
					"",
				)
				return fs.RestoreFile(cmd.Context(), swarm.NewAddress(addrBytes), srcFile, dstPath, bStore, Encrypted)
			}()

			s.Stop()

			if err != nil {
				return err
			}

			cmd.Println("Successfully restored file", filepath.Base(args[0]), "from snapshot", SnapshotRef)

			return nil
		},
	}

	fileRestoreCmd.Flags().StringVar(&SnapshotRef, "snapshot", "", "Swarm address of snapshot")
	fileRestoreCmd.MarkFlagRequired("snapshot")

	restoreCmd.PersistentFlags().StringVar(&APIHost, "host", "127.0.0.1", "Bee API Host")
	restoreCmd.PersistentFlags().IntVar(&APIHostPort, "port", 1633, "Bee API port")
	restoreCmd.PersistentFlags().BoolVar(&APIUseSSL, "ssl", false, "use ssl")
	restoreCmd.PersistentFlags().BoolVar(&Encrypted, "encrypted", false, "is encrypted")

	restoreCmd.AddCommand(snapRestoreCmd)
	restoreCmd.AddCommand(fileRestoreCmd)

	root.AddCommand(restoreCmd)
}
