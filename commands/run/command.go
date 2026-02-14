package run

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/MatthiasHarzer/hka-2fa-proxy/otp"
	"github.com/MatthiasHarzer/hka-2fa-proxy/proxy"
	"github.com/spf13/cobra"
)

var username string
var otpSecret string
var port int
var targetURL string
var retryOnAuthFailure bool
var maxRetries int
var retryDelay = 30

func init() {
	Command.Flags().StringVarP(&username, "username", "u", "", "The username to use for authentication")
	Command.Flags().StringVarP(&otpSecret, "secret", "s", "", "The OTP-secret to use for generating the OTPs")
	Command.Flags().IntVarP(&port, "port", "p", 8080, "The port to run the proxy on")
	Command.Flags().StringVarP(&targetURL, "target", "t", "https://owa.h-ka.de", "The target url to proxy to")
	Command.Flags().BoolVarP(&retryOnAuthFailure, "retry-on-auth-failure", "r", false, "Whether to retry the request if authentication fails")
	Command.Flags().IntVarP(&maxRetries, "max-retries", "m", 3, "The maximum number of retries if authentication fails")
	Command.Flags().IntVarP(&retryDelay, "retry-delay", "d", 30, "The delay in seconds between retries if authentication fails")
}

var Command = &cobra.Command{
	Use:   "run",
	Short: "Runs the proxy server",
	Long:  "Runs the proxy server",
	RunE: func(c *cobra.Command, args []string) error {
		if username == "" {
			return errors.New("username is required")
		}
		if otpSecret == "" {
			return errors.New("otp-secret is required")
		}

		generator, err := otp.NewGenerator(otpSecret)
		if err != nil {
			return err
		}

		if strings.HasSuffix(targetURL, "/") {
			targetURL = targetURL[:len(targetURL)-1]
		}

		var server http.Handler

		retries := 0
		for {
			var err error
			server, err = proxy.NewServer(targetURL, username, generator)
			if err == nil {
				break
			}

			if !retryOnAuthFailure || retries >= maxRetries {
				return fmt.Errorf("could not create proxy server: %w", err)
			}

			retries++
			log.Printf("authentication failed, retrying in %d seconds... (attempt %d/%d)\n", retryDelay, retries, maxRetries)
			time.Sleep(time.Duration(retryDelay) * time.Second)
			continue
		}

		log.Printf("starting server on port %d\n", port)

		err = http.ListenAndServe(fmt.Sprintf(":%d", port), server)
		return err
	},
}
