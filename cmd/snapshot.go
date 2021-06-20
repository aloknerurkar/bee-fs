package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aloknerurkar/bee-fs/pkg/mounter"
	"github.com/briandowns/spinner"
	"github.com/cheynewallace/tabby"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

func initSnapshotCommands(root *cobra.Command) {
	snapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "bee-fs endpoint snapshot related commands",
	}

	snapCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create bee-fs endpoint snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Color("green")
			s.Start()

			var snap swarm.Address

			err := func() error {
				url := strings.Join([]string{apiHost, "snapshot"}, "/")

				req, err := http.NewRequestWithContext(cmd.Context(), "POST", url, nil)
				if err != nil {
					return fmt.Errorf("failed creating HTTP request err: %w", err)
				}
				q := req.URL.Query()
				q.Add("path", args[0])
				req.URL.RawQuery = q.Encode()

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("failed executing snapshot request err: %w", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return errors.New("invalid status returned by snapshot create")
				}

				if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
					return fmt.Errorf("failed parsing response err: %w", err)
				}

				return nil
			}()

			s.Stop()

			if err != nil {
				return err
			}

			cmd.Println("Successfully created snapshot with reference", snap.String())

			return nil
		},
	}

	snapInfoCmd := &cobra.Command{
		Use:   "info",
		Short: "Get bee-fs endpoint snapshot information",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Color("green")
			s.Start()

			var snapInfos []mounter.SnapshotInfo

			err := func() error {
				url := strings.Join([]string{apiHost, "snapshot"}, "/")

				req, err := http.NewRequestWithContext(cmd.Context(), "GET", url, nil)
				if err != nil {
					return fmt.Errorf("failed creating HTTP request err: %w", err)
				}
				q := req.URL.Query()
				q.Add("path", args[0])
				req.URL.RawQuery = q.Encode()

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("failed executing snapshot request err: %w", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return errors.New("invalid status returned by snapshot create")
				}

				if err := json.NewDecoder(resp.Body).Decode(&snapInfos); err != nil {
					return fmt.Errorf("failed parsing response err: %w", err)
				}

				return nil
			}()

			s.Stop()

			if err != nil {
				return err
			}

			showSnapshotInfo(cmd, snapInfos...)

			return nil
		},
	}

	snapshotCmd.AddCommand(snapCreateCmd)
	snapshotCmd.AddCommand(snapInfoCmd)

	root.AddCommand(snapshotCmd)
}

func showSnapshotInfo(cmd *cobra.Command, snaps ...mounter.SnapshotInfo) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	t := tabby.NewCustom(w)
	t.AddHeader("NAME", "CREATED", "REFERENCE", "SYNCED")
	for _, s := range snaps {
		syncedPercent := fmt.Sprintf("%d%%", (s.Stats.Synced * 100 / s.Stats.Processed))
		t.AddLine(s.Name, s.Timestamp, s.Reference, syncedPercent)
	}
}
