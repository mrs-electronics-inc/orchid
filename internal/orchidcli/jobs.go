package cli

const (
	daemonJobStateQueued    = "queued"
	daemonJobStateRunning   = "running"
	daemonJobStateSucceeded = "succeeded"
	daemonJobStateFailed    = "failed"

	daemonJobStageValidatingRequest   = "validating_request"
	daemonJobStageCreatingDisk        = "creating_disk"
	daemonJobStageWritingSeed         = "writing_seed"
	daemonJobStageStartingVM          = "starting_vm"
	daemonJobStageWaitingForIP        = "waiting_for_ip"
	daemonJobStageWaitingForSSH       = "waiting_for_ssh"
	daemonJobStageWaitingForCloudInit = "waiting_for_cloud_init"
	daemonJobStageWarmingDevShell     = "warming_dev_shell"
	daemonJobStageReady               = "ready"
)

type daemonCreateVMRequest struct {
	Name       string `json:"name,omitempty"`
	RepoURL    string `json:"repo_url"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

type daemonCreateVMResponse struct {
	JobID string `json:"job_id"`
}

type daemonJobStatus struct {
	JobID   string `json:"job_id"`
	State   string `json:"state"`
	Stage   string `json:"stage,omitempty"`
	Message string `json:"message,omitempty"`
	VMName  string `json:"vm_name,omitempty"`
	IP      string `json:"ip,omitempty"`
	Error   string `json:"error,omitempty"`
}
