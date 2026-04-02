package orchidcli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

type daemonVMIPResponse struct {
	VMName string `json:"vm_name"`
	IP     string `json:"ip"`
}

func fetchDaemonVMs(hypervisor string) ([]daemonVM, error) {
	var response daemonVMsResponse
	if err := daemonJSONRequest(hypervisor, http.MethodGet, "/v1/vms", nil, &response); err != nil {
		return nil, err
	}
	return response.VMs, nil
}

func fetchDaemonVMIP(hypervisor, vmName string) (string, error) {
	var response daemonVMIPResponse
	if err := daemonJSONRequest(hypervisor, http.MethodGet, "/v1/vms/"+url.PathEscape(vmName)+"/ip", nil, &response); err != nil {
		return "", err
	}
	return response.IP, nil
}

func submitDaemonCreateVM(hypervisor string, req daemonCreateVMRequest) (daemonCreateVMResponse, error) {
	var response daemonCreateVMResponse
	body, err := json.Marshal(req)
	if err != nil {
		return response, err
	}
	if err := daemonJSONRequest(hypervisor, http.MethodPost, "/v1/vms", body, &response); err != nil {
		return response, err
	}
	return response, nil
}

func submitDaemonDestroyVM(hypervisor, vmName string) error {
	return daemonJSONRequest(hypervisor, http.MethodDelete, "/v1/vms/"+url.PathEscape(vmName), nil, nil)
}

func fetchDaemonJob(hypervisor, jobID string) (daemonJobStatus, error) {
	var response daemonJobStatus
	if err := daemonJSONRequest(hypervisor, http.MethodGet, "/v1/jobs/"+url.PathEscape(jobID), nil, &response); err != nil {
		return response, err
	}
	return response, nil
}

func daemonJSONRequest(hypervisor, method, path string, payload []byte, response any) error {
	resp, err := daemonRequest(hypervisor, method, path, payload)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if len(body) == 0 {
			if closeErr != nil {
				return closeErr
			}
			return fmt.Errorf("daemon request %s %s failed: %s", method, path, resp.Status)
		}
		if closeErr != nil {
			return closeErr
		}
		return fmt.Errorf("daemon request %s %s failed: %s: %s", method, path, resp.Status, strings.TrimSpace(string(body)))
	}

	if response == nil {
		io.Copy(io.Discard, resp.Body)
		if err := resp.Body.Close(); err != nil {
			return err
		}
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		_ = resp.Body.Close()
		return fmt.Errorf("decoding daemon response for %s %s: %w", method, path, err)
	}
	if err := resp.Body.Close(); err != nil {
		return err
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
		waitErr := cmd.Wait()
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText != "" {
			if waitErr != nil {
				return nil, fmt.Errorf("reading daemon response via %s failed: %w: %s", hypervisor, err, stderrText)
			}
			return nil, fmt.Errorf("reading daemon response via %s failed: %w: %s", hypervisor, err, stderrText)
		}
		if waitErr != nil {
			return nil, fmt.Errorf("reading daemon response via %s failed: %w", hypervisor, err)
		}
		return nil, fmt.Errorf("reading daemon response via %s failed: %w", hypervisor, err)
	}
	resp.Body = &daemonResponseBody{
		ReadCloser: resp.Body,
		cmd:        cmd,
		stderr:     &stderr,
		hypervisor: hypervisor,
	}
	return resp, nil
}

type daemonResponseBody struct {
	io.ReadCloser
	cmd        *exec.Cmd
	stderr     *bytes.Buffer
	hypervisor string
}

func (b *daemonResponseBody) Close() error {
	closeErr := b.ReadCloser.Close()
	waitErr := b.cmd.Wait()
	if waitErr != nil {
		stderrText := strings.TrimSpace(b.stderr.String())
		if stderrText != "" {
			return fmt.Errorf("daemon proxy via %s failed: %w: %s", b.hypervisor, waitErr, stderrText)
		}
		return fmt.Errorf("daemon proxy via %s failed: %w", b.hypervisor, waitErr)
	}
	return closeErr
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
