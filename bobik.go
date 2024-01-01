package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/keybase/go-keychain"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/term"
)

type SecretData struct {
	Password string `json:"password"`
	TOTPURL  string `json:"totp_url"`
}

const graceTimeout = 5 * time.Second

func run() error {
	service := "bobik"
	account := os.Args[len(os.Args)-1]
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(service)
	query.SetAccount(account)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnAttributes(true)
	query.SetReturnData(true)
	results, err := keychain.QueryItem(query)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	} else {
		for _, r := range results {
			// fmt.Printf("%#v\n", r)
			data := SecretData{}
			if err := json.Unmarshal(r.Data, &data); err != nil {
				return err
			}
			// fmt.Printf("data: %+v\n", data)
			key, err := otp.NewKeyFromURL(data.TOTPURL)
			if err != nil {
				return err
			}
			code, err := totp.GenerateCode(key.Secret(), time.Now().Add(-graceTimeout))
			if err != nil {
				return err
			}
			// fmt.Printf("OTP: %s\n", code)

			cmd := exec.Command(os.Args[1], os.Args[2:]...)

			// Start the command with a pty.
			ptmx, err := pty.Start(cmd)
			if err != nil {
				return fmt.Errorf("execute ssh: %w", err)
			}
			// Make sure to close the pty at the end.
			defer func() { ptmx.Close() }() // Best effort.

			// Handle pty size.
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, syscall.SIGWINCH)
			go func() {
				for range ch {
					if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
						log.Printf("error resizing pty: %s", err)
					}
				}
			}()
			ch <- syscall.SIGWINCH                        // Initial resize.
			defer func() { signal.Stop(ch); close(ch) }() // Cleanup signals when done.

			// Set stdin in raw mode.
			oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
			if err != nil {
				panic(err)
			}
			defer func() { term.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

			// Copy stdin to the pty and the pty to stdout.
			go func() {
				_, _ = io.Copy(ptmx, os.Stdin)
			}()

			pf := NewPromptFinder(ptmx, func(s string) bool {
				return strings.HasSuffix(s, ":")
			})

			go func() {
				defer pf.Stop()
				<-pf.Found
				_, err := fmt.Fprintf(ptmx, "%s\n", data.Password)
				if err != nil {
					log.Fatalf("write bobi command to the remote: %v", err)
				}
				<-pf.Found
				_, err = fmt.Fprintf(ptmx, "%s\n", code)
				if err != nil {
					log.Fatalf("write bobi command to the remote: %v", err)
				}
			}()
			_, _ = io.Copy(os.Stdout, pf)
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
