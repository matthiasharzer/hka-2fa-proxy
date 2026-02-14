package run

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/MatthiasHarzer/hka-2fa-proxy/otp"
	"github.com/MatthiasHarzer/hka-2fa-proxy/proxy"
	"github.com/spf13/cobra"
)

var username string
var otpSecret string
var port int
var targetURL string
var skipInitialAuth bool

func init() {
	Command.Flags().StringVarP(&username, "username", "u", "", "The username to use for authentication")
	Command.Flags().StringVarP(&otpSecret, "secret", "s", "", "The OTP-secret to use for generating the OTPs")
	Command.Flags().IntVarP(&port, "port", "p", 8080, "The port to run the proxy on")
	Command.Flags().StringVarP(&targetURL, "target", "t", "https://owa.h-ka.de", "The target url to proxy to")
	Command.Flags().BoolVarP(&skipInitialAuth, "skip-initial-auth", "", false, "Whether to skip the initial authentication when starting the proxy")
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

		server, err := proxy.NewServer(targetURL, username, generator, skipInitialAuth)
		if err == nil {
			return fmt.Errorf("initial authentication failed: %w", err)
		}

		log.Printf("starting server on port %d\n", port)

		err = http.ListenAndServe(fmt.Sprintf(":%d", port), server)
		return err
	},
}
