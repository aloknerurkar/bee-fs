package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/aloknerurkar/bee-fs/pkg/api"
	"github.com/aloknerurkar/bee-fs/pkg/mounter"
	"github.com/spf13/cobra"
)

func initDaemon(cmd *cobra.Command) {
	var (
		port        int
		apiFilePath string
	)

	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start bee-fs daemon",
		RunE: func(cmd *cobra.Command, args []string) error {

			if apiFilePath == "" {
				apiFilePath = getApiFilepath()
				err := os.MkdirAll(filepath.Dir(apiFilePath), 0755)
				if err != nil {
					return err
				}
			}

			addr := fmt.Sprintf("http://localhost:%d", port)

			err := ioutil.WriteFile(apiFilePath, []byte(addr), 0755)
			if err != nil {
				return fmt.Errorf("failed creating api file err: %w", err)
			}

			mntr := mounter.New()
			router := api.NewRouter(mntr)

			defer mntr.Close()

			srv := &http.Server{
				Addr: fmt.Sprintf("0.0.0.0:%d", port),
				// Good practice to set timeouts to avoid Slowloris attacks.
				WriteTimeout: time.Second * 15,
				ReadTimeout:  time.Second * 15,
				IdleTimeout:  time.Second * 60,
				Handler:      router, // Pass our instance of gorilla/mux in.
			}

			// Run our server in a goroutine so that it doesn't block.
			go func() {
				if err := srv.ListenAndServe(); err != nil {
					fmt.Println(err)
				}
			}()

			c := make(chan os.Signal, 1)
			// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
			// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
			signal.Notify(c, os.Interrupt)

			// Block until we receive our signal.
			<-c

			// Create a deadline to wait for.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
			defer cancel()
			// Doesn't block if no connections, but will otherwise wait
			// until the timeout deadline.
			srv.Shutdown(ctx)

			return nil
		},
	}

	daemonCmd.Flags().IntVar(&port, "port", 8080, "Bee-fs API port")
	daemonCmd.Flags().StringVar(&apiFilePath, "api-file-path", "", "Bee API file")

	cmd.AddCommand(daemonCmd)
}
