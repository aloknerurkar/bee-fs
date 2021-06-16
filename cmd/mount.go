package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/aloknerurkar/bee-fs/pkg/api"
	"github.com/aloknerurkar/bee-fs/pkg/mounter"
	"github.com/spf13/cobra"
)

var mountCmd = &cobra.Command{
	Use:   "mount",
	Short: "Mount bee-fs endpoint",
}

func initMountCommands() *cobra.Command {
	mountOpts := api.CreateMountRequest{}

	mountCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Mount bee-fs endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Color("green")
			s.Start()

			info := mounter.MountInfo{}

			err := func() error {
				if mountOpts.Batch == "" {
					return errors.New("postage batch is required")
				}

				st, err := os.Stat(args[0])
				if err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("invalid mount path err: %w", err)
				}
				if os.IsNotExist(err) {
					err = os.MkdirAll(args[0], 0755)
					if err != nil {
						return fmt.Errorf("failed to create directory err: %w", err)
					}
				}
				if err == nil && !st.Mode().IsDir() {
					return errors.New("path exists as a file")
				}

				mountOpts.Path = args[0]

				reqBytes, err := json.Marshal(mountOpts)
				if err != nil {
					return fmt.Errorf("failed sending mount request err: %w", err)
				}

				req, err := http.NewRequestWithContext(cmd.Context(), "POST", "", bytes.NewBuffer(reqBytes))
				if err != nil {
					return fmt.Errorf("failed creating HTTP request err: %w", err)
				}

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("failed executing mount request err: %w", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusCreated {
					return errors.New("invalid status on mount create")
				}

				if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
					return fmt.Errorf("failed parsing response err: %w", err)
				}

				return nil
			}()

			s.Stop()

			if err != nil {
				return err
			}

			return nil
		},
	}

	mountCreateCmd.Flags().StringVar(&mountOpts.APIHost, "host", "127.0.0.1", "Bee API Host")
	mountCreateCmd.Flags().IntVar(&mountOpts.APIHostPort, "port", 1633, "Bee API port")
	mountCreateCmd.Flags().BoolVar(&mountOpts.APIUseSSL, "ssl", false, "use ssl")
	mountCreateCmd.Flags().BoolVar(&mountOpts.Encrypt, "encrypt", false, "use encryption")
	mountCreateCmd.Flags().StringVar(&mountOpts.Batch, "postage-batch", "", "Postage stamp batch ID")
	mountCreateCmd.Flags().StringVar(&mountOpts.Reference, "reference", "", "Use existing reference")
	mountCreateCmd.Flags().StringVar(&mountOpts.SnapshotPolicy, "snapshot-policy", "@daily", "snapshot policy")
	mountCreateCmd.Flags().IntVar(&mountOpts.KeepCount, "keep-count", 5, "no. of snapshots to retain")

	mountRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove mounted bee-fs endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Color("green")
			s.Start()

			err := func() error {
				req, err := http.NewRequestWithContext(cmd.Context(), "DELETE", "", nil)
				if err != nil {
					return fmt.Errorf("failed creating HTTP request err: %w", err)
				}

				q := req.URL.Query()
				q.Add("path", args[0])
				req.URL.RawQuery = q.Encode()

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("failed executing mount request err: %w", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return errors.New("invalid status returned by mount remove")
				}

				return nil
			}()

			s.Stop()

			if err != nil {
				return err
			}

			cmd.Println("Successfully unmounted " + args[0])

			return nil
		},
	}

	mountShowCmd := &cobra.Command{
		Use:   "get",
		Short: "Show mounted bee-fs endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Color("green")
			s.Start()

			info := mounter.MountInfo{}

			err := func() error {
				req, err := http.NewRequestWithContext(cmd.Context(), "GET", "", nil)
				if err != nil {
					return fmt.Errorf("failed creating HTTP request err: %w", err)
				}

				q := req.URL.Query()
				q.Add("path", args[0])
				req.URL.RawQuery = q.Encode()

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("failed executing mount request err: %w", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return errors.New("invalid status returned by mount remove")
				}

				if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
					return fmt.Errorf("failed parsing response err: %w", err)
				}

				return nil
			}()

			s.Stop()

			if err != nil {
				return err
			}

			return nil
		},
	}

	mountListCmd := &cobra.Command{
		Use:   "list",
		Short: "List mounted bee-fs endpoints",
		RunE: func(cmd *cobra.Command, args []string) error {

			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Color("green")
			s.Start()

			infos := []mounter.MountInfo{}

			err := func() error {
				req, err := http.NewRequestWithContext(cmd.Context(), "GET", "", nil)
				if err != nil {
					return fmt.Errorf("failed creating HTTP request err: %w", err)
				}

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("failed executing mount request err: %w", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return errors.New("invalid status returned by mount remove")
				}

				if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
					return fmt.Errorf("failed parsing response err: %w", err)
				}

				return nil
			}()

			s.Stop()

			if err != nil {
				return err
			}

			return nil
		},
	}

	mountCmd.AddCommand(mountCreateCmd)
	mountCmd.AddCommand(mountRemoveCmd)
	mountCmd.AddCommand(mountShowCmd)
	mountCmd.AddCommand(mountListCmd)

	return mountCmd
}
