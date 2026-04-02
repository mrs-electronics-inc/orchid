package orchidcli

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	serverSocketPath  = "/run/orchid/orchid.sock"
	serverSocketGroup = "orchid"
	serverUnitName    = "orchid.service"
	serverBaseLink    = "/var/lib/libvirt/images/orchid-base.qcow2"
)

//go:embed systemd/orchid.service
var serverUnitFS embed.FS

var serverJobStore = newDaemonJobStore()

func runServer(args []string) int {
	if len(args) < 1 {
		usageServer()
	}

	switch args[0] {
	case "install":
		return runServerInstall(args[1:])
	case "build-base":
		return runServerBuildBase(args[1:])
	case "proxy":
		return runServerProxy(args[1:])
	case "run":
		return runServerRun(args[1:])
	case "status":
		return runServerStatus(args[1:])
	case "-h", "--help", "help":
		usageServer()
	default:
		fmt.Fprintf(os.Stderr, "unknown server command: %s\n\n", args[0])
		usageServer()
	}

	return 0
}

func usageServer() {
	fmt.Fprintln(os.Stderr, "usage: orchid server <install|build-base|proxy|run|status>")
	os.Exit(2)
}

func runServerRun(args []string) int {
	if len(args) != 0 {
		usageServer()
	}

	if err := serveOrchidDaemon(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func serveOrchidDaemon() error {
	if err := os.MkdirAll(filepath.Dir(serverSocketPath), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(serverSocketPath), err)
	}
	if err := os.Remove(serverSocketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing stale socket %s: %w", serverSocketPath, err)
	}

	listener, err := net.Listen("unix", serverSocketPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", serverSocketPath, err)
	}
	defer listener.Close()
	defer os.Remove(serverSocketPath)
	// The SSH proxy runs as trusted hypervisor users, so the socket is shared
	// with the orchid group rather than made world-writable.
	if err := setOrchidSocketPermissions(serverSocketPath); err != nil {
		return fmt.Errorf("setting %s permissions: %w", serverSocketPath, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", handleHealth)
	mux.HandleFunc("/v1/vms", handleVMs)
	mux.HandleFunc("/v1/vms/", handleVMByName)
	mux.HandleFunc("/v1/jobs/", handleJob)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return server.Serve(listener)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleVMs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/vms" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		output, err := runVirshCommand("list", "--all")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		vms := parseVirshList(output)
		resp := struct {
			VMs []daemonVM `json:"vms"`
		}{VMs: make([]daemonVM, 0, len(vms))}
		for _, vm := range vms {
			managed, err := domainIsOrchidVM(vm.Name)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("checking %s metadata: %v", vm.Name, err))
				return
			}
			if !managed {
				continue
			}
			resp.VMs = append(resp.VMs, daemonVM{Name: vm.Name, State: vm.State})
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req daemonCreateVMRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("decoding create-vm request: %v", err))
			return
		}

		job, err := serverJobStore.startCreateVM(req)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		writeJSON(w, http.StatusAccepted, daemonCreateVMResponse{JobID: job.snapshot().JobID})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func handleJob(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/v1/jobs/") {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	jobID := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	if jobID == "" || strings.Contains(jobID, "/") {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	jobID, err := url.PathUnescape(jobID)
	if err != nil || jobID == "" || strings.Contains(jobID, "/") {
		writeJSONError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	job, ok := serverJobStore.get(jobID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "job not found")
		return
	}

	writeJSON(w, http.StatusOK, job.snapshot())
}

func handleVMByName(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/v1/vms/") {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/vms/")
	name, suffix, found := strings.Cut(trimmed, "/")
	if name == "" || strings.Contains(name, "/") {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	name, err := url.PathUnescape(name)
	if err != nil || name == "" || strings.Contains(name, "/") {
		writeJSONError(w, http.StatusBadRequest, "invalid VM name")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !found || suffix != "ip" {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}

		if _, err := runVirshCommand("dominfo", name); err != nil {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		managed, err := domainIsOrchidVM(name)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("checking %s metadata: %v", name, err))
			return
		}
		if !managed {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}

		ip, err := resolveVMIP(name)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"vm_name": name,
			"ip":      ip,
		})
	case http.MethodDelete:
		if found {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}

		if _, err := runVirshCommand("dominfo", name); err == nil {
			managed, err := domainIsOrchidVM(name)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("checking %s metadata: %v", name, err))
				return
			}
			if !managed {
				writeJSONError(w, http.StatusNotFound, "not found")
				return
			}
		}

		if err := destroyVM(name); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func runVirshCommand(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "virsh", append([]string{"-c", "qemu:///system"}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("virsh %s timed out after %s", strings.Join(args, " "), 10*time.Second)
		}
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", fmt.Errorf("virsh %s failed: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("virsh %s failed: %s", strings.Join(args, " "), trimmed)
	}
	return strings.TrimSpace(string(output)), nil
}

