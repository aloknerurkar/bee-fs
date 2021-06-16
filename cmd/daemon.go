package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/aloknerurkar/bee-fs/pkg/api"
	"github.com/aloknerurkar/bee-fs/pkg/mounter"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start bee-fs daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		mntr := mounter.New()
		router := api.NewRouter(mntr)

		srv := &http.Server{
			Addr: "0.0.0.0:8080",
			// Good practice to set timeouts to avoid Slowloris attacks.
			WriteTimeout: time.Second * 15,
			ReadTimeout:  time.Second * 15,
			IdleTimeout:  time.Second * 60,
			Handler:      router, // Pass our instance of gorilla/mux in.
		}

		// Run our server in a goroutine so that it doesn't block.
		go func() {
			if err := srv.ListenAndServe(); err != nil {
				// log.Println(err)
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
