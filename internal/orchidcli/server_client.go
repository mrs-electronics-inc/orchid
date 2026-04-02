package orchidcli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
)

type daemonVM struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type daemonVMsResponse struct {
	VMs []daemonVM `json:"vms"`
}

func fetchDaemonVMs(hypervisor string) ([]daemonVM, error) {
	var response daemonVMsResponse
	if err := daemonJSONRequest(hypervisor, http.MethodGet, "/v1/vms", nil, &response); err != nil {
		return nil, err
	}
	return response.VMs, nil
}

func daemonJSONRequest(hypervisor, method, path string, payload []byte, response any) error {
	resp, err := daemonRequest(hypervisor, method, path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		if len(body) == 0 {
			return fmt.Errorf("daemon request %s %s failed: %s", method, path, resp.Status)
		}
		return fmt.Errorf("daemon request %s %s failed: %s: %s", method, path, resp.Status, strings.TrimSpace(string(body)))
	}

	if response == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return fmt.Errorf("decoding daemon response for %s %s: %w", method, path, err)
	}
	return nil
}

func daemonRequest(hypervisor, method, path string, payload []byte) (*http.Response, error) {
	req, err := http.NewRequest(method, "http://orchid"+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Host = "orchid"
	req.Header.Set("Connection", "close")
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	args := append(sshBaseArgs(hypervisor), "orchid", "server", "proxy")
	cmd := exec.Command("ssh", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		_ = req.Write(stdin)
		_ = stdin.Close()
	}()

	resp, err := http.ReadResponse(bufio.NewReader(stdout), req)
	if err != nil {
		_ = cmd.Wait()
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText != "" {
			return nil, fmt.Errorf("reading daemon response via %s failed: %w: %s", hypervisor, err, stderrText)
		}
		return nil, fmt.Errorf("reading daemon response via %s failed: %w", hypervisor, err)
	}

	if err := cmd.Wait(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText != "" {
			return nil, fmt.Errorf("daemon proxy via %s failed: %w: %s", hypervisor, err, stderrText)
		}
		return nil, fmt.Errorf("daemon proxy via %s failed: %w", hypervisor, err)
	}

	return resp, nil
}

func sshBaseArgs(hypervisor string) []string {
	args := []string{
		"-T",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		hypervisor,
	}
	return args
}
