package deepseek

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// FetchLocal fetches a page as Markdown by invoking the lightpanda binary.
// It performs no fallback: if lightpanda is missing or the page fails to
// load, the error is returned to the caller as-is.
func FetchLocal(ctx context.Context, url string) (string, error) {
	if strings.TrimSpace(url) == "" {
		return "", errors.New("local fetch: empty url")
	}

	bin, err := exec.LookPath("lightpanda")
	if err != nil {
		return "", errors.New("local fetch: lightpanda not found in PATH; install lightpanda or drop --local")
	}

	// networkidle is the only wait strategy that produced non-empty
	// output consistently in testing; load and a fixed --wait-ms both
	// intermittently dumped an empty document on the same URL.
	args := []string{
		"fetch",
		"--dump", "markdown",
		"--wait-until", "networkidle",
		url,
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("local fetch: %w", err)
		}
		return "", fmt.Errorf("local fetch: %w: %s", err, msg)
	}

	text := stdout.String()
	if strings.TrimSpace(text) == "" {
		return "", errors.New("local fetch: lightpanda returned an empty document")
	}

	return text, nil
}
