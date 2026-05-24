package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// addAPIFlags adds the --api-url, --username, --password flags to a command.
// Same precedence as the rest of the CLI: flag → env → default.
func addAPIFlags(c *cobra.Command) {
	c.PersistentFlags().String("api-url", "", "Base URL of the videonode daemon (overrides VIDEONODE_API_URL)")
	c.PersistentFlags().String("username", "", "Basic-auth username (overrides VIDEONODE_USERNAME)")
	c.PersistentFlags().String("password", "", "Basic-auth password (overrides VIDEONODE_PASSWORD)")
}

// apiClient bundles the HTTP target + credentials for one CLI invocation.
type apiClient struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

// newAPIClient resolves the API target from flags + env + defaults.
func newAPIClient(c *cobra.Command) *apiClient {
	base, _ := c.Flags().GetString("api-url")
	if base == "" {
		base = os.Getenv("VIDEONODE_API_URL")
	}
	if base == "" {
		base = "http://localhost:8090"
	}
	user, _ := c.Flags().GetString("username")
	if user == "" {
		user = os.Getenv("VIDEONODE_USERNAME")
	}
	if user == "" {
		user = "videonode"
	}
	pass, _ := c.Flags().GetString("password")
	if pass == "" {
		pass = os.Getenv("VIDEONODE_PASSWORD")
	}
	if pass == "" {
		pass = "videonode"
	}
	return &apiClient{
		baseURL:  strings.TrimRight(base, "/"),
		username: user,
		password: pass,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

// do executes an HTTP request, decoding the JSON response body into out (if non-nil).
// Returns an error for non-2xx statuses with the response body as context.
func (a *apiClient) do(method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, a.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(a.username, a.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// printJSON writes pretty-printed JSON to the command's stdout.
func printJSON(c *cobra.Command, v any) error {
	enc := json.NewEncoder(c.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// readJSONBody reads a JSON document from --file or stdin (when --file is empty
// or "-"). Returns a parsed any so the CLI doesn't need to know each entity's
// concrete schema.
func readJSONBody(c *cobra.Command) (any, error) {
	path, _ := c.Flags().GetString("file")
	var data []byte
	var err error
	switch path {
	case "", "-":
		data, err = io.ReadAll(c.InOrStdin())
	default:
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("empty request body")
	}
	var body any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("parse json body: %w", err)
	}
	return body, nil
}