func resolveVMIP(vmName string) (string, error) {
	domifaddr, err := runVirshCommand("domifaddr", vmName)
	if err != nil {
		return "", err
	}
	if ip := parseDomifaddr(domifaddr); ip != "" {
		return ip, nil
	}

	domiflist, err := runVirshCommand("domiflist", vmName)
	if err != nil {
		return "", err
	}
	mac := parseMAC(domiflist)
	if mac == "" {
		return "", fmt.Errorf("no MAC address found for %s", vmName)
	}

	leases, err := runVirshCommand("net-dhcp-leases", "default")
	if err != nil {
		return "", err
	}
	if ip := parseLeaseIP(leases, mac); ip != "" {
		return ip, nil
	}

	return "", fmt.Errorf("lease not found for MAC %s", mac)
}

func runServerProxy(args []string) int {
	if len(args) != 0 {
		usageServer()
	}

	conn, err := net.Dial("unix", serverSocketPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer conn.Close()

	unixConn, _ := conn.(*net.UnixConn)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, os.Stdin)
		if unixConn != nil {
			_ = unixConn.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stdout, conn)
		if unixConn != nil {
			_ = unixConn.CloseRead()
		}
	}()

	wg.Wait()
	return 0
}

func runServerInstall(args []string) int {
	if len(args) != 0 {
		usageServer()
	}

	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "orchid server install must be run with sudo")
		return 1
	}

	if err := runCommandChecked("groupadd", "--system", "--force", serverSocketGroup); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := ensureBaseImagePresent(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	unit, err := serverUnitFS.ReadFile("systemd/orchid.service")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	unitPath := filepath.Join("/etc/systemd/system", serverUnitName)

	tmpUnit, err := os.CreateTemp("", "orchid-service-*.service")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	tmpUnitPath := tmpUnit.Name()
	if _, err := tmpUnit.Write(unit); err != nil {
		tmpUnit.Close()
		os.Remove(tmpUnitPath)
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := tmpUnit.Close(); err != nil {
		os.Remove(tmpUnitPath)
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.Remove(tmpUnitPath)

	if err := installFile(tmpUnitPath, unitPath, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := runCommandChecked("systemctl", "daemon-reload"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := runCommandChecked("systemctl", "enable", "--now", serverUnitName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Restart here so an existing running service picks up the refreshed unit.
	if err := runCommandChecked("systemctl", "restart", serverUnitName); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("Installed %s and refreshed it.\n", serverUnitName)
	fmt.Println("Run `orchid server status` to confirm the daemon is active.")
	return 0
}

func runServerBuildBase(args []string) int {
	if len(args) != 0 {
		usageServer()
	}

	if err := buildOrchidBaseImage(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func ensureBaseImagePresent() error {
	info, err := os.Lstat(serverBaseLink)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(serverBaseLink); err == nil {
				return nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("checking %s: %w", serverBaseLink, err)
			}
			return buildOrchidBaseImage()
		}
		return fmt.Errorf("refusing to overwrite non-symlink base image at %s", serverBaseLink)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking %s: %w", serverBaseLink, err)
	}

	return buildOrchidBaseImage()
}

func runServerStatus(args []string) int {
	if len(args) != 0 {
		usageServer()
	}

	active := strings.TrimSpace(runCommandOutput("systemctl", "is-active", serverUnitName))
	enabled := strings.TrimSpace(runCommandOutput("systemctl", "is-enabled", serverUnitName))
	if active == "" {
		active = "unknown"
	}
	if enabled == "" {
		enabled = "unknown"
	}

	fmt.Printf("%s: enabled=%s active=%s\n", serverUnitName, enabled, active)
	return 0
}

func installFile(srcPath, dstPath string, mode os.FileMode) error {
	return runCommandChecked("install", "-m", fmt.Sprintf("%04o", mode), srcPath, dstPath)
}

func runCommandChecked(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("%s failed: %w", strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s failed: %s", strings.Join(args, " "), trimmed)
	}
	return nil
}

func runCommandOutput(args ...string) string {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}

func setOrchidSocketPermissions(path string) error {
	group, err := user.LookupGroup(serverSocketGroup)
	if err != nil {
		return fmt.Errorf("looking up group %s: %w", serverSocketGroup, err)
	}

	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return fmt.Errorf("parsing gid for %s: %w", serverSocketGroup, err)
	}

	if err := os.Chown(path, 0, gid); err != nil {
		return fmt.Errorf("setting ownership for %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o660); err != nil {
		return fmt.Errorf("setting mode for %s: %w", path, err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
